# Terraform Enterprise and Terraform Cloud Integration

Terraform is a secure and robust platform designed to store the Terraform states 
for your production systems. When working with Infrastructure as Code, 
managing and ensuring the state is both secure and consistent is critical. 

Tofu-controller supports both Terraform Cloud and Terraform Enterprise. The `spec.cloud` in the Terraform CRD enables users to integrate their Kubernetes configurations with Terraform workflows.

To get started, simply place your Terraform Cloud token in a Kubernetes Secret
and specify it in the `spec.cliConfigSecretRef` field of the Terraform CR.
The `spec.cloud` field specifies the organization and workspace name.

## Terraform Enterprise

Here are the steps to set up tofu-controller for your TFE instance.

![](tfe_integration_01.png)

### Terraform Login

First, you need to obtain an API token from your TFE. You can use `terraform login` command to do so.

```shell
terraform login tfe.dev.example.com
```

Then you can find your API token inside `$HOME/.terraform.d/credentials.tfrc.json`.
Content of the file will look like this:

```json
{
  "credentials": {
    "tfe.dev.example.com": {
      "token": "mXXXXXXXXX.atlasv1.ixXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
    }
  }
}
```

### Prepare an TFRC file
Tofu-controller accepts an TFRC file in the HCL format. So you have to prepare `terraform.tfrc` file using contents from above.
```hcl
credentials "tfe.dev.example.com" {
  token = "mXXXXXXXXX.atlasv1.ixXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
}
```

### Create a Secret
We will now create a Kubernetes Secret from your`terraform.tfrc` file, 
name it `tfe-cli-config` and put it inside the `flux-system` namespace.

```shell
kubectl create secret generic \
  tfe-cli-config \
  --namespace=flux-system \
  --from-file=terraform.tfrc=./terraform.tfrc
```

### Terraform Object

In your Terraform object, you'll have to 1. disable the backend by setting `spec.backendConfig.disable: true`, and 2. point `spec.cliConfigSecretRef:` to the Secret created in the previous step, like this:

```yaml hl_lines="10-14"
---
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: tfe-demo
  namespace: flux-system
spec:
  approvePlan: auto
  interval: 2m
  path: ./terraform/tfe-demo
  backendConfig:
    disable: true
  cliConfigSecretRef:
    name: tfe-cli-config
    namespace: flux-system
  vars:
  - name: subject
    value: World
  sourceRef:
    kind: GitRepository
    name: flux-system
    namespace: flux-system
  writeOutputsToSecret:
    name: tfe-helloworld-output
    outputs:
    - greeting
```

### Terraform Module

Don't forget that you need to tell your Terraform model to use your enterprise instance as well. Here's an example,
```hcl
terraform {
  required_version = ">= 1.1.0"
  cloud {
    hostname = "tfe.dev.example.com"
    organization = "flux-iac"

    workspaces {
      name = "dev"
    }
  }
}

variable "subject" {
   type = string
   default = "World"
   description = "Subject to hello"
}

output "greeting" {
  value = "Hello ${var.subject} from Terraform Enterprise"
}
```

### Terraform Cloud

Tofu-controller can send your Terraform resources to be planned and applied via Terraform Cloud. 
States are automatically stored in your Terraform Cloud's workspace. 
To use tofu-controller with Terraform Cloud, replace your hostname to `app.terraform.io`. Also, set `spec.approvalPlan` to `auto`. 

Here's how the configuration looks:

```yaml
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: branch-planner-tfc
  namespace: flux-system
spec:
  interval: 2m
  approvePlan: auto
  cloud:
    organization: flux-iac
    workspaces:
      name: branch-planner-tfc
  cliConfigSecretRef:
    name: tfc-cli-config
    namespace: flux-system
```
