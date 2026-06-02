# Use Tofu Controller to configure plan-only options

Tofu Controller runs every OpenTofu/Terraform command through the
[`terraform-exec`](https://github.com/hashicorp/terraform-exec) library. That
library **rejects** the `TF_CLI_ARGS` and `TF_CLI_ARGS_*` environment variables,
so there is no way to inject extra plan arguments through the runner's
environment.

To support extra plan arguments in a typed, validated way, the `Terraform`
resource exposes an optional `.spec.plan` block. These options affect **only how
the plan runs**, not what it contains, so they never carry into the apply phase
— apply always runs lock-protected.

## Disabling the state lock for plans

The most common need is running `terraform plan -lock=false`. Without it, every
plan acquires the state lock, so plans against the same state serialise — most
painfully with the [Branch Planner](../branch-planner/index.md), where multiple
open pull requests would otherwise queue behind a single lock.

Set `.spec.plan.lock` to `false` to run plans without acquiring the lock:

```yaml hl_lines="11-12"
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  approvePlan: auto
  interval: 1m
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
  plan:
    lock: false
```

Leaving `lock` unset preserves Terraform's default (locking enabled). Only the
plan phase is affected; the subsequent apply still acquires the lock.

!!! tip "Branch Planner"
    The Branch Planner copies the source `Terraform` spec when it creates the
    per-pull-request objects, so `.spec.plan` is inherited automatically. Set
    `plan.lock: false` on your source `Terraform` resource to let parallel PR
    plans run concurrently.

## All plan options

| Field | terraform flag | Description |
|-------|----------------|-------------|
| `plan.lock` | `-lock=false` | Disable state locking for the plan. Leave unset to keep locking enabled. |

## What is not supported

Only options expressible through `terraform-exec` are available. Flags that the
library does not model — for example `-compact-warnings` — cannot be passed, and
`TF_CLI_ARGS*` environment variables remain blocked by the library.
