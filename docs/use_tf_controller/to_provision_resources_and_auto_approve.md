# Use TF-controller to provision resources and auto approve

To provision resources with TF-controller, you need to create a `Terraform` object and a Flux source object, 
such as a `GitRepository` or `OCIRepository` object.

## Create a Terraform object

The `Terraform` object is a Kubernetes custom resource definition (CRD) object.
It is the core object of TF-controller. 

It defines the Terraform module, the backend configuration, and the GitOps automation mode.

The Terraform module is a Terraform configuration that can be used to provision resources.
It can be placed inside a Git repository, or packaged as an OCI image in an OCI registry.

The backend configuration is the configuration for the Terraform backend to be used to store the Terraform state.
It is optional. If not specified, the Kubernetes backend will be used by default.

## GitOps automation mode

the GitOps automation mode is the GitOps automation mode to be used to run the Terraform module.
It is optional. If not specified, the "plan-and-manually-apply" mode will be used by default.
In this example, we use the "auto-apply" mode.

The following is an example of a `Terraform` object:

```yaml hl_lines="8"
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: helloworld
spec:
  path: ./helloworld
  interval: 10m
  approvePlan: auto
  sourceRef:
    kind: GitRepository
    name: helloworld
```
