# How to pre-load Providers in your custom tf-runner image

To build a custom runner image, follow the instructions in the [Build and Use a Custom Runner Image](build-and-use-a-custom-runner-image.md) guide. In this guide, we will customize the Dockerfile and the tofu-controller helm values.

This guide is useful if you are trying to avoid networking costs associated with downloading providers on each run. Without caching, this can add up to significant costs in scenarios where you're downloading providers through a NAT gateway.

## Prerequisites

You need Docker and Git to build the image.

## Set up your build directory

1. Create a `Dockerfile` that extends the base image and that adds Terraform, plus any additional required tooling. For example:

```Dockerfile
# Pass in the baseimage and tag from the tf-runner image you want to extend
ARG BASE_IMAGE
ARG BASE_TAG

FROM $BASE_IMAGE:$BASE_TAG

# Set up HOME var so that the terraformrc can be found by tf cli
ENV HOME=/home/runner

# Add the terraformrc file to the image
ADD .build-terraformrc $HOME/.terraformrc

# Add the tf_providers.tf file to the image
ADD tf_providers.tf /tmp/cache-init/tf_providers.tf

# Switch to root to have permissions for operations
USER root

# Create the plugins directory and run terraform init to download the providers
RUN mkdir -p /usr/local/share/terraform/plugins && \
    cd /tmp/cache-init && \
    terraform init -backend=false && \
    cd - && \
    rm -rf /tmp/cache-init && \
    # Ensure proper permissions for runtime access to registry mirror
    # Note: do not use `/home/runner` or `/tmp`, as they get overwritten at runtime
    chown -R 65532:65532 /usr/local/share/terraform

# Switch back to the non-root user after operations
USER 65532:65532
```

2. Add `.build-terraformrc` and `tf_providers.tf` to the directory.

```
# .build-terraformrc
# Create a local cache dir
plugin_cache_dir   = "/usr/local/share/terraform/plugins"
# Disable reaching out to Hashicorp for upgrade and security checks
disable_checkpoint = true
```

```
# tf_providers.tf
terraform {
  required_providers {
    aws = {
      source = "hashicorp/aws"
      version = "5.47.0"
    }
    azuread = {
      source = "hashicorp/azuread"
      version = "2.48.0"
    }
    azurerm = {
      source = "hashicorp/azurerm"
      version = "3.101.0"
    }
    google = {
      source = "hashicorp/google"
      version = "5.26.0"
    }
  }
}
```

3. Build the image from the directory containing the `Dockerfile` you created above:

```bash
export BASE_IMAGE=ghcr.io/flux-iac/tf-runner
export BASE_TAG=v0.16.0-rc.4
docker build \
    --build-arg BASE_IMAGE=${BASE_IMAGE} \
    --build-arg BASE_TAG=${BASE_TAG} \
    --tag my-custom-runner:${BASE_TAG} .
docker tag my-custom-runner:${BASE_TAG} $REMOTE_REPO:${BASE_TAG}
docker push $REMOTE_REPO:${BASE_TAG}
```

Replace the relevant values above with the corresponding values in your organisation/implementation.

4. Update the `values.runner.image` values in the TF-Controller Helm chart values to point to the new image:

```yaml
values:
  runner:
    image:
      repository: ghcr.io/my-org/custom-runnner
      tag: v0.16.0-rc.3
```

5. Add secret to tofu-controller namespace with the runtime tf cli rc

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: tf-cli-config # This name is specific, it gets referenced by the Terraform resource
  namespace: flux-system
data:
  # This file name has to end with `.tfrc` to be picked up correctly
  ### CONTENTS ###
  # # Disable reaching out to external registry
  # disable_checkpoint = true
  # # Force TF to use the local registry mirror for all providers
  # provider_installation {
  #   filesystem_mirror {
  #     path    = "/usr/local/share/terraform/plugins"
  #     include = ["registry.terraform.io/*/*"]
  #   }
  # }
  ### END CONTENTS ###
  # Base64 encoded contents of .runtime-terraformrc
  cli-config.tfrc: IyBEaXNhYmxlIHJlYWNoaW5nIG91dCB0byBIYXNoaWNvcnAgZm9yIHVwZ3JhZGUgYW5kIHNlY3VyaXR5IGNoZWNrcwpkaXNhYmxlX2NoZWNrcG9pbnQgPSB0cnVlCnByb3ZpZGVyX2luc3RhbGxhdGlvbiB7CiAgZmlsZXN5c3RlbV9taXJyb3IgewogICAgcGF0aCAgICA9ICIvdXNyL2xvY2FsL3NoYXJlL3RlcnJhZm9ybS9wbHVnaW5zIgogICAgaW5jbHVkZSA9IFsicmVnaXN0cnkudGVycmFmb3JtLmlvLyovKiJdCiAgfQp9Cg==
```

7. Update all Terraform resources to use the cliConfigSecret

```yaml
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: example-tf
  namespace: flux-system
spec:
  cliConfigSecret:
    name: tf-cli-config
    namespace: flux-system
```

8. Commit and push the changes to Git. Confirm that the HelmRelease has been updated:

```bash
kubectl get deployments.apps -n flux-system tf-controller -o jsonpath='{.spec.template.spec.containers[*]}' | jq '.env[] | select(.name == "RUNNER_POD_IMAGE")'
{
  "name": "RUNNER_POD_IMAGE",
  "value": "ghcr.io/my-org/custom-runner:v0.16.0-rc3"
}
```
