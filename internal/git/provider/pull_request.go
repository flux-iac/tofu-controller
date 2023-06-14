package provider

type PullRequest struct {
	Repository Repository
	Number     int
	BaseBranch string
	HeadBranch string
	BaseSha    string
	HeadSha    string
}
