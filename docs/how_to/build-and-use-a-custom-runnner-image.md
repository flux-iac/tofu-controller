# Using a custom runner image for TF-Controller

In order to build a custom runner image, you need to have a Dockerfile that extends the base image and adds Terraform, plus any additional tooling that may be needed.
The repository which contains the base images is [here](ghcr.io/weaveworks/tf-runner) - all base image tags have the following format: `$TF_CONTROLLER_VERSION-base`

## Prerequisites

docker and git are required to build the image.

## Build the image

1. Create a `Dockerfile` that extends the base image and adds Terraform, plus any additional tooling that may be needed. For example:

```Dockerfile
ARG BASE_IMAGE
FROM $BASE_IMAGE

ARG TARGETARCH
ARG TF_VERSION=1.3.9

# Switch to root to have permissions for operations
USER root

ADD https://releases.hashicorp.com/terraform/${TF_VERSION}/terraform_${TF_VERSION}_linux_${TARGETARCH}.zip /terraform_${TF_VERSION}_linux_${TARGETARCH}.zip
RUN unzip -q /terraform_${TF_VERSION}_linux_${TARGETARCH}.zip -d /usr/local/bin/ && \
    rm /terraform_${TF_VERSION}_linux_${TARGETARCH}.zip && \
    chmod +x /usr/local/bin/terraform

# Switch back to the non-root user after operations
USER 65532:65532
```

The original Dockerfile for the runner can be found [here](https://github.com/weaveworks/tf-controller/blob/89e0c7edde91efebba825b31e9f0ef3cc583684b/runner.Dockerfile)

2. Build the image (from the directory containing the `Dockerfile` you created above):

```bash
export TF_CONTROLLER_VERSION=v0.16.0-rc.3
export TF_VERSION=1.3.9
export BASE_IMAGE=ghcr.io/weaveworks/tf-runner:${TF_CONTROLLER_VERSION}-base
export TARGETARCH=amd64
export REMOTE_REPO=ghcr.io/my-org/custom-runnner
docker build \
    --build-arg BASE_IMAGE=${BASE_IMAGE} \
    --build-arg TARGETARCH=${TARGETARCH} \
    --tag my-custom-runner:${TF_CONTROLLER_VERSION} .
docker tag my-custom-runner:${TF_CONTROLLER_VERSION} $REMOTE_REPO:${TF_CONTROLLER_VERSION}
docker push $REMOTE_REPO:${TF_CONTROLLER_VERSION}
```

(replacing the relevant values above with the corresponding values in your organisation/implementation)

3. Update the `values.runner.image` values in the tf-controller Helm chart values to point to the new image:

```yaml
values:
  runner:
    image:
      repository: ghcr.io/my-org/custom-runnner
      tag: v0.16.0-rc.3
```

4. Commit and push the changes to git, and confirm that the HelmRelease has been updated:

```bash
kubectl get deployments.apps -n flux-system tf-controller -o jsonpath='{.spec.template.spec.containers[*]}' | jq '.image'
"ghcr.io/my-org/custom-runner:v0.16.0-rc.3"
```

### References:

There is a set of Github actions in the tf-controller community repo which facilitate a similar process to the above, but using Github actions to build and push the image.
The actions can be found [here](https://github.com/tf-controller/tf-runner-images/blob/main/.github/workflows/release-runner-images.yaml)
