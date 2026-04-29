package provider_test

import (
	"testing"

	"github.com/flux-iac/tofu-controller/internal/git/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			repoOrg:  "flux-iac",
			repoName: "tofu-controller",
		},
		{
			url:      "https://github.com/flux-iac/tofu-controller.git",
			repoOrg:  "flux-iac",
			repoName: "tofu-controller",
		},
		{
			url:      "ssh://git@github.com/flux-iac/tofu-controller.git",
			repoOrg:  "flux-iac",
			repoName: "tofu-controller",
		},
		{
			url:      "https://gitlab.com/flux-iac/tofu-controller",
			repoOrg:  "flux-iac",
			repoName: "tofu-controller",
		},
		{
			url:      "https://gitlab.com/flux-iac/tofu-controller.git",
			repoOrg:  "flux-iac",
			repoName: "tofu-controller",
		},
		{
			url:      "ssh://git@gitlab.com/flux-iac/tofu-controller.git",
			repoOrg:  "flux-iac",
			repoName: "tofu-controller",
		},
		{
			url:         "https://github.com/flux-iac",
			shouldError: true,
		},
		{
			url:         "https://weave.works/flux-iac/tofu-controller",
			shouldError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.url, func(t *testing.T) {
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
		})
	}
}

func TestNewGitLabProvider(t *testing.T) {
	p, err := provider.New(provider.ProviderGitlab, provider.WithToken("api-token", "test-token"))
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestNewGitLabProviderMissingToken(t *testing.T) {
	_, err := provider.New(provider.ProviderGitlab)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required option: Token")
}

func TestNewGitHubProvider(t *testing.T) {
	p, err := provider.New(provider.ProviderGitHub, provider.WithToken("api-token", "test-token"))
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestNewUnknownProvider(t *testing.T) {
	_, err := provider.New(provider.ProviderType("unknown"), provider.WithToken("api-token", "test-token"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}

func TestRepoFromURL(t *testing.T) {
	testCases := []struct {
		url      string
		repoOrg  string
		repoName string
	}{
		{
			url:      "https://github.com/flux-iac/tofu-controller",
			repoOrg:  "flux-iac",
			repoName: "tofu-controller",
		},
		{
			url:      "https://gitlab.com/group/subgroup/project",
			repoOrg:  "group/subgroup",
			repoName: "project",
		},
		{
			url:      "ssh://git@gitlab.mycompany.com/team/infra/modules.git",
			repoOrg:  "team/infra",
			repoName: "modules",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.url, func(t *testing.T) {
			repo, err := provider.RepoFromURL(testCase.url)
			require.NoError(t, err)
			assert.Equal(t, testCase.repoOrg, repo.Org)
			assert.Equal(t, testCase.repoName, repo.Name)
		})
	}
}

func TestFromURLSelfHosted(t *testing.T) {
	testCases := []struct {
		url      string
		repoOrg  string
		repoName string
	}{
		{
			url:      "https://gitlab.mycompany.com/infra/terraform",
			repoOrg:  "infra",
			repoName: "terraform",
		},
		{
			url:      "https://gitlab.internal.io/team/subgroup/modules",
			repoOrg:  "team/subgroup",
			repoName: "modules",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.url, func(t *testing.T) {
			p, repo, err := provider.FromURL(testCase.url, provider.WithToken("api-token", "token"))
			require.NoError(t, err)
			assert.NotNil(t, p)
			assert.Equal(t, testCase.repoOrg, repo.Org)
			assert.Equal(t, testCase.repoName, repo.Name)
		})
	}
}
