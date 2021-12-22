# tf-controller

A Terraform controller for Flux

## Roadmap

### Q1 2022
  * Terraform outputs as Kubernetes Secrets
  * Secret and ConfigMap as input variables 
  * Support the GitOps way to "plan" / "re-plan" 
  * Support the GitOps way to "apply"
  
### Q2 2022  
   
  * Interop with Kustomization controller's health checks (via Output)
  * Interop with Notification controller's Events and Alert

### Q3 2022
  * Write back and show plan in PRs
  * Support auto-apply so that the reconciliation detect drifts and always make changes
