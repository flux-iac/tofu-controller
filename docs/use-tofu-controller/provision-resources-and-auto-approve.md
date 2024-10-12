# Use tofu-controller to provision resources and auto approve

To provision resources with tofu-controller, you need to create a `Terraform` object and a Flux source object, 
such as a `GitRepository` or `OCIRepository` object.

## Create a Terraform object

The `Terraform` object is a Kubernetes custom resource definition (CRD) object.
It is the core object of tofu-controller and defines
the Terraform module, backend configuration, and GitOps automation mode.

The Terraform module is a Terraform configuration that you can use to provision resources.
It can either be placed inside a Git repository, or packaged as an OCI image in an OCI registry.

The backend configuration is the configuration for the Terraform backend to be used to store the Terraform state.
It is optional. If not specified, the Kubernetes backend will be used by default.

## GitOps Automation mode

Use the GitOps automation mode to run the Terraform module. It determines how Terraform runs and manages your infrastructure. It is optional. If not specified, the "plan-and-manually-apply" mode is used by default.
In the "plan-and-manually-apply" mode,
tofu-controller will run a Terraform plan and output the proposed changes to a Git repository.
A human must then review and manually apply the changes.

In the "auto-apply" mode, tofu-controller will automatically apply the changes after a Terraform plan is run.
This can be useful for environments where changes can be made automatically,
but it is important to ensure that the proper controls, like policies, are in place to prevent unintended changes
from being applied.

To specify the GitOps automation mode in a Terraform object, set the `spec.approvePlan` field to the desired value. For example, to use the "auto-apply" mode, set it to `spec.approvePlan: auto`.

It is important to carefully consider which GitOps automation mode is appropriate for your use case to ensure that
your infrastructure is properly managed and controlled.

The following is an example of a `Terraform` object; we use the "auto-apply" mode:

```yaml hl_lines="8"
apiVersion: infra.contrib.fluxcd.io/v1alpha2
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

This code is defining a `Terraform` object in Kubernetes.
The `apiVersion` field specifies the version of the Kubernetes API being used,
and the `kind` field specifies that it is a `Terraform` object.
The `metadata` block contains information about the object, including its `name`.

The `spec` field contains the specification for the `Terraform` object.
The `path` field specifies the path to the Terraform configuration files,
in this case a directory named "helloworld".
The `interval` field specifies the frequency at which tofu-controller should run the Terraform configuration,
in this case every 10 minutes. The `approvePlan` field specifies whether or not
to automatically approve the changes proposed by a Terraform plan.
In this case, it is set to `auto`, meaning that changes will be automatically approved.

The `sourceRef` field specifies the Flux source object to be used.
In this case, it is a `GitRepository` object with the name "helloworld".
This indicates that the Terraform configuration is stored in a Git repository object with the name `helloworld`.
