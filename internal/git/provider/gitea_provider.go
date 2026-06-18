package provider

func newGiteaProvider() *scmProvider {
	return newSCMProvider(scmProviderConfig{
		driverName:          "gitea",
		defaultHostname:     "",
		usePRComments:       false,
		supportsEditComment: true,
		oauthTokenPath:      "/login/oauth/access_token",
	})
}
