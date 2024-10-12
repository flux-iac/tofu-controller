# Tofu Controller: An IAC Controller for Flux

[![OpenSSF Best Practices](https://bestpractices.coreinfrastructure.org/projects/7761/badge)](https://bestpractices.coreinfrastructure.org/projects/7761)

Tofu Controller (previously known as Weave TF-Controller) is a controller for [Flux](https://fluxcd.io) to reconcile OpenTofu and Terraform resources
in the GitOps way.
With the power of Flux together with OpenTofu and Terraform, Tofu Controller allows you to GitOps-ify infrastructure,
and application resources, in the Kubernetes and IAC universe, at your own pace.

"At your own pace" means you don't need to GitOps-ify everything at once.

Tofu Controller offers many GitOps models:
  1. **GitOps Automation Model:** GitOps your OpenTofu and Terraform resources from the provision steps to the enforcement steps, like a whole EKS cluster.
  2. **Hybrid GitOps Automation Model:** GitOps parts of your existing infrastructure resources. For example, you have an existing EKS cluster.
     You can choose to GitOps only its nodegroup, or its security group.
  3. **State Enforcement Model:** You have a TFSTATE file, and you'd like to use GitOps enforce it, without changing anything else.
  4. **Drift Detection Model:** You have a TFSTATE file, and you'd like to use GitOps just for drift detection, so you can decide to do things later when a drift occurs.

## Get in touch

If you have a feature request to share or a bug to report, please file an issue. You can also reach out via our [Tofu Controller Slack channel](https://weave-community.slack.com/archives/C054MR4UP88) — get there by first joining the [Weave Community Slack space](https://weave-community.slack.com).

## Quickstart and documentation

To get started check out this [guide](https://flux-iac.github.io/tofu-controller/getting_started/) on how to GitOps your Terraform resources with Tofu Controller and Flux.

Check out the [documentation](https://flux-iac.github.io/tofu-controller/) and [use cases](https://flux-iac.github.io/tofu-controller/use-tofu-controller/).

## Roadmap

### Q2 2024
  * [ ] Write back and show plan in PRs (Atlantis-like experience)
  * [ ] CLI to GitOpsify existing Terraform workflows (UX improvement for CLI) 
  * [ ] Type safety for custom backends

### Q3 2024
  * [ ] Improvement GitOps dependency management 
  * [ ] External drift detector
  * [ ] Cloud cost estimation 

### Q4 2024
  * [ ] Observability - logging from the different stages of the runner
  * [ ] `v1alpha3` API  
  * [ ] ARM64 & Gravitron support

### Q1 2025
  * [ ] `v1beta1` API (stabilization)

### Q2 2025
  * [ ] `v1beta2` API
