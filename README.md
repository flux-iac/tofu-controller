# Weave GitOps Terraform Controller

**Weave GitOps Terraform Controller** (aka Weave TF-controller) is a controller for [Flux](https://fluxcd.io) to reconcile Terraform resources
in the GitOps way.
With the power of Flux together with Terraform, TF-controller allows you to GitOps-ify infrastructure,
and application resources, in the Kubernetes and Terraform universe, at your own pace.

"At your own pace" means you don't need to GitOps-ify everything at once.

TF-controller offers many GitOps models:
  1. **GitOps Automation Model:** GitOps your Terraform resources from the provision steps to the enforcement steps, like a whole EKS cluster.
  2. **Hybrid GitOps Automation Model:** GitOps parts of your existing infrastructure resources. For example, you have an existing EKS cluster.
     You can choose to GitOps only its nodegroup, or its security group.
  3. **State Enforcement Model:** You have a TFSTATE file, and you'd like to use GitOps enforce it, without changing anything else.
  4. **Drift Detection Model:** You have a TFSTATE file, and you'd like to use GitOps just for drift detection, so you can decide to do things later when a drift occurs.

## Quickstart and documentation

To get started check out this [guide](https://weaveworks.github.io/tf-controller/getting_started/) on how to GitOps your Terraform resources with TF-controller and Flux.

Check out the [documentation](https://weaveworks.github.io/tf-controller/) for a list of [features](https://weaveworks.github.io/tf-controller/#features) and [use cases](https://weaveworks.github.io/tf-controller/use_cases/).

## Roadmap

### Q3 2022
  * [x] Performance
  * [x] Scalability
  * [x] Interop with Notification controller's Events and Alert
  * [x] Validation webhooks for the planning stage

### Q4 2022
  * [x]  ~~CRD wrappers~~ A new component model for Terraform modules
  * [ ] AWS package for TF-controller (e.g. EKS, RDS, etc.)
  * [ ] Dependency management for TF-controller objects
  * [ ] CLI for Weave GitOps Enterprise

### Q1 2023
  * [ ] CLI to GitOpsify existing Terraform workflows
  * [ ] Selectable Terraform versions
  * [ ] Cloud cost estimation
  * [ ] Write back and show plan in PRs (Atlantis-like experience)

### Q2 2023
  * [ ] Enhanced security (the lockdown mode)
  * [ ] External drift detector
  * [ ] Type safety for custom backends
  * [ ] `v1alpha2` API

### Q3 2023
  * [ ] ARM64 & Gravitron support
  * [ ] Containerd compatibility

### Q4 2023
  * [ ] `v1beta1` API (stabilization)
