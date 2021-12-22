module github.com/chanwit/tf-controller

go 1.16

require (
	github.com/cyphar/filepath-securejoin v0.2.3
	github.com/fluxcd/pkg/apis/meta v0.10.1
	github.com/fluxcd/pkg/runtime v0.12.2
	github.com/fluxcd/pkg/untar v0.1.0
	github.com/fluxcd/source-controller/api v0.19.2
	github.com/go-logr/logr v0.4.0
	github.com/google/uuid v1.2.0 // indirect
	github.com/hashicorp/go-retryablehttp v0.6.8
	github.com/hashicorp/terraform-exec v0.15.0
	github.com/onsi/gomega v1.15.0
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/crypto v0.0.0-20210616213533-5ff15b29337e // indirect
	k8s.io/api v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	sigs.k8s.io/cli-utils v0.26.1 // indirect
	sigs.k8s.io/controller-runtime v0.10.3
	sigs.k8s.io/yaml v1.3.0 // indirect
)
