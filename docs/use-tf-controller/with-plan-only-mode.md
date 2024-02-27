# Use tofu-controller with a plan-only mode

This plan-only mode is designed to be used in conjunction with the [Branch Planner](../branch-planner/index.md).
But you can also use it whenever you want to run `terraform plan` only.

If `planOnly` is set to `true`, tofu-controller will skip the `apply` step, run
`terraform plan`, and save the output.

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
