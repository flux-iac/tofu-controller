# Use TF-controller to plan and manually apply Terraform resources

In this guide, we will walk through the steps of using TF-controller to plan and
manually apply Terraform resources.

We will start by creating the `Terraform` object and specifying the necessary fields,
including the `approvePlan` field.

We will then create the `GitRepository` object,
which points to the Git repository containing the Terraform configuration.

Once these objects are created, we will use kubectl to obtain the `approvePlan` value
and set it in the `Terraform` object. After making our changes and pushing them to the Git repository,
TF-controller will apply the plan and create the real resources.

## Define the Terraform object

Assume that you have a `GitRepository` object named `helloworld` pointing to a Git repository, and you want to plan and apply the Terraform resources under `./` of that Git repo.

For the plan & manual approval workflow, please start by either setting `.spec.approvePlan` to be the blank value, or omitting the field. This will tell TF-controller to use the plan & manual approval workflow, rather than the auto-apply workflow.
If you want to use the auto-apply workflow, you will need to set the `spec.approvePlan` field to "auto".

In addition to setting the `spec.approvePlan` field, you will also need to specify the `interval`, `path`,
and `sourceRef` fields in the spec field.
The `interval` field determines how often TF-controller will run the Terraform configuration,
the `path` field specifies the location of the configuration files,
and the `sourceRef` field points to the GitRepository object.

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

## View the approval message

Then after a reconciliation loop, the controller will generate a plan, and tell you how to use field `.spec.approvePlan` to approve the plan.
You can run the following command to obtain that message.

```bash
kubectl -n flux-system get tf/helloworld
```

This command will output the message containing the approvePlan value
that you will need to use to approve the plan.
Once you have this value, you can edit the Terraform object file, and set the `spec.approvePlan` field
to the value obtained from the message.

After making your changes and pushing them to the Git repository,
TF-controller will apply the plan and create the real resources.
This process is known as the plan & manual approval workflow,
as it involves generating a plan and requiring manual approval before the changes are applied.

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