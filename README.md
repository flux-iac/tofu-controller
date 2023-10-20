# Weave GitOps' Terraform Controller

[![OpenSSF Best Practices](https://bestpractices.coreinfrastructure.org/projects/7761/badge)](https://bestpractices.coreinfrastructure.org/projects/7761)

Weave GitOps' **Terraform Controller** (aka Weave TF-Controller) is a controller for [Flux](https://fluxcd.io) to reconcile Terraform resources
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

## Get in touch

If you have a feature request to share or a bug to report, please file an issue. You can also reach out via our [TF-Controller Slack channel](https://app.slack.com/client/T2NDH1D9D/C054MR4UP88)—get there by first joining the [Weave Community Slack space](https://weave-community.slack.com).

## Quickstart and documentation

To get started check out this [guide](https://weaveworks.github.io/tf-controller/getting_started/) on how to GitOps your Terraform resources with TF-controller and Flux.

Check out the [documentation](https://weaveworks.github.io/tf-controller/) and [use cases](https://weaveworks.github.io/tf-controller/use_tf_controller/).

## Roadmap

### Q3 2023
  * [ ] Enhanced security (the lockdown mode)
  * [ ] Write back and show plan in PRs (Atlantis-like experience)
  * [ ] CLI to GitOpsify existing Terraform workflows (UX improvement for CLI) 
  * [ ] Type safety for custom backends

### Q4 2023
  * [ ] Improvement GitOps dependency management 
  * [ ] External drift detector
  * [ ] Cloud cost estimation 

### Q1 2024
  * [ ] Observability - logging from the different stages of the runner
  * [ ] `v1alpha3` API  
  * [ ] Azure package for TF-controller (e.g. AKS, CosmosDB, etc.)
  * [ ] GCP package for TF-controller (e.g. GKE, CloudSQL, etc.) 
  * [ ] ARM64 & Gravitron support

### Q2 2024
  * [ ] `v1beta1` API (stabilization)

### Q3 2024
  * [ ] `v1beta2` API
