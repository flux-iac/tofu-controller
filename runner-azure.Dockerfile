# Build the manager binary
FROM golang:1.19 as builder

ARG TARGETARCH

RUN apt-get update && apt-get install -y unzip

WORKDIR /workspace
# Copy API and its Go module
COPY api/ api/
# Copy tfctl and its Go module
COPY tfctl/ tfctl/

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/runner/main.go cmd/runner/main.go
COPY controllers/ controllers/
COPY mtls/ mtls/
COPY runner/ runner/
COPY utils/ utils/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -a -o tf-runner cmd/runner/main.go

ARG TF_VERSION=1.3.9
ADD https://releases.hashicorp.com/terraform/${TF_VERSION}/terraform_${TF_VERSION}_linux_${TARGETARCH}.zip /terraform_${TF_VERSION}_linux_${TARGETARCH}.zip
RUN unzip -q /terraform_${TF_VERSION}_linux_${TARGETARCH}.zip

FROM alpine:3.18

LABEL org.opencontainers.image.source="https://github.com/weaveworks/tf-controller"

RUN apk update

RUN apk add --no-cache libcrypto3=3.1.1-r1 && \
    apk add --no-cache libssl3=3.1.1-r1 && \
    apk add --no-cache ca-certificates tini git openssh-client gnupg && \
    apk add --no-cache libretls && \
    apk add --no-cache busybox

# Install az cli
ARG AZCLI_VERSION=2.40.0
RUN apk add --no-cache py3-pip && \
    apk add --no-cache gcc musl-dev python3-dev libffi-dev openssl-dev
RUN pip install --upgrade pip && \
    pip install azure-cli==${AZCLI_VERSION}

COPY --from=builder /workspace/tf-runner /usr/local/bin/
COPY --from=builder /workspace/terraform /usr/local/bin/

# Create minimal nsswitch.conf file to prioritize the usage of /etc/hosts over DNS queries.
# https://github.com/gliderlabs/docker-alpine/issues/367#issuecomment-354316460
RUN echo 'hosts: files dns' > /etc/nsswitch.conf

RUN addgroup --gid 65532 -S runner && adduser --uid 65532 -S runner -G runner && chmod +x /usr/local/bin/terraform

USER 65532:65532

ENV GNUPGHOME=/tmp

ENTRYPOINT [ "/sbin/tini", "--", "tf-runner" ]
