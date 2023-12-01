# Getting Started With Branch Planner

## Prerequisites

1. Flux is installed on the cluster.
2. A GitHub API token. For public repositories, it's sufficient to enable `Public Repositories` without
any additional permissions. For private repositories, you need the following permissions:
  - `Pull requests` with Read-Write access. This is required to check Pull Request
  changes, list comments, and create or update comments.
  - `Metadata` with Read-only access. This is automatically marked as "mandatory"
  because of the permissions listed above.
3. General knowledge about TF-Controller [(see docs)](https://weaveworks.github.io/tf-controller/).

## Quick Start

This section describes how to install Branch Planner using a HelmRelease object in the `flux-system` namespace with minimum configuration on a KinD cluster.

1. Create a KinD cluster.
```
kind create cluster
```

2. Install Flux. Make sure you have the latest version of Flux (v2 GA).

```
flux install
```

3. Create a secret that contains a GitHub API token. If you do not use the `gh` CLI, copy and paste the token from GitHub's website.

```
export GITHUB_TOKEN=$(gh auth token)

kubectl create secret generic branch-planner-token \
    --namespace=flux-system \
    --from-literal="token=${GITHUB_TOKEN}"
```

4. Install Branch Planner from a HelmRelease provided by the TF-Controller repository. Use TF-Controller v0.16.0-rc.2 or later.

```
kubectl apply -f https://raw.githubusercontent.com/weaveworks/tf-controller/fa4b3b85d316340d897fda4fed757265ba2cd30e/docs/branch_planner/release.yaml
```

5. Create a Terraform object with a Source pointing to a repository. Your repository must contain a Terraform file—for example, `main.tf`. Check out [this demo](https://github.com/tf-controller/branch-planner-demo) for an example.

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

6. Now you can create a pull request on your GitHub repo. The Branch Planner will create a new Terraform object with the plan-only mode enabled and will generate a new plan for you. It will post the plan as a new comment in the pull request.

## Configure Branch Planner

Branch Planner uses a ConfigMap as configuration. The ConfigMap is optional but useful for fine-tuning Branch Planner.

### Configuration

By default, Branch Planner will look for the `branch-planner` ConfigMap in the same namespace as where the TF-Controller is installed.
That ConfigMap allows users to specify which Terraform resources in a cluster the Brach Planner should monitor.

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
    - namespace: flux-system
```

#### Secret

Branch Planner uses the referenced Secret with a `token` field that acquires the
API token to fetch pull request information.

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

If no ConfigMap is found, the Branch Planner will not watch any namespaces for Terraform resources and look for a GitHub token in a secret named `branch-planner-token` in the `flux-system` namespace. Supplying a secret with a token is a necessary task, otherwise Branch Planner will not be able to interact with the GitHub API.

## Enable Branch Planner

To enable branch planner, set the `branchPlanner.enabled` to `true` in the Helm
values files.

```
---
branchPlanner:
  enabled: true
```
