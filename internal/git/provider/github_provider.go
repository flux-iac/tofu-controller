package provider

import (
	"fmt"

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

func (p GitHubProvider) ListPullRequests(ctx context.Context, repo Repository) ([]PullRequest, error) {
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
		})
	}

	return prs, nil
}

func (p GitHubProvider) AddCommentToPullRequest(ctx context.Context, pr PullRequest, body []byte) (*Comment, error) {
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

func (p *GitHubProvider) SetLogger(log logr.Logger) error {
	p.log = log

	return nil
}

func (p *GitHubProvider) SetToken(tokenType, token string) error {
	switch tokenType {
	case APITokenType:
		p.apiToken = token
	default:
		return fmt.Errorf("uknown token type: %s", tokenType)
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
