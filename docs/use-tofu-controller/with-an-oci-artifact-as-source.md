# Use tofu-controller with an OCI Artifact as Source

To use OCI artifacts as the source of Terraform objects, you need Flux 2 version **v0.32.0** or higher.

Assuming that you have Terraform files (your root module may contain sub-modules) under ./modules,
you can use Flux CLI to create an OCI artifact for your Terraform modules
by running the following commands:

```bash
flux push artifact oci://ghcr.io/flux-iac/helloworld:$(git rev-parse --short HEAD) \
    --path="./modules" \
    --source="$(git config --get remote.origin.url)" \
    --revision="$(git branch --show-current)/$(git rev-parse HEAD)"

flux tag artifact oci://ghcr.io/flux-iac/helloworld:$(git rev-parse --short HEAD) \
    --tag main
```

Then you define a source (`OCIRepository`), and use it as the `sourceRef` of your Terraform object.

```yaml hl_lines="5 20-22"
---
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: OCIRepository
metadata:
  name: helloworld-oci
spec:
  interval: 1m
  url: oci://ghcr.io/flux-iac/helloworld
  ref:
    tag: main
---
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: helloworld-tf-oci
spec:
  path: ./
  approvePlan: auto
  interval: 1m
  sourceRef:
    kind: OCIRepository
    name: helloworld-oci
  writeOutputsToSecret:
    name: helloworld-outputs
```
