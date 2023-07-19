# Getting Started With Branch Planner

If the Branch Planner is enabled through Helm values, the Branch Planner will
watch all configured Terraform resources, check their referenced Source, and
polls for Pull Requests using GitHub's API with the provided token.

When an open Pull Request is detected, the Branch Planner creates a new or
updates an existing Terraform object with Plan Only mode from the original
Terraform object.

When a Plan Output is available, the Branch Planner creates a new comment under
the Pull Request with the content of the Plan Output.

## Pre-requirements

1. Flux is installed on the cluster.
2. GitHub [API token](./least-required-permissions.md).
3. Knowledge about GitOps Terraform Controller.

## Configure Branch Planner

Branch Planner uses a ConfigMap as configuration. By default it's looking for a
`barnch-planner` ConfigMap in the same namespace as the `tf-controller` is
installed.

The ConfigMap has two fields:

1. `secretName` that contains the API token to access GitHub.
2. `resources` that defined a list of resources to watch.

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: branch-planner
data:
  secretName: branch-planner-token
  resources: |-
    - namespace: terraform
```

### Secret

Branch Planner uses the referenced Secret with a `token` field to acquire the
API token to fetch Pull Request information.

```bash
kubectl create secret generic branch-planner-token \
    --namespace=flux-system \
    --from-literal="token=${GITHUB_TOKEN}"
```

### Resources

If the `resources` list is empty, nothing will be watched. Resource definition
can be exact or namespace-wide.

With the following configuration file, all Terraform objects will be watched in
the `terraform` namespace, and `exact-terraform-object` Terraform object in
`default` namespace.

```yaml
data:
  resources:
    - namespace: default
      name: exact-terraform-object
    - namespace: terraform
```

## Enable Branch Planner

To enable branch planner, set the `branchPlanner.enabled` to `true` in the Helm
values files.

```
---
branchPlanner:
  enabled: true
```
