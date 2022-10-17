# Use TF-controller to plan and manually apply Terraform resources

Assume that you have a `GitRepository` object named `helloworld` pointing to a Git repository, and you want to plan and apply the Terraform resources under `./` of that Git repo.

For the plan & manual approval workflow, please start by either setting `.spec.approvePlan` to be the blank value, or omitting the field.

```yaml hl_lines="7"
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  approvePlan: "" # or you can omit this field
  interval: 1m
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
```

Then after a reconciliation loop, the controller will generate a plan, and tell you how to use field `.spec.approvePlan` to approve the plan.
You can run the following command to obtain that message.

```bash
kubectl -n flux-system get tf/helloworld
```

After making change and push, it will apply the plan to create real resources.

```yaml hl_lines="7"
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: hello-world
  namespace: flux-system
spec:
  approvePlan: plan-main-b8e362c206 # first 8 digits of a commit hash is enough
  interval: 1m
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
```
