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
			url:      "https://bitbucket.org/team/my-repo",
			repoOrg:  "team",
			repoName: "my-repo",
		},
		{
			url:      "ssh://git@bitbucket.org/team/my-repo.git",
			repoOrg:  "team",
			repoName: "my-repo",
		},
		{
			url:         "https://dev.azure.com/myorg/myproject/_git/myrepo",
			repoOrg:     "myorg",
			repoName:    "myrepo",
			repoProject: "myproject",
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

func TestNewBitbucketCloudProvider(t *testing.T) {
	p, err := provider.New(provider.ProviderBitbucket, provider.WithToken("api-token", "test-token"))
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestNewBitbucketServerProvider(t *testing.T) {
	p, err := provider.New(provider.ProviderBitbucketServer, provider.WithToken("api-token", "test-token"), provider.WithDomain("bitbucket.mycompany.com"))
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestNewBitbucketServerProviderMissingHostname(t *testing.T) {
	_, err := provider.New(provider.ProviderBitbucketServer, provider.WithToken("api-token", "test-token"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hostname")
}

func TestNewGiteaProvider(t *testing.T) {
	// Gitea's factory.NewClient verifies the server by calling /api/v1/version,
	// so creation fails with a dial error for fake hostnames. We verify that the
	// error is a network error (not a config error) to confirm the provider was
	// correctly configured up to the point of the network call.
	_, err := provider.New(provider.ProviderGitea, provider.WithToken("api-token", "test-token"), provider.WithDomain("gitea.test.invalid"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gitea.test.invalid")
}

func TestNewGiteaProviderMissingHostname(t *testing.T) {
	_, err := provider.New(provider.ProviderGitea, provider.WithToken("api-token", "test-token"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hostname")
}

func TestNewAzureProvider(t *testing.T) {
	p, err := provider.New(provider.ProviderAzure, provider.WithToken("api-token", "test-token"))
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

func TestOptsFromSecretToken(t *testing.T) {
	data := map[string][]byte{
		"token": []byte("my-token"),
	}
	opts, err := provider.OptsFromSecret(data)
	require.NoError(t, err)
	assert.Len(t, opts, 1)
}

func TestOptsFromSecretGitHubApp(t *testing.T) {
	data := map[string][]byte{
		"githubAppID":             []byte("12345"),
		"githubAppInstallationID": []byte("67890"),
		"githubAppPrivateKey":     []byte("-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----"),
	}
	opts, err := provider.OptsFromSecret(data)
	require.NoError(t, err)
	assert.Len(t, opts, 1)
}

func TestOptsFromSecretInvalidAppID(t *testing.T) {
	data := map[string][]byte{
		"githubAppID":             []byte("not-a-number"),
		"githubAppInstallationID": []byte("67890"),
		"githubAppPrivateKey":     []byte("key"),
	}
	_, err := provider.OptsFromSecret(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "githubAppID")
}

func TestOptsFromSecretGitLabOAuth(t *testing.T) {
	data := map[string][]byte{
		"gitlabOAuthClientID":     []byte("client-id"),
		"gitlabOAuthClientSecret": []byte("client-secret"),
		"gitlabOAuthRefreshToken": []byte("refresh-token"),
	}
	opts, err := provider.OptsFromSecret(data)
	require.NoError(t, err)
	assert.Len(t, opts, 1)
}

func TestOptsFromSecretGitLabOAuthWithToken(t *testing.T) {
	data := map[string][]byte{
		"token":                   []byte("current-access-token"),
		"gitlabOAuthClientID":     []byte("client-id"),
		"gitlabOAuthClientSecret": []byte("client-secret"),
		"gitlabOAuthRefreshToken": []byte("refresh-token"),
	}
	opts, err := provider.OptsFromSecret(data)
	require.NoError(t, err)
	assert.Len(t, opts, 1)
}

func TestOptsFromSecretGenericOAuth(t *testing.T) {
	data := map[string][]byte{
		"oauthClientID":     []byte("client-id"),
		"oauthClientSecret": []byte("client-secret"),
		"oauthRefreshToken": []byte("refresh-token"),
	}
	opts, err := provider.OptsFromSecret(data)
	require.NoError(t, err)
	assert.Len(t, opts, 1)
}

func TestOptsFromSecretGenericOAuthWithToken(t *testing.T) {
	data := map[string][]byte{
		"token":             []byte("current-access-token"),
		"oauthClientID":     []byte("client-id"),
		"oauthClientSecret": []byte("client-secret"),
		"oauthRefreshToken": []byte("refresh-token"),
	}
	opts, err := provider.OptsFromSecret(data)
	require.NoError(t, err)
	assert.Len(t, opts, 1)
}

func TestOptsFromSecretEmpty(t *testing.T) {
	data := map[string][]byte{}
	_, err := provider.OptsFromSecret(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "secret must contain")
}

func TestHostnameValidation(t *testing.T) {
	malicious := []string{
		"evil.com/foo?x=",
		"evil.com/path",
		"evil.com?query",
		"evil.com#fragment",
		"user@evil.com",
		"evil.com\\path",
		"evil..com",
	}
	for _, hostname := range malicious {
		t.Run(hostname, func(t *testing.T) {
			_, err := provider.New(provider.ProviderGitHub, provider.WithToken("api-token", "tok"), provider.WithDomain(hostname))
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid hostname")
		})
	}

	valid := []string{
		"github.com",
		"gitlab.mycompany.com",
		"git.internal.io",
		"192.168.1.1",
		"dev.azure.com",
	}
	for _, hostname := range valid {
		t.Run(hostname, func(t *testing.T) {
			_, err := provider.New(provider.ProviderGitHub, provider.WithToken("api-token", "tok"), provider.WithDomain(hostname))
			assert.NoError(t, err)
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
