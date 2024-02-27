# Development

We follow the Flux development best practices.

> **Note:** Please take a look at <https://fluxcd.io/docs/contributing/flux/>
> to find out about how to contribute to Flux and how to interact with the
> Flux Development team.

## Developer Certificate of Origin

By submitting any contributions to this repository as an individual or on behalf of a corporation, you agree to the [Developer Certificate of Origin](DCO).

## Code Reviews

Although you are a contributor with the write access to this repository,
please do not merge PRs by yourself. Please ask the project's [maintainers](MAINTAINERS)
to merge them for you after reviews.

## Protobuf Setup

TF-controller requires a specific version of Protobuf compiler and its Go plugins.

* Protoc: version [3.19.4](https://github.com/protocolbuffers/protobuf/releases/download/v3.19.4/protoc-3.19.4-linux-x86_64.zip)
* Go plugin: `go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.27.1`
* Go plugin: `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2`

These can be installed by running `make tools`.

## How to run the test suite

Prerequisites:

* go = 1.20.x
* kubebuilder = 3.6.x
* controller-gen = 0.8.x
* kustomize = 4.x
* kubectl >= 1.21

You can run the unit tests by simply doing

```bash
make test
```

If you get an error stating "etcd" not found then run:

```bash
make install-envtest
```

and then retry `make test`

## GRPC

Any changes to the [runner.pb.go](./runner/runner.pb.go) file will require you to regenerate the necessary proto files.

To do so, first run:

```bash
make protoc-gen-go-grpc
```

to install the library and then:

```bash
make gen-grpc
```

to update the the generated `runner.pg.go` data.

## How to run the controller locally

Install flux on your test cluster:

```bash
flux install
```

Port forward to source-controller artifacts server:

```bash
kubectl -n flux-system port-forward svc/source-controller 8080:80
```

Export the local address as `SOURCE_CONTROLLER_LOCALHOST`:

```bash
export SOURCE_CONTROLLER_LOCALHOST=localhost:8080
```

Export Kubernetes service and port of the test cluster:

```bash
export KUBERNETES_SERVICE_HOST=
export KUBERNETES_SERVICE_PORT=
```

Disable Terraform Kubernetes backend so that it doesn't store the state:

```bash
export DISABLE_TF_K8S_BACKEND=1
```

Run the controller locally:

```bash
make install
make run
```

## How to install the controller

### Building the container image

Set the name of the container image to be created from the source code. This will be used when building, pushing and referring to the image on YAML files:

```sh
export MANAGER_IMG=registry-path/tf-controller
```

Build the container image, tagging it as `$MANAGER_IMG:latest`:

```sh
make docker-build
```

Push the image into the repository:

```sh
make docker-push
```

**Note**: `make docker-build` will build an image for the `amd64` architecture.

### Deploying into a cluster

Deploy `tf-controller` into the cluster that is configured in the local kubeconfig file (i.e. `~/.kube/config`):

```sh
make deploy
```

Running the above will also deploy `source-controller` and its CRDs to the cluster.

### Debug

`sudo dlv --listen=:2345 --headless=true --api-version=2 attach $(pgrep tf-controller)`

## Communications

For realtime communications we use Slack: To join the conversation, simply join the [Weave Users](https://weave-community.slack.com/) Slack workspace and use the [#tf-controller](https://weave-community.slack.com/messages/tf-controller/) channel.

To discuss ideas and specifications we use [Github Discussions](https://github.com/flux-iac/tofu-controller/discussions).

## Acceptance policy

These things will make a PR more likely to be accepted:

* a well-described requirement
* tests for new code
* tests for old code!
* new code and tests follow the conventions in old code and tests
* a good commit message (see below)
* all code must abide [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
* names should abide [What's in a name](https://talks.golang.org/2014/names.slide#1)
* code must build on both Linux and Darwin, via plain `go build`
* code should have appropriate test coverage and tests should be written
  to work with `go test`

In general, we will merge a PR once one maintainer has endorsed it.
For substantial changes, more people may become involved, and you might
get asked to resubmit the PR or divide the changes into more than one PR.

### Format of the Commit Message

We prefer the following rules for good commit messages:

* Limit the subject to 50 characters and write as the continuation
  of the sentence "If applied, this commit will ..."
* Explain what and why in the body, if more than a trivial change;
  wrap it at 72 characters.

The [following article](https://chris.beams.io/posts/git-commit/#seven-rules)
has some more helpful advice on documenting your work.
