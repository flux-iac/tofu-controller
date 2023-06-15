package provider

import (
	"github.com/go-logr/logr"
)

const APITokenType = "api-token"

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
