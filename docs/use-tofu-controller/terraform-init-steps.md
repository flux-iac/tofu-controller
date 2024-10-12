## Aligning tofu-controller with Terraform's Init Workflow Stage

This page covers required and optional steps you should take in alignment with Terraform's "init" workflow stage. We cover "plan," "apply," and "destroy" steps in subsequent pages. 

### Define Source

First, we need to define the Source controller's source (`GitRepository`, `Bucket`, `OCIRepository`). For example:

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: helloworld
  namespace: flux-system
spec:
  interval: 30s
  url: https://github.com/flux-iac/helloworld
  ref:
    branch: main
```

Here's guidance for [when your source is an OCI artifact](with-an-oci-artifact-as-source.md).

### Optional Steps 

At this point you have options to enhance your use of tofu-controller:
- Optional: [Use tofu-controller with GitOps Dependency Management](with-gitops-dependency-management.md)
    - This is to avoid the Kustomization controller's variable substitution
- Optional: [Using tofu-controller with Primitive Modules](with-primitive-modules.md) for an optional way to write Terraform code.

### Resource Provisioning

Related resources, with optional steps noted:

- [Use tofu-controller to Provision Resources and Auto-Approve](provision-resources-and-auto-approve.md)
- Optional: [Provision Resources and Destroy Them When the Terraform Object Gets Deleted](provision-resources-and-destroy-them-when-terraform-object-gets-deleted.md)
- Optional: [Provision Terraform Resources That Are Required Health Checks](provision-Terraform-resources-that-are-required-health-checks.md)
    - You would check these during the "apply" workflow stage
- Optional, operations-related: [Using a Custom Backend](with-a-custom-backend.md)
    - tofu-controller uses the Kubernetes backend by default

Be mindful of locking mechanism when pursuing these steps.

### Optional: Working with Integrations
- [Working with Terraform Cloud and Terraform Enterprise](integration-with-terraform-enterprise-or-cloud.md); see also: [Terraform Cloud and Branch Planner](../branch-planner/branch-planner-tfc-integration-getting-started.md)

### Context-Related Steps
- Optional: [Use tofu-controller with Terraform Runners enabled via Env Variables](with-tf-runner-logging.md)
- Optional: [Set variables for Terraform resources](set-variables-for-terraform-resources.md)
- Optional: [Provision resources and obtain outputs](provision-resources-obtain-outputs.md)
- [Provision resources with customized Runner Pods](provision-resources-with-customized-runner-pods.md)
- Optional: [Use with external webhooks](with-external-webhooks.md)
