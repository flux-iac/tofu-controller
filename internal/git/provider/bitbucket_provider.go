package provider

func newBitbucketCloudProvider() *scmProvider {
	return newSCMProvider(scmProviderConfig{
		driverName:          "bitbucket",
		defaultHostname:     "bitbucket.org",
		usePRComments:       true,
		supportsEditComment: false,
		oauthTokenPath:      "/site/oauth2/access_token",
	})
}

func newBitbucketServerProvider() *scmProvider {
	return newSCMProvider(scmProviderConfig{
		driverName:          "stash",
		defaultHostname:     "",
		usePRComments:       true,
		supportsEditComment: true,
	})
}
