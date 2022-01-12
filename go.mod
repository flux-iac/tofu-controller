module github.com/chanwit/tf-controller

go 1.16

require (
	github.com/cyphar/filepath-securejoin v0.2.3
	github.com/fluxcd/pkg/apis/meta v0.10.2
	github.com/fluxcd/pkg/runtime v0.12.2
	github.com/fluxcd/pkg/untar v0.1.0
	github.com/fluxcd/source-controller/api v0.20.1
	github.com/go-logr/logr v1.2.2
	github.com/hashicorp/go-retryablehttp v0.6.8
	github.com/hashicorp/terraform-exec v0.15.0
	github.com/onsi/gomega v1.17.0
	github.com/spf13/pflag v1.0.5
	github.com/zclconf/go-cty v1.9.1
	k8s.io/api v0.23.0
	k8s.io/apimachinery v0.23.1
	k8s.io/client-go v0.23.0
	sigs.k8s.io/cli-utils v0.26.1
	sigs.k8s.io/controller-runtime v0.11.0
)
