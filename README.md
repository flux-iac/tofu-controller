# tf-controller

`tf-controller` is an experimental controller for Flux to reconcile Terraform resources.

## Features
  
  * **Fully GitOps Automation for Terraform**: With setting `.spec.approvePlan=auto`, it allows a `Terraform` object 
   to be reconciled and act as the representation of your Terraform resources. The TF-controller uses the spec of 
   the `Terraform` object to perform `plan`, `apply` its associated Terraform resources. It then stores 
   the `TFSTATE` of the applied resources as a `Secret` inside the Kubernetes cluster. After `.spec.interval` passes, 
   the controller performs drift detection to check if there is a drift occurred between your live system, 
   and your Terraform resources. If a drift occurs, the plan to fix that drift will be generated and applied automatically. 
   _This feature is available since v0.3.0._
  * **Drift detection**: This feature is a part of the GitOps automation feature. The controller detects and fixes drift
   for your infrastructures, based on the Terraform resources and their `TFSTATE`. _This feature is available since v0.5.0._
  * **Plan and Manual Approve**: This feature allows you to separate the `plan`, out of the `apply` step, just like 
   the Terraform workflow you are familiar with. A good thing about this is that it is done in a GitOps way. When a plan 
   is generated, the controller shows you a message like **'set approvePlan: "plan-main-123" to apply this plan.'**. 
   You make change to the field `.spec.approvePlan`, commit and push to tell the TF-controller to apply the plan for you.
   With this GitOps workflow, you can optionally create and push this change to a new branch for your team member to 
   review and approve too. _This feature is available since v0.6.0._

## Dependencies

| Version  | Terraform | Source Controller | Flux v2 |
|:--------:|:---------:|:-----------------:|:-------:|
|**v0.6.0**| v1.1.3    | v0.20.1           | v0.25.x |
| v0.5.2   | v1.1.3    | v0.19.2           | v0.24.x |

## Quick start

Before using TF-controller, please install Flux by using either `flux install` or `flux bootstrap`.
Here's how to install TF-controller manually,

```shell script
export TF_CON_VER=v0.6.0
kubectl apply -f https://github.com/chanwit/tf-controller/releases/download/${TF_CON_VER}/tf-controller.crds.yaml
kubectl apply -f https://github.com/chanwit/tf-controller/releases/download/${TF_CON_VER}/tf-controller.rbac.yaml
kubectl apply -f https://github.com/chanwit/tf-controller/releases/download/${TF_CON_VER}/tf-controller.deployment.yaml
```

Here's a simple example of how to GitOps-ify your Terraform resources with `tf-controller` and Flux.

### Define source

```yaml
apiVersion: source.toolkit.fluxcd.io/v1beta1
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

### Auto-mode

```yaml
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  approvePlan: "auto"
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
```

### Plan and manual approval

```diff
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
- approvePlan: "auto"
+ approvePlan: "" # or you can omit this field 
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
```

then use field `approvePlan` to approve the plan so that it apply the plan to create real resources.

```diff
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: hello-world
  namespace: flux-system
spec:
- approvePlan: ""
+ approvePlan: "plan-main-b8e362c206" # first 8 digits of a commit hash is enough
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
```

## Examples
  * A Terraform GitOps with Flux to automatic reconcile your [AWS IAM Policies](https://github.com/tf-controller/aws-iam-policies). 

## Roadmap

### Q1 2022
  * Terraform outputs as Kubernetes Secrets
  * Secret and ConfigMap as input variables 
  * Support the GitOps way to "plan" / "re-plan" 
  * Support the GitOps way to "apply"
  * Drift detection
  * Support auto-apply so that the reconciliation detect drifts and always make changes
  * Test coverage reaching 70%

### Q2 2022  
  * Interop with Notification controller's Events and Alert   
  * Interop with Kustomization controller's health checks (via the Output resources)
  * Test coverage reaching 75%

### Q3 2022
  * Write back and show plan in PRs
  * Test coverage reaching 80%
