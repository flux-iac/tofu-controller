ARG BASE_IMAGE
ARG TERRAFORM_VERSION=1.14.3

FROM mcr.microsoft.com/azure-cli:2.50.0 AS azcli
FROM hashicorp/terraform:${TERRAFORM_VERSION} AS terraform

FROM $BASE_IMAGE

COPY --from=azcli /usr/local/bin/az /usr/local/bin/az
COPY --from=terraform /bin/terraform /usr/local/bin/terraform

# Switch back to the non-root user after operations
USER 65532:65532
