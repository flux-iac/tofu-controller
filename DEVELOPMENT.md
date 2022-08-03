# Development

We follow the Flux development best practices.

> **Note:** Please take a look at <https://fluxcd.io/docs/contributing/flux/>
> to find out about how to contribute to Flux and how to interact with the
> Flux Development team.

## Code Reviews

Although you are a contributor with the write access to this repository,
please do not merge PRs by yourself. Please ask the project's [maintainers](MAINTAINERS)
to merge them for you after reviews.

## Protobuf Setup

TF-controller requires a specific version of Protobuf compiler and its Go plugins. 

* Protoc: version [3.19.4](https://github.com/protocolbuffers/protobuf/releases/download/v3.19.4/protoc-3.19.4-linux-x86_64.zip)
* Go plugin: `go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.27.1`
* Go plugin: `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2`

## How to run the test suite

Prerequisites:
* go = 1.17.x
* kubebuilder >= 2.3
* kustomize = 4.x
* kubectl >= 1.21

You can run the unit tests by simply doing

```bash
make test
```

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
export IMG=registry-path/tf-controller:latest
```

Build the container image, tagging it as `$IMG`:

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
