package provider

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/go-scm/scm/driver/github"
	"github.com/jenkins-x/go-scm/scm/factory"
	"golang.org/x/net/context"
)

type GitHubProvider struct {
	log      logr.Logger
	apiToken string
	hostname string
	client   *scm.Client
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
	prList, _, err := p.client.PullRequests.List(ctx, repo.String(), &scm.PullRequestListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}

	prs := []PullRequest{}

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

	return prs, nil
}

func (p *GitHubProvider) AddCommentToPullRequest(ctx context.Context, pr PullRequest, body []byte) (*Comment, error) {
	comment, _, err := p.client.Issues.CreateComment(ctx, pr.Repository.String(), pr.Number, &scm.CommentInput{
		Body: string(body),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}

	return &Comment{
		ID:   comment.ID,
		Link: comment.Link,
	}, nil
}

func (p *GitHubProvider) GetLastComments(ctx context.Context, pr PullRequest, since time.Time) ([]*Comment, error) {
	// TODO make sure that we get the last comment
	comments, _, err := p.client.Issues.ListComments(ctx, pr.Repository.String(), pr.Number, &scm.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}

	if len(comments) == 0 {
		return nil, nil
	}

	sort.Slice(comments, func(i, j int) bool {
		return comments[i].Created.After(comments[j].Created)
	})

	commentsSince := []*Comment{}
	for _, comment := range comments {
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
	// tofu-controller plan output:
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
	var err error

	if p.apiToken == "" {
		return fmt.Errorf("missing required option: Token")
	}

	if p.hostname == "" {
		p.hostname = "github.com"
	}

	if p.hostname != "" {
		p.client, err = github.New(fmt.Sprintf("https://%s", p.hostname))
		if err != nil {
			return fmt.Errorf("failed to create new github client: %w", err)
		}
	} else {
		p.client = github.NewDefault()
	}

	p.client, err = factory.NewClient(
		"github",
		fmt.Sprintf("https://%s", p.hostname),
		p.apiToken,
	)

	return err
}

func newGitHubProvider() *GitHubProvider {
	return &GitHubProvider{
		log: logr.Discard(),
	}
}
