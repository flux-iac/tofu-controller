# Build the manager binary
FROM golang:1.19 as builder

RUN apt-get update && apt-get install -y unzip

WORKDIR /workspace
# Copy API and it's go module
COPY api/ api/

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
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o tf-runner cmd/runner/main.go

ARG TF_VERSION=1.3.1
ADD https://releases.hashicorp.com/terraform/${TF_VERSION}/terraform_${TF_VERSION}_linux_amd64.zip /terraform_${TF_VERSION}_linux_amd64.zip
RUN unzip -q /terraform_${TF_VERSION}_linux_amd64.zip

FROM python:3.10.5-alpine3.16 as aws-builder

ARG AWS_CLI_VERSION=2.7.20
RUN apk add --no-cache git unzip groff build-base libffi-dev cmake
RUN git clone --single-branch --depth 1 -b ${AWS_CLI_VERSION} https://github.com/aws/aws-cli.git

WORKDIR aws-cli
RUN sed -i'' 's/PyInstaller.*/PyInstaller==5.2/g' requirements-build.txt
RUN python -m venv venv
RUN . venv/bin/activate
RUN scripts/installers/make-exe
RUN unzip -q dist/awscli-exe.zip
RUN aws/install --bin-dir /aws-cli-bin
RUN /aws-cli-bin/aws --version

# reduce image size: remove autocomplete and examples
RUN rm -rf /usr/local/aws-cli/v2/current/dist/aws_completer /usr/local/aws-cli/v2/current/dist/awscli/data/ac.index /usr/local/aws-cli/v2/current/dist/awscli/examples
RUN find /usr/local/aws-cli/v2/current/dist/awscli/botocore/data -name examples-1.json -delete


FROM alpine:3.16

LABEL org.opencontainers.image.source="https://github.com/weaveworks/tf-controller"

RUN apk add --no-cache ca-certificates tini git openssh-client gnupg && \
    apk add --no-cache libretls && \
    apk add --no-cache busybox

COPY --from=builder /workspace/tf-runner /usr/local/bin/
COPY --from=builder /workspace/terraform /usr/local/bin/
#aws-cli
COPY --from=aws-builder /usr/local/aws-cli/ /usr/local/aws-cli/
COPY --from=aws-builder /aws-cli-bin/ /usr/local/bin/

# Create minimal nsswitch.conf file to prioritize the usage of /etc/hosts over DNS queries.
# https://github.com/gliderlabs/docker-alpine/issues/367#issuecomment-354316460
RUN [ ! -e /etc/nsswitch.conf ] && echo 'hosts: files dns' > /etc/nsswitch.conf

RUN addgroup --gid 65532 -S runner && adduser --uid 65532 -S runner -G runner && chmod +x /usr/local/bin/terraform

USER 65532:65532

ENV GNUPGHOME=/tmp

ENTRYPOINT [ "/sbin/tini", "--", "tf-runner" ]
