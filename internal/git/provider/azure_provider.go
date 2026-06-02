package provider

func newAzureProvider() *scmProvider {
	return newSCMProvider(scmProviderConfig{
		driverName:          "azure",
		defaultHostname:     "dev.azure.com",
		usePRComments:       false,
		supportsEditComment: false,
		oauthTokenPath:      "",
	})
}
