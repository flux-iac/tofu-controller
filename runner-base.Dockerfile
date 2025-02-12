# Build the manager binary
FROM golang:1.23 as builder

ARG TARGETARCH
ARG BUILD_SHA
ARG BUILD_VERSION

RUN apt-get update && apt-get install -y unzip

WORKDIR /workspace

# Copy API and its Go module
COPY api/ api/
# Copy tfctl and its Go module
COPY tfctl/ tfctl/

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# Cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/runner/ cmd/runner/
COPY controllers/ controllers/
COPY mtls/ mtls/
COPY runner/ runner/
COPY internal/ internal/
COPY utils/ utils/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build \
        -gcflags=all="-N -l" \
        -ldflags "-X main.BuildSHA=${BUILD_SHA} -X main.BuildVersion=${BUILD_VERSION}" \
        -a \
        -o tf-runner \
        ./cmd/runner/main.go

FROM alpine:3.21

LABEL org.opencontainers.image.source="https://github.com/flux-iac/tofu-controller"

ARG LIBCRYPTO_VERSION

RUN apk update && \
    apk add --no-cache \
    busybox \
    ca-certificates \
    git \
    libc6-compat \
    gnupg \
    libcrypto3=${LIBCRYPTO_VERSION} \
    libssl3=${LIBCRYPTO_VERSION} \
    libretls \
    openssh-client \
    tini

COPY --from=builder /workspace/tf-runner /usr/local/bin/

RUN addgroup --gid 65532 -S runner && adduser --uid 65532 -S runner -G runner

USER 65532:65532

ENV GNUPGHOME=/tmp

ENTRYPOINT [ "/sbin/tini", "--", "tf-runner" ]
