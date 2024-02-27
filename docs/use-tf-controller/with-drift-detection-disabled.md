# Use tofu-controller with drift detection disabled

Drift detection is enabled by default. You can set `.spec.disableDriftDetection: true` to disable it.

```yaml hl_lines="8"
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  approvePlan: auto
  disableDriftDetection: true
  interval: 1m
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
```
