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