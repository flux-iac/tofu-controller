# Build the manager binary
FROM golang:1.20 as builder

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
      -gcflags=all="-N -l" \
      -ldflags "-X main.BuildSHA=${BUILD_SHA} -X main.BuildVersion=${BUILD_VERSION}" \
      -a \
      -o branch-planner \
      ./cmd/branch-planner

FROM alpine:3.18

LABEL org.opencontainers.image.source="https://github.com/weaveworks/tf-controller"

RUN apk update

RUN apk add --no-cache libcrypto3=3.1.2-r0 && \
    apk add --no-cache libssl3=3.1.2-r0 && \
    apk add --no-cache ca-certificates tini git openssh-client gnupg && \
    apk add --no-cache libretls && \
    apk add --no-cache busybox

COPY --from=builder /workspace/branch-planner /usr/local/bin/

RUN addgroup --gid 65532 -S controller && adduser --uid 65532 -S controller -G controller

USER 65532:65532

ENV GNUPGHOME=/tmp

ENTRYPOINT [ "/sbin/tini", "--", "branch-planner" ]
