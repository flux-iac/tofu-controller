# Development

> **Note:** Please take a look at <https://fluxcd.io/docs/contributing/flux/>
> to find out about how to contribute to Flux and how to interact with the
> Flux Development team.

## How to run the test suite

Prerequisites:
* go >= 1.17
* kubebuilder >= 2.3
* kustomize >= 3.1
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