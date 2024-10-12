# Option: Use tofu-controller with a Custom Backend

By default, tofu-controller uses the [Kubernetes backend](https://www.terraform.io/language/settings/backends/kubernetes) to store the Terraform state file (tfstate) in clusters.

The tfstate is stored in a secret named: `tfstate-${workspace}-${secretSuffix}`. The default `suffix` will be the name of the Terraform resource, however you may override this setting using `.spec.backendConfig.secretSuffix`. The default `workspace` name is "default", you can also override the workspace by setting `.spec.workspace` to another value.

If you wish to use a custom backend, you can configure it by defining the `.spec.backendConfig.customConfiguration` with one of the backends such as **GCS** or **S3**, for example:

```yaml hl_lines="9-21"
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  approvePlan: auto
  backendConfig:
    customConfiguration: |
      backend "s3" {
        bucket                      = "s3-terraform-state1"
        key                         = "dev/terraform.tfstate"
        region                      = "us-east-1"
        endpoint                    = "http://localhost:4566"
        skip_credentials_validation = true
        skip_metadata_api_check     = true
        force_path_style            = true
        dynamodb_table              = "terraformlock"
        dynamodb_endpoint           = "http://localhost:4566"
        encrypt                     = true
      }
  interval: 1m
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
  runnerPodTemplate:
    spec:
      image: registry.io/tf-runner:xyz
```

To add fields from secrets or configMaps, use `backendConfigsFrom`, for example to add `access_key` and `secret_key` from a secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: terraform-s3-backend
  namespace: flux-system
type: Opaque
data:
  access_key: <base64 encoded key>
  secret_key: <base64 encoded key>

---

apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  approvePlan: auto
  backendConfig:
    customConfiguration: |
      backend "s3" {
        bucket                      = "s3-terraform-state1"
        key                         = "dev/terraform.tfstate"
        region                      = "us-east-1"
        endpoint                    = "http://localhost:4566"
        skip_credentials_validation = true
        skip_metadata_api_check     = true
        force_path_style            = true
        dynamodb_table              = "terraformlock"
        dynamodb_endpoint           = "http://localhost:4566"
        encrypt                     = true
      }
  backendConfigsFrom:
    - kind: Secret
      name: terraform-s3-backend
      keys:
      - access_key
      - secret_key
      optional: false
  interval: 1m
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
  runnerPodTemplate:
    spec:
      image: registry.io/tf-runner:xyz
```
