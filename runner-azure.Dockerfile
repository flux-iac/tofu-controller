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

# Install az cli
ARG AZCLI_VERSION=2.50.0
RUN apk add --no-cache py3-pip && \
    apk add --no-cache gcc musl-dev python3-dev libffi-dev openssl-dev
RUN pip install --upgrade pip && \
    pip install azure-cli==${AZCLI_VERSION}

# Switch back to the non-root user after operations
USER 65532:65532

ENV GNUPGHOME=/tmp