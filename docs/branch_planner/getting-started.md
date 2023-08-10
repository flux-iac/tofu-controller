# Getting Started With Branch Planner

When the Branch Planner is enabled through Helm values, it will watch all configured Terraform resources, check their referenced Source, and poll for Pull Requests using GitHub's API plus the provided token.

When the Branch Planner detects an open Pull Request, it either creates a new Terraform object or updates an existing one, applying Plan Only mode based on the original
Terraform object.

When a Plan Output becomes available, the Branch Planner creates a new comment under the Pull Request with the content of the Plan Output included.

## Prerequisites

1. Flux is installed on the cluster.
2. A GitHub [API token](./least-required-permissions.md).
3. Knowledge about GitOps Terraform Controller [(see docs)](https://weaveworks.github.io/tf-controller/).

## Quick Start Guide

This section describe how to install Branch Planner using HelmRelease object in the `flux-system` namespace with minimum configuration on a KinD cluster.

1. Create a KinD cluster.
```
kind create cluster
```

2. Install Flux. Please make sure you have the latest version of Flux (v2 GA).
```
flux install
```

3. Create a Secret that contains GitHub API token. If you do not use `gh` cli, please feel free to copy and paste the token from GitHub's website.
```
export GITHUB_TOKEN=$(gh auth token)

kubectl create secret generic branch-planner-token \
    --namespace=flux-system \
    --from-literal="token=${GITHUB_TOKEN}"
```

4. Install Branch Planner from a HelmRelease provided by the TF-controller repository. Please make sure that you use TF Controller v0.16.0-rc.2 or later.
```
kubectl apply -f https://raw.githubusercontent.com/weaveworks/tf-controller/main/docs/branch_planner/release.yaml
```

5. Create a Terraform object with a Source pointing to a repository.
You repository must contain a Terraform file, for example `main.tf`.
Please take a look at [https://github.com/tf-controller/branch-planner-demo](https://github.com/tf-controller/branch-planner-demo) for an example.
```bash
export GITHUB_USER=<your user>
export GITHUB_REPO=<your repo>

cat <<EOF | kubectl apply -f -
---
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: branch-planner-demo
  namespace: flux-system
spec:
  interval: 30s
  url: https://github.com/${GITHUB_USER}/${GITHUB_REPO}
  ref:
    branch: main
---
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: branch-planner-demo
  namespace: flux-system
spec:
  approvePlan: auto
  path: ./
  interval: 1m
  sourceRef:
    kind: GitRepository
    name: branch-planner-demo
    namespace: flux-system
EOF
```
6. Now you can go to your GitHub repo and create a Pull Request. The Branch Planner will create a new Terraform object with Plan Only mode enabled, and generate a new plan for you.

## Configure Branch Planner

Branch Planner uses a ConfigMap as configuration. That ConfigMap is optional to use but useful for fine-tuning Branch Planner.

### Custom Configuration

By default Branch Planner will look for a `branch-planner` ConfigMap in the same namespace as where the `tf-controller` is installed. That ConfigMap allows users to precisely specify which Terraform resources in a cluster should be monitored by Branch Planner.

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

#### Secret

Branch Planner uses the referenced Secret with a `token` field that acquires the
API token to fetch Pull Request information.

```bash
kubectl create secret generic branch-planner-token \
    --namespace=flux-system \
    --from-literal="token=${GITHUB_TOKEN}"
```

#### Resources

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

### Default Configuration

If a ConfigMap is not found, it will watch the `flux-system` namespace for any Terraform resources and expect to find a GitHub token in a secret named `branch-planner-token` in the `flux-system` namespace. Note that supplying a secret with a token is a necessary task, otherwise Branch Planner will not be able to interact with the GitHub API. 

## Enable Branch Planner

To enable branch planner, set the `branchPlanner.enabled` to `true` in the Helm
values files.

```
---
branchPlanner:
  enabled: true
```
