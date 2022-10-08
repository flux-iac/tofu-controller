## Use TF-controller to detect drifts only without plan or apply

We can set `.spec.approvePlan` to `disable` to tell the controller to detect drifts of your Terraform resources only. Doing so will skip the `plan` and `apply` stages.

```yaml hl_lines="7"
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: hello-world
  namespace: flux-system
spec:
  approvePlan: disable
  interval: 1m
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
```
