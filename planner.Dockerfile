# Build the manager binary
FROM golang:1.25 AS builder

ARG TARGETARCH
ARG BUILD_SHA
ARG BUILD_VERSION

RUN apt-get update && apt-get install -y unzip

WORKDIR /workspace
# Copy API and its Go module
COPY api/ api/
# Copy tfctl and its Go module
COPY tfctl/ tfctl/
# Copy utils and its Go module
COPY utils/ utils/

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/branch-planner cmd/branch-planner
COPY internal internal

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build \
    -ldflags "-X main.BuildSHA=${BUILD_SHA} -X main.BuildVersion=${BUILD_VERSION}" \
    -o branch-planner \
    ./cmd/branch-planner

FROM alpine:3.22

LABEL org.opencontainers.image.source="https://github.com/flux-iac/tofu-controller"

ARG LIBCRYPTO_VERSION

RUN apk update && \
    apk add --no-cache \
    libcrypto3=${LIBCRYPTO_VERSION} \
    libssl3=${LIBCRYPTO_VERSION} \
    ca-certificates tini git openssh-client gnupg \
    busybox

COPY --from=builder /workspace/branch-planner /usr/local/bin/

RUN addgroup --gid 65532 -S controller && adduser --uid 65532 -S controller -G controller

USER 65532:65532

ENV GNUPGHOME=/tmp

ENTRYPOINT [ "/sbin/tini", "--", "branch-planner" ]
