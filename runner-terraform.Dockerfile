ARG BASE_IMAGE
ARG TERRAFORM_VERSION=1.14.3

FROM hashicorp/terraform:${TERRAFORM_VERSION} AS terraform

FROM $BASE_IMAGE

COPY --from=terraform /bin/terraform /usr/local/bin/terraform

# Switch back to the non-root user after operations
USER 65532:65532
