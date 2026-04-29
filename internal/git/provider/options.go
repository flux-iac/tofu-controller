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
)

// GitHubAppConfig holds the configuration for GitHub App authentication.
type GitHubAppConfig struct {
	AppID          int64
	InstallationID int64
	PrivateKey     []byte
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

// OptsFromSecret builds provider options from a Kubernetes Secret's data map.
// It detects the auth type based on which keys are present:
//
//   - "token": API token auth (PAT, Project/Group Access Token, etc.)
//   - "githubAppID" + "githubAppInstallationID" + "githubAppPrivateKey": GitHub App auth
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

	// API token authentication
	if data["token"] != nil {
		return []ProviderOption{
			WithToken(APITokenType, string(data["token"])),
		}, nil
	}

	return nil, fmt.Errorf("secret must contain either a 'token' key or GitHub App keys (githubAppID, githubAppInstallationID, githubAppPrivateKey)")
}
