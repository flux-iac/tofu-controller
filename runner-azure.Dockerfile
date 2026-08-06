ARG BASE_IMAGE
ARG TOFU_VERSION=1.12.1

FROM ghcr.io/opentofu/opentofu:${TOFU_VERSION}-minimal AS tofu

FROM $BASE_IMAGE

ARG AZURE_CLI_VERSION=2.89.0

# azure-cli-core pins msal==1.36.0, which caps cryptography at <49 and leaves the
# image on 48.x (CVE-2026-69247, CVE-2026-69249). Upgraded as a set below; drop
# these once azure-cli moves to msal >= 1.37.0.
ARG CRYPTOGRAPHY_VERSION=50.0.0
ARG PYOPENSSL_VERSION=26.4.0
ARG MSAL_VERSION=1.37.0

# Switch to root temporarily for package installation (base image runs as 65532).
USER root

# azure-cli is a pip package; installing just the binary is not sufficient.
# We need Python and pip, then install the full azure-cli package.
# Build dependencies (gcc, etc.) are needed to compile psutil, a C extension
# required by azure-cli. They are removed after installation to keep image size down.
RUN apk add --no-cache python3 py3-virtualenv gcc python3-dev musl-dev linux-headers && \
    python3 -m venv /opt/az && \
    /opt/az/bin/pip install --no-cache-dir setuptools azure-cli==${AZURE_CLI_VERSION} && \
    /opt/az/bin/pip install --no-cache-dir --no-deps --upgrade \
        cryptography==${CRYPTOGRAPHY_VERSION} \
        pyOpenSSL==${PYOPENSSL_VERSION} \
        msal==${MSAL_VERSION} && \
    ln -s /opt/az/bin/az /usr/local/bin/az && \
    apk del gcc python3-dev musl-dev linux-headers

COPY --from=tofu /usr/local/bin/tofu /usr/local/bin/tofu

# Switch back to the non-root user after operations
USER 65532:65532

