package config_test

import (
	"testing"

	"github.com/flux-iac/tofu-controller/internal/config"
	gm "github.com/onsi/gomega"
)

func Test_PullRequestObjectName(t *testing.T) {
	g := gm.NewWithT(t)
	g.Expect(config.PullRequestObjectName("fancy-tf", "123")).
		To(gm.Equal("fancy-tf-pr-123"))
}

func Test_SourceName(t *testing.T) {
	g := gm.NewWithT(t)
	g.Expect(config.SourceName("fancy-tf", "fancy-source", "123")).
		To(gm.Equal("fancy-source-3ec173d658529c0e7327-pr-123"))
	g.Expect(config.SourceName("fancy-tf", "fancy-source", "124")).
		To(gm.Equal("fancy-source-1933cdc683fdc86fb62e-pr-124"))
}
