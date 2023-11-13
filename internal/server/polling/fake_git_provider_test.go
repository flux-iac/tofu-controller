package polling_test

import (
	"fmt"

	giturl "github.com/kubescape/go-git-url"
	"github.com/weaveworks/tf-controller/internal/git/provider"
)

func mockedProvider(gitProvider provider.Provider) provider.URLParserFn {
	return func(repoURL string, options ...provider.ProviderOption) (provider.Provider, provider.Repository, error) {
		gitURL, err := giturl.NewGitURL(repoURL)
		if err != nil {
			return nil, provider.Repository{}, fmt.Errorf("failed parsing repository url: %w", err)
		}

		repo := provider.Repository{
			Org:  gitURL.GetOwnerName(),
			Name: gitURL.GetRepoName(),
		}

		return gitProvider, repo, nil
	}
}
