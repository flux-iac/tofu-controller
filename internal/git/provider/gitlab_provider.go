package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/go-scm/scm/driver/gitlab"
	"github.com/jenkins-x/go-scm/scm/factory"
	"github.com/jenkins-x/go-scm/scm/transport/oauth2"
)

type GitLabProvider struct {
	log         logr.Logger
	apiToken    string
	hostname    string
	oauthConfig *GitLabOAuthConfig
	client      *scm.Client
}

func (p *GitLabProvider) ListPullRequestChanges(ctx context.Context, pr PullRequest) ([]Change, error) {
	changes := []Change{}

	changeList, _, err := p.client.PullRequests.ListChanges(ctx, pr.Repository.String(), pr.Number, &scm.ListOptions{})
	if err != nil {
		return changes, fmt.Errorf("unable to list merge request changes: %w", err)
	}

	for _, change := range changeList {
		changes = append(changes, Change{
			Path:         change.Path,
			PreviousPath: change.PreviousPath,
			Patch:        change.Patch,
			Sha:          change.Sha,
			Additions:    change.Additions,
			Deletions:    change.Deletions,
			Changes:      change.Changes,
			Added:        change.Added,
			Renamed:      change.Renamed,
			Deleted:      change.Deleted,
		})
	}

	return changes, nil
}

func (p *GitLabProvider) ListPullRequests(ctx context.Context, repo Repository) ([]PullRequest, error) {
	var prs []PullRequest

	opts := scm.PullRequestListOptions{Page: 1, Size: defaultPageSize}
	for {
		prList, res, err := p.client.PullRequests.List(ctx, repo.String(), &opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list merge requests: %w", err)
		}

		for _, pr := range prList {
			prs = append(prs, PullRequest{
				Repository: repo,
				Number:     pr.Number,
				BaseBranch: pr.Base.Ref,
				HeadBranch: pr.Head.Ref,
				BaseSha:    pr.Base.Sha,
				HeadSha:    pr.Head.Sha,
				Closed:     pr.Closed,
			})
		}

		if res.Page.Next == 0 || opts.Page >= maxPages {
			break
		}
		opts.Page = res.Page.Next
	}

	return prs, nil
}

func (p *GitLabProvider) AddCommentToPullRequest(ctx context.Context, pr PullRequest, body []byte) (*Comment, error) {
	comment, _, err := p.client.PullRequests.CreateComment(ctx, pr.Repository.String(), pr.Number, &scm.CommentInput{
		Body: string(body),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add comment to merge request: %w", err)
	}

	return &Comment{
		ID:   comment.ID,
		Link: comment.Link,
	}, nil
}

func (p *GitLabProvider) GetLastComments(ctx context.Context, pr PullRequest, since time.Time) ([]*Comment, error) {
	var allComments []*scm.Comment

	opts := scm.ListOptions{Page: 1, Size: defaultPageSize}
	for {
		comments, res, err := p.client.PullRequests.ListComments(ctx, pr.Repository.String(), pr.Number, &opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list merge request comments: %w", err)
		}

		allComments = append(allComments, comments...)

		if res.Page.Next == 0 || opts.Page >= maxPages {
			break
		}
		opts.Page = res.Page.Next
	}

	if len(allComments) == 0 {
		return nil, nil
	}

	sort.Slice(allComments, func(i, j int) bool {
		return allComments[i].Created.After(allComments[j].Created)
	})

	var commentsSince []*Comment
	for _, comment := range allComments {
		if comment.Created.After(since) {
			commentsSince = append(commentsSince, &Comment{
				ID:   comment.ID,
				Link: comment.Link,
				Body: comment.Body,
			})
		}
	}

	return commentsSince, nil
}

func (p *GitLabProvider) UpdateCommentOfPullRequest(ctx context.Context, pr PullRequest, commentID int, body []byte) error {
	comment, _, err := p.client.PullRequests.FindComment(ctx, pr.Repository.String(), pr.Number, commentID)

	if err != nil {
		if errors.Is(err, scm.ErrNotFound) {
			_, _, err = p.client.PullRequests.CreateComment(ctx, pr.Repository.String(), pr.Number, &scm.CommentInput{
				Body: string(body),
			})
		}

		return err
	}

	if strings.Contains(comment.Body, "```hcl") {
		_, _, err := p.client.PullRequests.CreateComment(ctx, pr.Repository.String(), pr.Number, &scm.CommentInput{
			Body: string(body),
		})

		return err
	}

	_, _, err = p.client.PullRequests.EditComment(ctx, pr.Repository.String(), pr.Number, commentID, &scm.CommentInput{
		Body: string(body),
	})

	return err
}

func (p *GitLabProvider) SetLogger(log logr.Logger) error {
	p.log = log

	return nil
}

func (p *GitLabProvider) SetToken(tokenType, token string) error {
	switch tokenType {
	case APITokenType:
		p.apiToken = token
	default:
		return fmt.Errorf("unknown token type: %s", tokenType)
	}

	return nil
}

func (p *GitLabProvider) SetHostname(hostname string) error {
	p.hostname = hostname

	return nil
}

func (p *GitLabProvider) Setup() error {
	if p.hostname == "" {
		p.hostname = "gitlab.com"
	}

	serverURL := fmt.Sprintf("https://%s", p.hostname)

	// OAuth2 authentication: uses a refresh token to obtain and
	// automatically renew access tokens before they expire.
	if p.oauthConfig != nil {
		var err error
		p.client, err = gitlab.New(serverURL)
		if err != nil {
			return fmt.Errorf("failed to create gitlab client: %w", err)
		}

		refresher := &oauth2.Refresher{
			ClientID:     p.oauthConfig.ClientID,
			ClientSecret: p.oauthConfig.ClientSecret,
			Endpoint:     serverURL + "/oauth/token",
			Source: oauth2.StaticTokenSource(&scm.Token{
				Token:   p.oauthConfig.Token,
				Refresh: p.oauthConfig.RefreshToken,
			}),
		}

		p.client.Client = &http.Client{
			Transport: &oauth2.Transport{
				Scheme: "Bearer",
				Source: refresher,
			},
		}

		return nil
	}

	// Static token authentication (PAT, Project/Group Access Token).
	if p.apiToken == "" {
		return fmt.Errorf("missing required option: Token or GitLabOAuth config")
	}

	var err error
	p.client, err = factory.NewClient("gitlab", serverURL, p.apiToken)
	if err != nil {
		return fmt.Errorf("failed to create gitlab client: %w", err)
	}

	return nil
}

func newGitLabProvider() *GitLabProvider {
	return &GitLabProvider{
		log: logr.Discard(),
	}
}
