ARG BASE_IMAGE
FROM $BASE_IMAGE

ARG TARGETARCH
ARG TF_VERSION=1.5.7
ARG TOFU_VERSION=1.6.2

# Switch to root to have permissions for operations
USER root

ADD https://releases.hashicorp.com/terraform/${TF_VERSION}/terraform_${TF_VERSION}_linux_${TARGETARCH}.zip /terraform_${TF_VERSION}_linux_${TARGETARCH}.zip
RUN unzip -q /terraform_${TF_VERSION}_linux_${TARGETARCH}.zip -d /usr/local/bin/ && \
    rm /terraform_${TF_VERSION}_linux_${TARGETARCH}.zip && \
    chmod +x /usr/local/bin/terraform

ADD https://github.com/opentofu/opentofu/releases/download/v${TOFU_VERSION}/tofu_${TOFU_VERSION}_linux_${TARGETARCH}.zip /tofu_${TOFU_VERSION}_linux_${TARGETARCH}.zip
RUN unzip -q /tofu_${TOFU_VERSION}_linux_${TARGETARCH}.zip -d /usr/local/bin/ && \
    rm /tofu_${TOFU_VERSION}_linux_${TARGETARCH}.zip && \
    chmod +x /usr/local/bin/tofu

# Switch back to the non-root user after operations
USER 65532:65532
