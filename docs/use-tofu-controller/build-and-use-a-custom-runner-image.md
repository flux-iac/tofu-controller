# Build and Use a Custom Runner Image

To build a custom runner image, you need a Dockerfile that extends the base image and that adds Terraform, plus any additional required tooling. The repository that contains the base images is [here](ghcr.io/flux-iac/tf-runner). All base image tags follow the following format: `${TF_CONTROLLER_VERSION}-base`.

## Prerequisites

You need Docker and Git to build the image.

## Build the Image

1. Create a `Dockerfile` that extends the base image and that adds Terraform, plus any additional required tooling. For example:

```Dockerfile
ARG BASE_IMAGE
FROM $BASE_IMAGE

ARG TARGETARCH
ARG TF_VERSION=1.5.7

# Switch to root to have permissions for operations
USER root

ADD https://releases.hashicorp.com/terraform/${TF_VERSION}/terraform_${TF_VERSION}_linux_${TARGETARCH}.zip /terraform_${TF_VERSION}_linux_${TARGETARCH}.zip
RUN unzip -q /terraform_${TF_VERSION}_linux_${TARGETARCH}.zip -d /usr/local/bin/ && \
    rm /terraform_${TF_VERSION}_linux_${TARGETARCH}.zip && \
    chmod +x /usr/local/bin/terraform

# Switch back to the non-root user after operations
USER 65532:65532
```

Find the original Dockerfile for the runner [here](https://github.com/flux-iac/tofu-controller/blob/main/runner.Dockerfile).

2. Build the image from the directory containing the `Dockerfile` you created above:

```bash
export TF_CONTROLLER_VERSION=v0.16.0-rc.3
export TF_VERSION=1.5.7
export BASE_IMAGE=ghcr.io/flux-iac/tf-runner:${TF_CONTROLLER_VERSION}-base
export TARGETARCH=amd64
export REMOTE_REPO=ghcr.io/my-org/custom-runnner
docker build \
    --build-arg BASE_IMAGE=${BASE_IMAGE} \
    --build-arg TARGETARCH=${TARGETARCH} \
    --tag my-custom-runner:${TF_CONTROLLER_VERSION} .
docker tag my-custom-runner:${TF_CONTROLLER_VERSION} $REMOTE_REPO:${TF_CONTROLLER_VERSION}
docker push $REMOTE_REPO:${TF_CONTROLLER_VERSION}
```

Replace the relevant values above with the corresponding values in your organisation/implementation.

3. Update the `values.runner.image` values in the tofu-controller Helm chart values to point to the new image:

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

A [set of GitHub actions in the tofu-controller community repo](https://github.com/flux-iac/tf-runner-images/blob/main/.github/workflows/release-runner-images.yaml) facilitates a process similar to the above, but uses GitHub Actions to build and push the image.
