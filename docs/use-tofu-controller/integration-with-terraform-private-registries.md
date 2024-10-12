# Terraform Private Registries Integration

Using Terraform private registries with the tofu-controller is exactly as you would use them directly via Terraform.  
For example, you would like to use the tofu-controller to deploy code that contains the following module:
```terraform
module "vpc" {
  source  = "my.private.server/terraform-modules/path/to/module"
  version = "1.2.3"

  ...
  ...
}
```
without configuring the terraform login process, deploying the module with the controller will result in the error:
```shell
Failed to retrieve available versions for module "vpc" (main.tf:1) from
my.private.server: error looking up module versions: 401 Unauthorized.
```

### Terraform Login
As a human you would normally execute `terraform login my.private.server` to obtain a token from the registry,  
with the tofu-controller use the native [terraform credentials](https://developer.hashicorp.com/terraform/cli/config/config-file#credentials) configs instead.

Obtain a token from your private registry, then follow one of the below options:

#### Using credentials file

content of `credentials.tfrc` should look like:
```json
{
  "credentials": {
    "my.private.server": {
      "token": "TOP_SECRET_TOKEN"
    }
  }
}
```

K8S secret example:
```yaml
apiVersion: "v1"
kind: "Secret"
metadata:
  name: tf-private-config
type: "Opaque"
stringData:
  credentials.tfrc: |-
    {
      "credentials": {
        "my.private.server": {
          "token": "TOP_SECRET_TOKEN"
        }
      }
    }
```
Then deploy the Terraform object, while referencing the above `tf-private-config` secret
```yaml
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: tf-private-demo
  namespace: flux-system
spec:
  approvePlan: auto
  interval: 2m
  path: ./terraform/tf-private-demo
  cliConfigSecretRef:
    name: tf-private-config
    namespace: flux-system
  sourceRef:
    kind: GitRepository
    name: flux-system
    namespace: flux-system
```
---
#### Using environment variables
Another option is to use [environment variable credentials](https://developer.hashicorp.com/terraform/cli/config/config-file#environment-variable-credentials),  
Terraform object should look like:
```yaml
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: tf-private-demo
  namespace: flux-system
spec:
  approvePlan: auto
  interval: 2m
  path: ./terraform/tf-private-demo
  sourceRef:
    kind: GitRepository
    name: flux-system
    namespace: flux-system
  # api referance https://flux-iac.github.io/tofu-controller/References/terraform/#infra.contrib.fluxcd.io/v1alpha2.RunnerPodTemplate
  runnerPodTemplate:
    spec:
      env:
        - name: "TF_TOKEN_my_private_server"
          value: "TOP_SECRET_TOKEN"
      # or use get ENV from existing secret
      envFrom:
        - secretRef:
            name: tf-private-token
```