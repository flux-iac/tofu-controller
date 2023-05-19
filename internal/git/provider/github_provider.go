package provider

import "golang.org/x/net/context"

type GitHubProvider struct{}

func (p GitHubProvider) ListPullRequests(ctx context.Context, repo Repository) (_ []PullRequest) {
	panic("not implemented") // TODO: Implement
}
func (p GitHubProvider) AddCommentToPullREquest(ctx context.Context, repo PullRequest, comment []byte) {
	panic("not implemented") // TODO: Implement
}

func newGitHubProvider() GitHubProvider {
	return GitHubProvider{}
}
