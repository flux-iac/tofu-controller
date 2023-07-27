# Getting Started With Branch Planner

If the Branch Planner is enabled through Helm values, it will
watch all configured Terraform resources, check their referenced Source, and
poll for Pull Requests using GitHub's API plus the provided token.

When the Branch Planner detects an open Pull Request, it either creates a new Terraform object or
updates an existing one, applying Plan Only mode based on the original
Terraform object.

When a Plan Output becomes available, the Branch Planner creates a new comment under
the Pull Request with the content of the Plan Output included.

## Prerequisites

1. Flux is installed on the cluster.
2. A GitHub [API token](./least-required-permissions.md).
3. Knowledge about GitOps Terraform Controller [(see docs)](https://weaveworks.github.io/tf-controller/).

## Configure Branch Planner

Branch Planner uses a ConfigMap as configuration. By default it's looking for a
`branch-planner` ConfigMap in the same namespace as where the `tf-controller` is
installed.

The ConfigMap has two fields:

1. `secretName`, which contains the API token to access GitHub.
2. `resources`, which defines a list of resources to watch.

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  namespace: flux-system
  name: branch-planner
data:
  secretName: branch-planner-token
  resources: |-
    - namespace: terraform
```

### Secret

Branch Planner uses the referenced Secret with a `token` field that acquires the
API token to fetch Pull Request information.

```bash
kubectl create secret generic branch-planner-token \
    --namespace=flux-system \
    --from-literal="token=${GITHUB_TOKEN}"
```

### Resources

If the `resources` list is empty, nothing will be watched. The resource definition
can be exact or namespace-wide.

With the following configuration file, the Branch Planner will watch all Terraform objects in
the `terraform` namespace, and the `exact-terraform-object` Terraform object in
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
