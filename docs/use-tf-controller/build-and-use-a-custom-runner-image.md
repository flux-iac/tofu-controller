# Build and Use a Custom Runner Image

To build a custom runner image, you need a Dockerfile that extends the base image and that adds OpenTofu or Terraform (plus any additional required tooling).

The repository that contains the base images is [here](ghcr.io/flux-iac/tf-runner).

All base image tags follow the following format: `${TF_CONTROLLER_VERSION}-base`.

## Using Terraform Instead of OpenTofu

The default runner images use OpenTofu. To use Terraform:

1. Use the `-terraform` tagged image: `ghcr.io/flux-iac/tf-runner:v0.16.0-rc.7-terraform`
2. Specify this in your Helm values or per-resource configuration

## Available Dockerfiles

The project provides four Dockerfiles:

- `runner.Dockerfile` - OpenTofu binary (default)
- `runner-terraform.Dockerfile` - Terraform binary
- `runner-azure.Dockerfile` - OpenTofu binary with Azure CLI
- `runner-terraform-azure.Dockerfile` - Terraform binary with Azure CLI

Build using the appropriate Dockerfile for your needs.

## Prerequisites

You need Docker and Git to build the image.

## Build the Image

1. Create a `Dockerfile` that extends the base image and that adds the OpenTofu or Terraform binary, plus any additional required tooling.

### OpenTofu Variant

```Dockerfile
ARG BASE_IMAGE
ARG TOFU_VERSION=1.11.2

FROM ghcr.io/opentofu/opentofu:${TOFU_VERSION}-minimal AS tofu

FROM $BASE_IMAGE

COPY --from=tofu /usr/local/bin/tofu /usr/local/bin/tofu

# Switch back to the non-root user after operations
USER 65532:65532
```

### Terraform Variant

```Dockerfile
ARG BASE_IMAGE
ARG TF_VERSION=1.14.3
ARG TARGETARCH

FROM alpine:3.22 AS terraform
ARG TARGETARCH
ARG TF_VERSION
RUN apk add --no-cache wget unzip && \
    wget https://releases.hashicorp.com/terraform/${TF_VERSION}/terraform_${TF_VERSION}_linux_${TARGETARCH}.zip && \
    unzip terraform_${TF_VERSION}_linux_${TARGETARCH}.zip -d /usr/local/bin/ && \
    rm terraform_${TF_VERSION}_linux_${TARGETARCH}.zip && \
    chmod +x /usr/local/bin/terraform

FROM $BASE_IMAGE

COPY --from=terraform /usr/local/bin/terraform /usr/local/bin/terraform

# Switch back to the non-root user after operations
USER 65532:65532
```

Find the original Dockerfile for the runner [here](https://github.com/flux-iac/tofu-controller/blob/main/runner.Dockerfile).

2. Build the image from the directory containing the `Dockerfile` you created above:

    ```bash
    export TF_CONTROLLER_VERSION=v0.16.0-rc.7
    export TOFU_VERSION=1.6.0
    export BASE_IMAGE=ghcr.io/flux-iac/tf-runner:${TF_CONTROLLER_VERSION}-base
    export REMOTE_REPO=ghcr.io/my-org/custom-runnner
    docker build \
        --build-arg BASE_IMAGE=${BASE_IMAGE} \
        --build-arg TOFU_VERSION=${TOFU_VERSION} \
        --tag my-custom-runner:${TF_CONTROLLER_VERSION} .
    docker tag my-custom-runner:${TF_CONTROLLER_VERSION} $REMOTE_REPO:${TF_CONTROLLER_VERSION}
    docker push $REMOTE_REPO:${TF_CONTROLLER_VERSION}
    ```

    Replace the relevant values above with the corresponding values in your organisation/implementation.

3. Update the `values.runner.image` values in the Tofu Controller Helm chart values to point to the new image:

    ```yaml
    values:
      runner:
        image:
          repository: ghcr.io/my-org/custom-runnner
          tag: v0.16.0-rc.3
    ```

4. Commit and push the changes to Git. Confirm that the HelmRelease has been updated:

    ```bash
    kubectl get deployments.apps -n flux-system tofu-controller -o jsonpath='{.spec.template.spec.containers[*]}' | jq '.env[] | select(.name == "RUNNER_POD_IMAGE")'
    {
      "name": "RUNNER_POD_IMAGE",
      "value": "ghcr.io/my-org/custom-runner:v0.16.0-rc3"
    }
    ```

### References

A [set of GitHub actions in the Tofu Controller community repo](https://github.com/flux-iac/tf-runner-images/blob/main/.github/workflows/release-runner-images.yaml) facilitates a process similar to the above, but uses GitHub Actions to build and push the image.
