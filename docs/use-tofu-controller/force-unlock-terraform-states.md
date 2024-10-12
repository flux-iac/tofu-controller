# Use tofu-controller to force unlock Terraform states

In some situations, you may need to perform the Terraform [force-unlock](https://www.terraform.io/language/state/locking#force-unlock) operation on the tfstate inside the cluster. 

There are three possible values of `.spec.tfstate.forceUnlock`, which are `yes`, `no`, and `auto`.
The default value is `no`, which means that you disable this behaviour.

The `auto` force-unlock mode will automatically use the lock identifier produced by the associated state file instead of the specified lock identifier.

The recommended way is to do manual force unlock. To manually `force-unlock`, you need to:

  1. set `forceUnlock` to `yes`, and
  2. specify a lock identifier to unlock a specific locked state,

as the following example:

```yaml hl_lines="14-16"
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
  tfstate:
    forceUnlock: "yes"
    lockIdentifier: f2ab685b-f84d-ac0b-a125-378a22877e8d
```
