# tf-controller

`tf-controller` is an experimental controller for Flux to reconcile Terraform resources.

## Features
  
  * **Fully GitOps Automation for Terraform**: With setting `.spec.approvePlan=true`, it allows the `Terraform` object 
   to fully perform GitOps reconciliation for your Terraform resources. The controller performs `plan`, `apply`, and stores 
   the `TFSTATE` of the applied resources as a Secret inside the cluster. Then, after `.spec.interval` passes, 
   the controller performs drift detection to check if there is a drift occurred between the live system, 
   and your Terraform resources. If a drift happens, the plan to fix that drift will be created and applied automatically. 
   This feature is available since v0.3.0. 
  * **Drift detection**: This feature is a part of the GitOps automation feature. The controller detects and fixes drift
   for your infrastructures, based on the Terraform resources and their `TFSTATE`. This feature is available since v0.5.0.  

## Dependencies

| Version | Terraform | Source Controller | Flux v2 |
|:-------:|:---------:|:-----------------:|:-------:|
| v0.5.0  | v1.1.3    | v0.19.2           | v0.24.x |

## Quick start

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
