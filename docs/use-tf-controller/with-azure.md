## Use tofu-controller with Azure

This content was [provided](https://github.com/flux-iac/tofu-controller/issues/561) by users [@mingmingshiliyu](https://github.com/mingmingshiliyu) and [@maciekdude](https://github.com/maciekdude).

Use the OIDC flag and explicitly point to the token. Due to a bug in AzureRM 3.44.x, use version 3.47.x or later.

Set env variables on the runner pod:

```
        - name: ARM_USE_OIDC
          value: "true"
        - name: ARM_OIDC_TOKEN_FILE_PATH
          value: "/var/run/secrets/azure/tokens/azure-identity-token"
```

Example yaml:

```yaml
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: terraformhello
  namespace: default
spec:
  tfstate:
    forceUnlock: auto
  backendConfig:
    customConfiguration: |
      backend "azurerm" {
        resource_group_name  = "l"
        storage_account_name = ""
        container_name       = "tfstate"
        key                  = "helloworld.tfstate"
        use_oidc             = true
      }
  interval: 1m
  serviceAccountName: service_account_registered_in_aad
  approvePlan: auto
  destroy: true
  path: ./tests/fixture
  sourceRef:
    kind: GitRepository
    name: terraformhello
    namespace: flux-system
  runnerPodTemplate:
    spec:
      image: azure_cli_runner.xxx
      env:
        - name: ARM_USE_OIDC
          value: "true"
        - name: ARM_SUBSCRIPTION_ID
          value: ""
        - name: ARM_TENANT_ID
          value: ""
        - name: ARM_CLIENT_ID
          value: ""
        - name: ARM_OIDC_TOKEN_FILE_PATH
          value: "/var/run/secrets/azure/tokens/azure-identity-token"
```

Import existing resources to a tfstate file stored on a storage account.
