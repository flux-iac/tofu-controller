## Use tofu-controller to detect drifts only without plan or apply

We can set `.spec.approvePlan` to `disable` to tell the controller to detect drifts of your Terraform resources only. Doing so will skip the `plan` and `apply` stages.

```yaml hl_lines="7"
apiVersion: infra.contrib.fluxcd.io/v1alpha2
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

## Troubleshooting

### When Terraform resource detects drift, but no plan is generated for approval

In this situation, you may not have `spec.approvePlan` set to `disable`. Try setting `spec.approvePlan: auto` and using `tfctl replan` to trigger a replan.
After the drift disappears, you can set the `spec.approvePlan: ""` to get into the manual mode again.
