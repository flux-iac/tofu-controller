# Getting Started

## Preflight Checks

Here are the requirements you need to set up before you start:

  1. For Terraform Controller **v0.15+**, it requires **Flux v2.0** or later (not only the CLI, but also the controllers on the cluster). If you are not sure about the Flux version on your cluster, please re-bootstrap your cluster.
  2. For Terraform Controller v0.13 and v0.14, Flux 2 v0.32 - v0.41 (of course, not only the CLI, but also the controllers on the cluster).
  3. TF-controller uses **the Controller/Runner architecture**. The Controller acts as a client, and talks to each Runner's Pod via gRPC. Please make sure 
     1. **Each Runner's Pod in each Namespace** is allowed to open, and serve at **port 30000** (the gRPC port of a Runner), and the Controller can connect to it.
     2. **The Controller** needs to download tar.gz BLOBs from the **Source controller** via **port 80**.
     3. **The Controller** needs to post the events to the **Notification controller** via **port 80**.

## Installation

Before using TF-controller, you have to install Flux by using either `flux install` or `flux bootstrap` command.
Please note that TF-controller now requires **Flux v2.0** or later, so please make sure you have the latest version of Flux.
After that you can install TF-controller with Flux HelmRelease by:

```shell
kubectl apply -f https://raw.githubusercontent.com/weaveworks/tf-controller/main/docs/release.yaml
```

For the most recent release candidate of TF-controller, please use [rc.yaml](https://raw.githubusercontent.com/weaveworks/tf-controller/main/docs/rc.yaml).

```shell
kubectl apply -f https://raw.githubusercontent.com/weaveworks/tf-controller/main/docs/rc.yaml
```

### Installation on GKE

As of September 2023, GKE Autopilot clusters will use Cloud DNS for internal DNS resolution.
This means that the default DNS resolution method used by TF-controller will not work.
To use TF-controller on GKE Autopilot, you must set flag `--use-pod-subdomain-resolution=true` on the TF-controller deployment.
This flag can be set by adding the following to the TF-controller HelmRelease:

```yaml
spec:
  values:
    usePodSubdomainResolution: true
    runner:
      allowedNamespaces:
      - flux-system
      - dev-team
```

Enabling this value will cause TF-controller to use the Pod's subdomain for DNS resolution instead of the default Pod resolution method.
Pod's subdomain resolution requires a Service to be created for the Pod.
The HelmRelease above will create a Service named `tf-runner` in each namespace specified by the `runner.allowedNamespaces` value.

We have provided a HelmRelease to install TF-controller on GKE Autopilot with Pod's subdomain resolution enabled here.

```shell
kubectl apply -f https://raw.githubusercontent.com/weaveworks/tf-controller/main/docs/rc-gke.yaml
```

Tested with GKE Autopilot v1.27.3-gke.100.

### With Branch Planner

```shell
kubectl apply -f https://raw.githubusercontent.com/weaveworks/tf-controller/main/docs/branch-planner/release.yaml
```

For the most recent release candidate of TF-controller with Branch Planner, please use [branch-planner/rc.yaml](https://raw.githubusercontent.com/weaveworks/tf-controller/main/docs/branch-planner/rc.yaml).

```shell
kubectl apply -f https://raw.githubusercontent.com/weaveworks/tf-controller/main/docs/branch-planner/rc.yaml
```

For more details about the Branch Planner, please visit the
[Branch Planner documentation](./branch-planner/branch-planner-getting-started.md).

### Manual installation

With Helm by:

```shell
# Add tf-controller helm repository
helm repo add tf-controller https://weaveworks.github.io/tf-controller/

# Install tf-controller
helm upgrade -i tf-controller tf-controller/tf-controller \
    --namespace flux-system
```

For details on configurable parameters of the TF-controller chart,
please see [chart readme](https://github.com/flux-iac/tofu-controller/tree/main/charts/tf-controller#tf-controller-for-flux).

Alternatively, you can install TF-controller via `kubectl`:

```shell
export TF_CON_VER=v0.15.1
kubectl apply -f https://github.com/flux-iac/tofu-controller/releases/download/${TF_CON_VER}/tf-controller.crds.yaml
kubectl apply -f https://github.com/flux-iac/tofu-controller/releases/download/${TF_CON_VER}/tf-controller.rbac.yaml
kubectl apply -f https://github.com/flux-iac/tofu-controller/releases/download/${TF_CON_VER}/tf-controller.deployment.yaml
```

## Quick start

Here's a simple example of how to GitOps your Terraform resources with TF-controller and Flux.

### Define source

First, we need to define a Source controller's source (`GitRepository`, `Bucket`, `OCIRepository`), for example:

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: helloworld
  namespace: flux-system
spec:
  interval: 30s
  url: https://github.com/tf-controller/helloworld
  ref:
    branch: main
```

### The GitOps Automation mode

The GitOps automation mode could be enabled by setting `.spec.approvePlan=auto`. In this mode, Terraform resources will be planned,
and automatically applied for you.

```yaml
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  interval: 1m
  approvePlan: auto
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
```

For a full list of features and how to use them, please follow the [Use TF-controller](use-tf-controller/index.md) guide.

## Other Examples
  * A Terraform GitOps with Flux to automatically reconcile your [AWS IAM Policies](https://github.com/tf-controller/aws-iam-policies).
  * GitOps an existing EKS cluster, by partially import its nodegroup and manage it with TF-controller: [An EKS scaling example](https://github.com/tf-controller/eks-scaling).
