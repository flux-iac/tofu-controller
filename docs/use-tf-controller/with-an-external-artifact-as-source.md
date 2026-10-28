# Use Tofu Controller with an External Artifact as Source

An `ExternalArtifact` is a Flux source whose artifact is produced by a controller other than
source-controller, for example the [source-watcher](https://github.com/fluxcd/source-watcher)
`ArtifactGenerator`. It requires Flux **v2.7.0** or higher.

Tofu Controller only watches `ExternalArtifact` when the `ExternalArtifact` feature gate is
enabled, because the CRD does not exist on older Flux versions. Start the controller with
`--feature-gates=ExternalArtifact=true`, or set `featureGates.externalArtifact: true` in the
Helm chart values.

A common use is giving each Terraform object an artifact that contains only its own module,
so that a commit touching one module does not trigger a new plan for every Terraform object
reading from the same repository. The following `ArtifactGenerator` produces one
`ExternalArtifact` per module from a single `GitRepository`:

```yaml
---
apiVersion: source.extensions.fluxcd.io/v1beta1
kind: ArtifactGenerator
metadata:
  name: terraform-modules
  namespace: flux-system
spec:
  sources:
    - alias: repo
      kind: GitRepository
      name: infra
  artifacts:
    - name: networking
      originRevision: "@repo"
      copy:
        - from: "@repo/terraform/networking/**"
          to: "@artifact/"
    - name: dns
      originRevision: "@repo"
      copy:
        - from: "@repo/terraform/dns/**"
          to: "@artifact/"
```

Then use the generated `ExternalArtifact` as the `sourceRef` of your Terraform object.
The artifact's revision changes only when the copied files change, so the Terraform
object is re-planned only when its own module changes.

```yaml hl_lines="10-12"
---
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: networking
  namespace: flux-system
spec:
  path: ./
  interval: 1h
  sourceRef:
    kind: ExternalArtifact
    name: networking
  approvePlan: auto
```
