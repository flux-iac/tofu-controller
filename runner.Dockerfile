ARG BASE_IMAGE
ARG TOFU_VERSION=1.11.5

FROM ghcr.io/opentofu/opentofu:${TOFU_VERSION}-minimal AS tofu

FROM $BASE_IMAGE

COPY --from=tofu /usr/local/bin/tofu /usr/local/bin/tofu

# Switch back to the non-root user after operations
USER 65532:65532
