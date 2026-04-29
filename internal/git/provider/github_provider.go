package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/go-logr/logr"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/go-scm/scm/driver/github"
	"github.com/jenkins-x/go-scm/scm/factory"
)

type GitHubProvider struct {
	log       logr.Logger
	apiToken  string
	hostname  string
	appConfig *GitHubAppConfig
	client    *scm.Client
}

func (p *GitHubProvider) ListPullRequestChanges(ctx context.Context, pr PullRequest) ([]Change, error) {
	changes := []Change{}

	changeList, _, err := p.client.PullRequests.ListChanges(ctx, pr.Repository.String(), pr.Number, &scm.ListOptions{})
	if err != nil {
		return changes, fmt.Errorf("unable to list pull request changes: %w", err)
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

func (p *GitHubProvider) ListPullRequests(ctx context.Context, repo Repository) ([]PullRequest, error) {
	var prs []PullRequest

	opts := scm.PullRequestListOptions{Page: 1, Size: defaultPageSize}
	for {
		prList, res, err := p.client.PullRequests.List(ctx, repo.String(), &opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list pull requests: %w", err)
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

func (p *GitHubProvider) AddCommentToPullRequest(ctx context.Context, pr PullRequest, body []byte) (*Comment, error) {
	comment, _, err := p.client.Issues.CreateComment(ctx, pr.Repository.String(), pr.Number, &scm.CommentInput{
		Body: string(body),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add comment to pull request: %w", err)
	}

	return &Comment{
		ID:   comment.ID,
		Link: comment.Link,
	}, nil
}

func (p *GitHubProvider) GetLastComments(ctx context.Context, pr PullRequest, since time.Time) ([]*Comment, error) {
	var allComments []*scm.Comment

	opts := scm.ListOptions{Page: 1, Size: defaultPageSize}
	for {
		comments, res, err := p.client.Issues.ListComments(ctx, pr.Repository.String(), pr.Number, &opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list pull request comments: %w", err)
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

func (p *GitHubProvider) UpdateCommentOfPullRequest(ctx context.Context, pr PullRequest, commentID int, body []byte) error {
	// tf-controller plan output:
	comment, _, err := p.client.Issues.FindComment(ctx, pr.Repository.String(), pr.Number, commentID)

	// if comment not found, scm.ErrNotFound
	if err != nil {
		if errors.Is(err, scm.ErrNotFound) {
			_, _, err = p.client.Issues.CreateComment(ctx, pr.Repository.String(), pr.Number, &scm.CommentInput{
				Body: string(body),
			})
		}

		return err
	}

	// if comment already contains hcl code block
	if strings.Contains(comment.Body, "```hcl") {
		// create new comment
		_, _, err := p.client.Issues.CreateComment(ctx, pr.Repository.String(), pr.Number, &scm.CommentInput{
			Body: string(body),
		})

		return err
	}

	// else update body to the placeholder comment
	_, _, err = p.client.Issues.EditComment(ctx, pr.Repository.String(), pr.Number, commentID, &scm.CommentInput{
		Body: string(body),
	})

	return err
}

func (p *GitHubProvider) SetLogger(log logr.Logger) error {
	p.log = log

	return nil
}

func (p *GitHubProvider) SetToken(tokenType, token string) error {
	switch tokenType {
	case APITokenType:
		p.apiToken = token
	default:
		return fmt.Errorf("unknown token type: %s", tokenType)
	}

	return nil
}

func (p *GitHubProvider) SetHostname(hostname string) error {
	p.hostname = hostname

	return nil
}

func (p *GitHubProvider) Setup() error {
	if p.hostname == "" {
		p.hostname = "github.com"
	}

	serverURL := fmt.Sprintf("https://%s", p.hostname)

	// GitHub App authentication: uses a short-lived installation token
	// that is automatically refreshed by the ghinstallation transport.
	if p.appConfig != nil {
		transport, err := ghinstallation.New(
			http.DefaultTransport,
			p.appConfig.AppID,
			p.appConfig.InstallationID,
			p.appConfig.PrivateKey,
		)
		if err != nil {
			return fmt.Errorf("failed to create github app transport: %w", err)
		}

		if p.hostname != "github.com" {
			transport.BaseURL = serverURL + "/api/v3"
		}

		p.client, err = github.New(serverURL)
		if err != nil {
			return fmt.Errorf("failed to create github client: %w", err)
		}
		p.client.Client = &http.Client{Transport: transport}

		return nil
	}

	// API token authentication (PAT or fine-grained token).
	if p.apiToken == "" {
		return fmt.Errorf("missing required option: Token or GitHubApp config")
	}

	var err error
	p.client, err = factory.NewClient("github", serverURL, p.apiToken)
	if err != nil {
		return fmt.Errorf("failed to create github client: %w", err)
	}

	return nil
}

func newGitHubProvider() *GitHubProvider {
	return &GitHubProvider{
		log: logr.Discard(),
	}
}
