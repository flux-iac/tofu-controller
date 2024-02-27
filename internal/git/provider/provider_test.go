package provider_test

import (
	"testing"

	"github.com/flux-iac/tofu-controller/internal/git/provider"
	"github.com/stretchr/testify/assert"
)

func TestFromURL(t *testing.T) {
	testCases := []struct {
		url         string
		repoName    string
		repoOrg     string
		repoProject string
		shouldError bool
	}{
		{
			url:      "https://github.com/flux-iac/tofu-controller",
			repoOrg:  "weaveworks",
			repoName: "tf-controller",
		},
		{
			url:      "https://github.com/flux-iac/tofu-controller.git",
			repoOrg:  "weaveworks",
			repoName: "tf-controller",
		},
		{
			url:      "ssh://git@github.com/flux-iac/tofu-controller.git",
			repoOrg:  "weaveworks",
			repoName: "tf-controller",
		},
		{
			url:         "https://github.com/weaveworks",
			shouldError: true,
		},
		{
			url:         "https://weave.works/weaveworks/tf-controller",
			shouldError: true,
		},
	}

	for _, testCase := range testCases {
		p, repo, err := provider.FromURL(testCase.url, provider.WithToken("api-token", "token"))

		if testCase.shouldError {
			assert.Error(t, err)
			assert.Nil(t, p)
		} else {
			assert.NoError(t, err)
			assert.NotNil(t, p)
		}

		assert.Equal(t, testCase.repoOrg, repo.Org)
		assert.Equal(t, testCase.repoName, repo.Name)
		assert.Equal(t, testCase.repoProject, repo.Project)
	}

}
