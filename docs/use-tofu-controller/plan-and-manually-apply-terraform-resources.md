# Use tofu-controller to plan and manually apply Terraform resources

Assume that you have a `GitRepository` object named `helloworld` pointing to a Git repository, and you want to plan and apply the Terraform resources under `./` of that Git repo. Let's walk through the steps of using tofu-controller to plan and
manually apply Terraform resources. 

- Create a `Terraform` object and set the necessary fields in the spec:
  - `approvePlan`, which sets the mode. For plan and manual approval mode, either keep this field blank or omit it entirely.
  - `interval`, which determines how often tofu-controller will run the Terraform configuration
  - `path`, which specifies the location of the configuration files, in this case `./`
  - `sourceRef`, which points to the `helloworld` GitRepository object
- Once this object is created, use kubectl to obtain the `approvePlan` value and set it in the `Terraform` object. 
- After making our changes and pushing them to the Git repository, tofu-controller will apply the plan and create the real resources.

Here is an example:

```yaml hl_lines="7"
apiVersion: infra.contrib.fluxcd.io/v1alpha2
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

## View the approval message

After a reconciliation loop, tofu-controller will generate a plan. Run this command to receive the `.spec.approvePlan` value from tofu-controller, which you'll need to approve the plan:

```bash
kubectl -n flux-system get tf/helloworld
```

This value enables you to edit the Terraform object file and set the `spec.approvePlan` field
to the value obtained from the message.

After making your changes and pushing them to the Git repository,
tofu-controller will apply the plan and create the real resources.

```yaml hl_lines="7"
apiVersion: infra.contrib.fluxcd.io/v1alpha2
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
