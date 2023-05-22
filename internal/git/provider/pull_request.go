package provider

type PullRequest struct {
	Repository Repository
	BaseBranch string
	HeadBranch string
}
