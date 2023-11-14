# Use TF-controller with a plan-only mode

This plan-only mode is designed to be used in conjunction with the [Branch Planner](../branch_planner/index.md).
But you can also use it in a circumstance where you want to run `terraform plan` only.

If `planOnly` is set to `true`, the controller will skip the apply part and runs
only `terraform plan` and saves the output.

```
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  interval: 1m
  planOnly: true
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
```
