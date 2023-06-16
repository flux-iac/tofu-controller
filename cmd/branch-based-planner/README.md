# Branch-based planner development

## How to run this

You can run this with

    go run ./cmd/branch-based-planner/

but it won't do much without a configuration. The configuration is in
the form of a ConfigMap in your Kubernetes cluster (as accessed by
`current-config` in your kubeconfig).

# Set up a Terraform and GitRepository

You can use my "helloworld" repository itself for this for now, since
it is public, has a valid Terraform program in it, and has an open PR.

Create a GitRepository object and a Terraform object representing this:

```bash
kubectl apply -f- <<EOF
---
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: helloworld
  namespace: default
spec:
  interval: 30s
  url: https://github.com/squaremo/tf-controller-helloworld
  ref:
    branch: main
---
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: helloworld-tf
  namespace: default
spec:
  path: ./
  interval: 1m
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: default
EOF
```

## Create a suitable secret

The secret needs to contain a field "token" with a personal access
token. It needs "read" permission to the repository or repositories in
question, and a [fine-grained
token](https://github.com/settings/tokens?type=beta) will work for
that. I used "Public read-only" rather than specifying individual
repos.

Assuming you have put the token in an environment variable `GITHUB_TOKEN`:

```bash
kubectl create secret generic bbp-token -n default --from-literal=token=$GITHUB_TOKEN
```

## Create a config

The configuration given in a ConfigMap in a form specified in
[internal/server/polling/config.go][].

Note the `resources` field is a string value (`|` in the example below
indicates a multiline string), with internal structure.

This will create a `ConfigMap` that works with the `GitRepository` and
`Terraform` object above:

```bash
kubectl apply -f- <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: branch-based-planner
  namespace: default
data:
  secretName: bbp-token
  resources: |
    - namespace: default
      name: helloworld-tf
EOF
```

### Targeting a different Kubernetes cluster

Supply the env entry `KUBECONFIG` to use a different kubeconfig; it
will still use `current-config`, but you can arrange for that to point
to the intended cluster.
