.DEFAULT_GOAL := help
# Image URL to use all building/pushing image targets
MANAGER_IMG ?= ghcr.io/flux-iac/tofu-controller
RUNNER_IMG  ?= ghcr.io/flux-iac/tf-runner
RUNNER_AZURE_IMAGE ?= ghcr.io/flux-iac/tf-runner-azure
BRANCH_PLANNER_IMAGE ?= ghcr.io/flux-iac/branch-planner
TAG ?= latest
BUILD_SHA ?= $(shell git rev-parse --short HEAD)
BUILD_VERSION ?= $(shell git describe --tags $$(git rev-list --tags --max-count=1))

# Update the following files too:
# - .github/workflows/build-and-publish.yaml
# - .github/workflows/release-runners.yaml
# - .github/workflows/release.yaml
# - Tiltfile
LIBCRYPTO_VERSION ?= 3.3.4-r0

# source controller version
SOURCE_VER ?= v1.0.0-rc.1

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
GOBIN=$(shell pwd)/bin

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# Allows for defining additional Docker buildx arguments, e.g. '--push'.
BUILD_ARGS ?=

# Set architecture for the binaries we build as well as the terraform binary that get bundled in the images
TARGETARCH ?= amd64

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config="config/crd/bases"
	cp config/crd/bases/infra.contrib.fluxcd.io_terraforms.yaml charts/tofu-controller/crds/crds.yaml
	cd api; $(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config="../config/crd/bases"

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	go generate ./...
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	cd api; $(CONTROLLER_GEN) object:headerFile="../hack/boilerplate.go.txt" paths="./..."

# Generate API reference documentation
.PHONY: api-docs
api-docs: gen-crd-api-reference-docs
	$(GEN_CRD_API_REFERENCE_DOCS) -api-dir=./api/v1alpha2 -config=./hack/api-docs/config.json -template-dir=./hack/api-docs/template -out-file=./docs/References/terraform.md

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: verify
verify: fmt vet manifests api-docs
	@if [ ! "$$(git status --porcelain --untracked-files=no)" = "" ]; then \
		echo "working directory is dirty:"; \
		git --no-pager diff; \
		exit 1; \
	fi

download-crd-deps:
	curl -s https://raw.githubusercontent.com/fluxcd/source-controller/${SOURCE_VER}/config/crd/bases/source.toolkit.fluxcd.io_gitrepositories.yaml > config/crd/bases/gitrepositories.yaml
	curl -s https://raw.githubusercontent.com/fluxcd/source-controller/${SOURCE_VER}/config/crd/bases/source.toolkit.fluxcd.io_buckets.yaml > config/crd/bases/buckets.yaml
	curl -s https://raw.githubusercontent.com/fluxcd/source-controller/${SOURCE_VER}/config/crd/bases/source.toolkit.fluxcd.io_ocirepositories.yaml > config/crd/bases/ocirepositories.yaml

TEST_SETTINGS=INSECURE_LOCAL_RUNNER=1 DISABLE_K8S_LOGS=1 DISABLE_TF_LOGS=1 DISABLE_TF_K8S_BACKEND=1 DISABLE_WEBHOOK_TLS_VERIFY=1 KUBEBUILDER_ASSETS="$(shell $(ENVTEST) --arch=$(ENVTEST_ARCH) use -i $(ENVTEST_KUBERNETES_VERSION) --bin-dir=$(ENVTEST_ASSETS_DIR) -p path)"

.PHONY: test
test: manifests generate download-crd-deps fmt vet envtest api-docs ## Run tests.
	$(TEST_SETTINGS) go test ./controllers -coverprofile cover.out -v

# usage: make TARGET=250 target-test
.PHONY: target-test
target-test: manifests generate download-crd-deps fmt vet envtest api-docs ## Run tests. e.g make TARGET=250 target-test
	$(TEST_SETTINGS) go test ./controllers -coverprofile cover.out -v -run $(TARGET)

.PHONY: normal-controller-test
normal-controller-test: manifests generate download-crd-deps fmt vet envtest api-docs ## Run non numbered controller tests.
	$(TEST_SETTINGS) go test ./controllers -coverprofile cover.out -v -skip "Test_0"

.PHONY: test-internal
test-internal: manifests generate download-crd-deps fmt vet envtest api-docs ## Run tests in the internal directory.
	$(TEST_SETTINGS) go test ./internal/... -coverprofile cover.out -v

.PHONY: gen-grpc
gen-grpc:
	env PATH=$(shell pwd)/bin:$$PATH $(PROJECT_DIR)/bin/protoc --go_out=. --go_opt=Mrunner/runner.proto=runner/ --go-grpc_out=. --go-grpc_opt=Mrunner/runner.proto=runner/ runner/runner.proto

##@ Build

.PHONY: build
build: gen-grpc generate fmt vet ## Build manager binary.
	go build -o bin/runner \
		-ldflags "-X main.BuildSHA=$(BUILD_SHA) -X main.BuildVersion=$(BUILD_VERSION)" \
		./cmd/runner
	go build -o bin/manager \
		-ldflags "-X main.BuildSHA=$(BUILD_SHA) -X main.BuildVersion=$(BUILD_VERSION)" \
		./cmd/manager
	go build -o bin/tfctl \
		-ldflags "-X main.BuildSHA=$(BUILD_SHA) -X main.BuildVersion=$(BUILD_VERSION)" \
		./cmd/tfctl

.PHONY: install-cli
install-cli:
	go build -o ${GOPATH}/bin/tfctl \
		-ldflags "-X main.BuildSHA=$(BUILD_SHA) -X main.BuildVersion=$(BUILD_VERSION)" \
		./cmd/tfctl

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run \
		-ldflags "-X main.BuildSHA=$(BUILD_SHA) -X main.BuildVersion=$(BUILD_VERSION)" \
		./cmd/manager/main.go

.PHONY: run-planner
run-planner: manifests generate fmt vet ## Run a branch planner from your host.
	go run \
		-ldflags "-X main.BuildSHA=$(BUILD_SHA) -X main.BuildVersion=$(BUILD_VERSION)" \
		./cmd/branch-planner/

.PHONY: docker-build
docker-build: ## Build docker
	docker build -t ${MANAGER_IMG}:${TAG} --build-arg LIBCRYPTO_VERSION=${LIBCRYPTO_VERSION} --build-arg TARGETARCH=${TARGETARCH} ${BUILD_ARGS} .
	docker build -t ${RUNNER_IMG}:${TAG}-base -f runner-base.Dockerfile --build-arg LIBCRYPTO_VERSION=${LIBCRYPTO_VERSION} --build-arg TARGETARCH=${TARGETARCH} ${BUILD_ARGS} .
	docker build -t ${RUNNER_IMG}:${TAG} -f runner.Dockerfile --build-arg BASE_IMAGE=${RUNNER_IMG}:${TAG}-base --build-arg TARGETARCH=${TARGETARCH} ${BUILD_ARGS} .
	docker build -t ${RUNNER_AZURE_IMAGE}:${TAG} -f runner-azure.Dockerfile --build-arg BASE_IMAGE=${RUNNER_IMG}:${TAG}-base --build-arg TARGETARCH=${TARGETARCH} ${BUILD_ARGS} .
	docker build -t ${BRANCH_PLANNER_IMAGE}:${TAG} -f planner.Dockerfile --build-arg LIBCRYPTO_VERSION=${LIBCRYPTO_VERSION} --build-arg TARGETARCH=${TARGETARCH} ${BUILD_ARGS} .

.PHONY: docker-buildx
docker-buildx: ## Build docker
	docker buildx build --load -t ${MANAGER_IMG}:${TAG} --build-arg LIBCRYPTO_VERSION=${LIBCRYPTO_VERSION} ${BUILD_ARGS} .
	docker buildx build --load -t ${RUNNER_IMG}:${TAG}-base -f runner-base.Dockerfile --build-arg LIBCRYPTO_VERSION=${LIBCRYPTO_VERSION} ${BUILD_ARGS} .
	docker buildx build --load -t ${RUNNER_IMG}:${TAG} -f runner.Dockerfile --build-arg BASE_IMAGE=${RUNNER_IMG}:${TAG}-base ${BUILD_ARGS} .
	docker buildx build --load -t ${RUNNER_AZURE_IMAGE}:${TAG} -f runner-azure.Dockerfile --build-arg BASE_IMAGE=${RUNNER_IMG}:${TAG}-base ${BUILD_ARGS} .
	docker buildx build --load -t ${BRANCH_PLANNER_IMAGE}:${TAG} -f planner.Dockerfile --build-arg LIBCRYPTO_VERSION=${LIBCRYPTO_VERSION} ${BUILD_ARGS} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${MANAGER_IMG}:${TAG}
	docker push ${RUNNER_IMG}:${TAG}-base
	docker push ${RUNNER_IMG}:${TAG}
	docker push ${RUNNER_AZURE_IMAGE}:${TAG}
	docker push ${BRANCH_PLANNER_IMAGE}:${TAG}

docker-dev-runner:
	docker buildx build --load -t ${RUNNER_IMG}:${TAG}-base -f runner-base.Dockerfile --build-arg LIBCRYPTO_VERSION=${LIBCRYPTO_VERSION} ${BUILD_ARGS} .
	docker buildx build --load -t ${RUNNER_IMG}:${TAG} -f runner.Dockerfile --build-arg BASE_IMAGE=${RUNNER_IMG}:${TAG}-base ${BUILD_ARGS} .
	docker push ${RUNNER_IMG}:${TAG}

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply --server-side -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | kubectl delete --server-side --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image flux-iac/tofu-controller=${MANAGER_IMG}:${TAG}
	$(KUSTOMIZE) build config/default | kubectl apply --server-side -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | kubectl delete --server-side --ignore-not-found=$(ignore-not-found) -f -

# Deploy controller dev image in the configured Kubernetes cluster in ~/.kube/config
.PHONY: dev-deploy
dev-deploy: manifests kustomize
	mkdir -p config/dev && cp config/default/* config/dev
	cd config/dev && $(KUSTOMIZE) edit set image ghcr.io/flux-iac/tofu-controller=${MANAGER_IMG}:${TAG}
	$(KUSTOMIZE) build config/dev | yq e "select(.kind == \"Deployment\" and .metadata.name == \"tofu-controller\").spec.template.spec.containers[0].env[1].value = \"test/tf-runner:$${TAG}\"" - | kubectl apply --server-side -f -
	rm -rf config/dev

# Delete dev deployment and CRDs
.PHONY: dev-cleanup
dev-cleanup: manifests kustomize
	mkdir -p config/dev && cp config/default/* config/dev
	cd config/dev && $(KUSTOMIZE) edit set image ghcr.io/flux-iac/tofu-controller=${MANAGER_IMG}:${TAG}
	$(KUSTOMIZE) build config/dev | kubectl delete --server-side -f -
	rm -rf config/dev

.PHONY: tools
tools: kustomize protoc protoc-gen-go protoc-gen-go-grpc controller-gen envtest ## Install tools required for development.

KUSTOMIZE = $(shell pwd)/bin/kustomize
.PHONY: kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@v4.5.7)

PROTOC = $(PROJECT_DIR)/protoc
.PHONY: protoc
protoc: ## Download protoc locally if necessary.
	# download and unzip protoc
	mkdir -p $(PROJECT_DIR)
	curl -qLO https://github.com/protocolbuffers/protobuf/releases/download/v3.19.4/protoc-3.19.4-linux-x86_64.zip
	unzip -q -o protoc-3.19.4-linux-x86_64.zip bin/protoc -d $(PROJECT_DIR)
	rm protoc-3.19.4-linux-x86_64.zip

# Find or download controller-gen
PROTOC_GEN_GO = $(GOBIN)/protoc-gen-go
.PHONY: protoc-gen-go
protoc-gen-go: ## Download controller-gen locally if necessary.
	$(call go-install-tool,$(PROTOC_GEN_GO),google.golang.org/protobuf/cmd/protoc-gen-go@v1.27.1)

PROTOC_GEN_GO_GRPC = $(GOBIN)/protoc-gen-go-grpc
.PHONY: protoc-gen-go-grpc
protoc-gen-go-grpc: ## Download controller-gen locally if necessary.
	$(call go-install-tool,$(PROTOC_GEN_GO_GRPC),google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2)

# Find or download controller-gen
CONTROLLER_GEN = $(GOBIN)/controller-gen
.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.17.0)

# Find or download gen-crd-api-reference-docs
GEN_CRD_API_REFERENCE_DOCS = $(GOBIN)/gen-crd-api-reference-docs
.PHONY: gen-crd-api-reference-docs
gen-crd-api-reference-docs:
	$(call go-install-tool,$(GEN_CRD_API_REFERENCE_DOCS),github.com/ahmetb/gen-crd-api-reference-docs@v0.3.0)

ENVTEST_ARCH ?= amd64

ENVTEST_ASSETS_DIR=$(shell pwd)/testbin
ENVTEST_KUBERNETES_VERSION?=latest
install-envtest: setup-envtest
	mkdir -p ${ENVTEST_ASSETS_DIR}
	$(ENVTEST) use $(ENVTEST_KUBERNETES_VERSION) --arch=$(ENVTEST_ARCH) --bin-dir=$(ENVTEST_ASSETS_DIR)

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST = $(shell pwd)/bin/setup-envtest
.PHONY: envtest
setup-envtest: ## Download envtest-setup locally if necessary.
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.16)

# go-install-tool will 'go install' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-install-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

.PHONY: release-manifests
release-manifests:
	rm -rf ./config/release || true
	mkdir ./config/release
	kustomize build ./config/crd > ./config/release/tofu-controller.crds.yaml
	kustomize build ./config/rbac > ./config/release/tofu-controller.rbac.yaml
	kustomize build ./config/manager > ./config/release/tofu-controller.deployment.yaml
	kustomize build ./config/package > ./config/release/tofu-controller.packages.yaml

# Helm
SRC_ROOT = $(shell git rev-parse --show-toplevel)

helm-docs: HELMDOCS_VERSION := v1.13.0
helm-docs: docker
	@docker run -v "$(SRC_ROOT):/helm-docs" jnorwood/helm-docs:$(HELMDOCS_VERSION) --chart-search-root /helm-docs

helm-lint: CT_VERSION := v3.3.1
helm-lint: docker
	@docker run -v "$(SRC_ROOT):/workdir" --entrypoint /bin/sh quay.io/helmpack/chart-testing:$(CT_VERSION) -c "cd /workdir; ct lint --config ct.yaml --lint-conf lintconf.yaml --all --debug"

docker:
	@hash docker 2>/dev/null || {\
		echo "You need docker" &&\
		exit 1;\
	}

.PHONY: serve-docs
serve-docs: ## Run a local server to serve the docs
	@docker run --rm -it -p 8000:8000 -v $(shell pwd):/docs squidfunk/mkdocs-material
