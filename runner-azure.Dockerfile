ARG BASE_IMAGE
ARG TOFU_VERSION=1.12.1

FROM ghcr.io/opentofu/opentofu:${TOFU_VERSION}-minimal AS tofu

FROM $BASE_IMAGE

ARG AZURE_CLI_VERSION=2.86.0

# Switch to root temporarily for package installation (base image runs as 65532).
USER root

# azure-cli is a pip package; installing just the binary is not sufficient.
# We need Python and pip, then install the full azure-cli package.
# Build dependencies (gcc, etc.) are needed to compile psutil, a C extension
# required by azure-cli. They are removed after installation to keep image size down.
RUN apk add --no-cache python3 py3-virtualenv gcc python3-dev musl-dev linux-headers && \
    python3 -m venv /opt/az && \
    /opt/az/bin/pip install --no-cache-dir setuptools azure-cli==${AZURE_CLI_VERSION} && \
    ln -s /opt/az/bin/az /usr/local/bin/az && \
    apk del gcc python3-dev musl-dev linux-headers

COPY --from=tofu /usr/local/bin/tofu /usr/local/bin/tofu

# Switch back to the non-root user after operations
USER 65532:65532

