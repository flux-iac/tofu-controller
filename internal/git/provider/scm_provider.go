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
	"github.com/jenkins-x/go-scm/scm/factory"
	"github.com/jenkins-x/go-scm/scm/transport/oauth2"
)

// commentService abstracts the difference between providers that use
// Issues (GitHub, Gitea) vs PullRequests (GitLab, Bitbucket) for
// PR/MR comments.
type commentService interface {
	CreateComment(ctx context.Context, repo string, number int, input *scm.CommentInput) (*scm.Comment, *scm.Response, error)
	ListComments(ctx context.Context, repo string, number int, opts *scm.ListOptions) ([]*scm.Comment, *scm.Response, error)
	FindComment(ctx context.Context, repo string, number int, id int) (*scm.Comment, *scm.Response, error)
	EditComment(ctx context.Context, repo string, number int, id int, input *scm.CommentInput) (*scm.Comment, *scm.Response, error)
}

// scmProviderConfig defines the driver-specific settings for an SCM provider.
type scmProviderConfig struct {
	// driverName is the go-scm factory driver string (e.g. "github", "gitlab", "bitbucket").
	driverName string
	// defaultHostname is used when no hostname is provided (e.g. "github.com", "bitbucket.org").
	// Empty means a hostname is required (self-hosted only providers).
	defaultHostname string
	// usePRComments indicates whether to use PullRequests service for comments
	// (true for GitLab, Bitbucket) or Issues service (false for GitHub, Gitea).
	usePRComments bool
	// supportsEditComment indicates whether the provider supports FindComment/EditComment.
	// When false, UpdateCommentOfPullRequest will always create a new comment.
	supportsEditComment bool
	// oauthTokenPath is the path appended to the server URL to form the OAuth2
	// token endpoint (e.g. "/site/oauth2/access_token" for Bitbucket Cloud,
	// "/login/oauth/access_token" for Gitea). Empty means OAuth2 is not supported.
	oauthTokenPath string
}

// SCMOAuthConfig holds OAuth2 credentials for generic SCM providers
// (Bitbucket Cloud, Gitea) that support the refresh token grant.
type SCMOAuthConfig struct {
	ClientID     string
	ClientSecret string
	Token        string
	RefreshToken string
}

// scmProvider is a generic provider implementation backed by go-scm.
type scmProvider struct {
	log         logr.Logger
	apiToken    string
	hostname    string
	oauthConfig *SCMOAuthConfig
	config      scmProviderConfig
	client      *scm.Client
}

func (p *scmProvider) comments() commentService {
	if p.config.usePRComments {
		return p.client.PullRequests
	}
	return p.client.Issues
}

func (p *scmProvider) ListPullRequestChanges(ctx context.Context, pr PullRequest) ([]Change, error) {
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

func (p *scmProvider) ListPullRequests(ctx context.Context, repo Repository) ([]PullRequest, error) {
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

func (p *scmProvider) AddCommentToPullRequest(ctx context.Context, pr PullRequest, body []byte) (*Comment, error) {
	comment, _, err := p.comments().CreateComment(ctx, pr.Repository.String(), pr.Number, &scm.CommentInput{
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

func (p *scmProvider) GetLastComments(ctx context.Context, pr PullRequest, since time.Time) ([]*Comment, error) {
	var allComments []*scm.Comment

	opts := scm.ListOptions{Page: 1, Size: defaultPageSize}
	for {
		comments, res, err := p.comments().ListComments(ctx, pr.Repository.String(), pr.Number, &opts)
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

func (p *scmProvider) UpdateCommentOfPullRequest(ctx context.Context, pr PullRequest, commentID int, body []byte) error {
	svc := p.comments()

	// If this provider doesn't support find/edit, always create a new comment.
	if !p.config.supportsEditComment {
		_, _, err := svc.CreateComment(ctx, pr.Repository.String(), pr.Number, &scm.CommentInput{
			Body: string(body),
		})
		return err
	}

	comment, _, err := svc.FindComment(ctx, pr.Repository.String(), pr.Number, commentID)

	if err != nil {
		if errors.Is(err, scm.ErrNotFound) {
			_, _, err = svc.CreateComment(ctx, pr.Repository.String(), pr.Number, &scm.CommentInput{
				Body: string(body),
			})
		}

		return err
	}

	if strings.Contains(comment.Body, "```hcl") {
		_, _, err := svc.CreateComment(ctx, pr.Repository.String(), pr.Number, &scm.CommentInput{
			Body: string(body),
		})

		return err
	}

	_, _, err = svc.EditComment(ctx, pr.Repository.String(), pr.Number, commentID, &scm.CommentInput{
		Body: string(body),
	})

	return err
}

func (p *scmProvider) SetLogger(log logr.Logger) error {
	p.log = log
	return nil
}

func (p *scmProvider) SetToken(tokenType, token string) error {
	switch tokenType {
	case APITokenType:
		p.apiToken = token
	default:
		return fmt.Errorf("unknown token type: %s", tokenType)
	}
	return nil
}

func (p *scmProvider) SetHostname(hostname string) error {
	p.hostname = hostname
	return nil
}

func (p *scmProvider) Setup() error {
	if p.hostname == "" {
		if p.config.defaultHostname == "" {
			return fmt.Errorf("missing required option: hostname (self-hosted provider)")
		}
		p.hostname = p.config.defaultHostname
	}

	serverURL := fmt.Sprintf("https://%s", p.hostname)

	// OAuth2 authentication with auto-refresh.
	if p.oauthConfig != nil && p.config.oauthTokenPath != "" {
		var err error
		p.client, err = factory.NewClient(p.config.driverName, serverURL, "")
		if err != nil {
			return fmt.Errorf("failed to create %s client: %w", p.config.driverName, err)
		}

		refresher := &oauth2.Refresher{
			ClientID:     p.oauthConfig.ClientID,
			ClientSecret: p.oauthConfig.ClientSecret,
			Endpoint:     serverURL + p.config.oauthTokenPath,
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

	// Static token authentication.
	if p.apiToken == "" {
		return fmt.Errorf("missing required option: Token")
	}

	var err error
	p.client, err = factory.NewClient(p.config.driverName, serverURL, p.apiToken)
	if err != nil {
		return fmt.Errorf("failed to create %s client: %w", p.config.driverName, err)
	}

	return nil
}

func newSCMProvider(cfg scmProviderConfig) *scmProvider {
	return &scmProvider{
		log:    logr.Discard(),
		config: cfg,
	}
}
