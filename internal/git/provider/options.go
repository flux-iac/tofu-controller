package provider

import (
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
)

const (
	APITokenType = "api-token"

	// defaultPageSize is the number of items to request per page from the SCM API.
	defaultPageSize = 100

	// maxPages is the upper bound on pagination to prevent resource exhaustion
	// from repositories with extremely large numbers of PRs or comments.
	maxPages = 50
)

// GitHubAppConfig holds the configuration for GitHub App authentication.
type GitHubAppConfig struct {
	AppID          int64
	InstallationID int64
	PrivateKey     []byte
}

// GitLabOAuthConfig holds the configuration for GitLab OAuth2 authentication
// with automatic token refresh.
type GitLabOAuthConfig struct {
	ClientID     string
	ClientSecret string
	Token        string
	RefreshToken string
}

type ProviderOption func(Provider) error

func WithLogger(log logr.Logger) ProviderOption {
	return func(p Provider) error {
		return p.SetLogger(log)
	}
}

func WithToken(tokenType, token string) ProviderOption {
	return func(p Provider) error {
		return p.SetToken(tokenType, token)
	}
}

func WithDomain(domain string) ProviderOption {
	return func(p Provider) error {
		if err := validateHostname(domain); err != nil {
			return err
		}
		return p.SetHostname(domain)
	}
}

func WithGitHubApp(cfg GitHubAppConfig) ProviderOption {
	return func(p Provider) error {
		if gh, ok := p.(*GitHubProvider); ok {
			gh.appConfig = &cfg
			return nil
		}
		return nil
	}
}

func WithGitLabOAuth(cfg GitLabOAuthConfig) ProviderOption {
	return func(p Provider) error {
		if gl, ok := p.(*GitLabProvider); ok {
			gl.oauthConfig = &cfg
			return nil
		}
		return nil
	}
}

func WithSCMOAuth(cfg SCMOAuthConfig) ProviderOption {
	return func(p Provider) error {
		if sp, ok := p.(*scmProvider); ok {
			sp.oauthConfig = &cfg
			return nil
		}
		return nil
	}
}

// OptsFromSecret builds provider options from a Kubernetes Secret's data map.
// It detects the auth type based on which keys are present:
//
//   - "githubAppID" + "githubAppInstallationID" + "githubAppPrivateKey": GitHub App auth
//   - "gitlabOAuthClientID" + "gitlabOAuthClientSecret" + "gitlabOAuthRefreshToken": GitLab OAuth2 with auto-refresh
//   - "token": API token auth (PAT, Project/Group Access Token, etc.)
func OptsFromSecret(data map[string][]byte) ([]ProviderOption, error) {
	// GitHub App authentication
	if data["githubAppID"] != nil && data["githubAppInstallationID"] != nil && data["githubAppPrivateKey"] != nil {
		appID, err := strconv.ParseInt(string(data["githubAppID"]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid githubAppID: %w", err)
		}

		installationID, err := strconv.ParseInt(string(data["githubAppInstallationID"]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid githubAppInstallationID: %w", err)
		}

		return []ProviderOption{
			WithGitHubApp(GitHubAppConfig{
				AppID:          appID,
				InstallationID: installationID,
				PrivateKey:     data["githubAppPrivateKey"],
			}),
		}, nil
	}

	// GitLab OAuth2 authentication with auto-refresh
	if data["gitlabOAuthClientID"] != nil && data["gitlabOAuthClientSecret"] != nil && data["gitlabOAuthRefreshToken"] != nil {
		cfg := GitLabOAuthConfig{
			ClientID:     string(data["gitlabOAuthClientID"]),
			ClientSecret: string(data["gitlabOAuthClientSecret"]),
			RefreshToken: string(data["gitlabOAuthRefreshToken"]),
		}
		if data["token"] != nil {
			cfg.Token = string(data["token"])
		}

		return []ProviderOption{
			WithGitLabOAuth(cfg),
		}, nil
	}

	// Generic OAuth2 authentication (Bitbucket Cloud, Gitea)
	if data["oauthClientID"] != nil && data["oauthClientSecret"] != nil && data["oauthRefreshToken"] != nil {
		cfg := SCMOAuthConfig{
			ClientID:     string(data["oauthClientID"]),
			ClientSecret: string(data["oauthClientSecret"]),
			RefreshToken: string(data["oauthRefreshToken"]),
		}
		if data["token"] != nil {
			cfg.Token = string(data["token"])
		}

		return []ProviderOption{
			WithSCMOAuth(cfg),
		}, nil
	}

	// API token authentication
	if data["token"] != nil {
		return []ProviderOption{
			WithToken(APITokenType, string(data["token"])),
		}, nil
	}

	return nil, fmt.Errorf("secret must contain a 'token' key, GitHub App keys, GitLab OAuth keys, or OAuth keys (oauthClientID, oauthClientSecret, oauthRefreshToken)")
}
