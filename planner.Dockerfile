# Build the manager binary
FROM golang:1.20 as builder

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
COPY cmd/branch-based-planner/ cmd/branch-based-planner/
COPY internal/ internal/
COPY utils/ utils/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -gcflags=all="-N -l" -a -o branch-based-planner cmd/branch-based-planner/*.go

FROM alpine:3.18

LABEL org.opencontainers.image.source="https://github.com/weaveworks/tf-controller"

RUN apk update

RUN apk add --no-cache libcrypto3=3.1.1-r1 && \
    apk add --no-cache libssl3=3.1.1-r1 && \
    apk add --no-cache ca-certificates tini git openssh-client gnupg && \
    apk add --no-cache libretls && \
    apk add --no-cache busybox

COPY --from=builder /workspace/branch-based-planner /usr/local/bin/

# Create minimal nsswitch.conf file to prioritize the usage of /etc/hosts over DNS queries.
# https://github.com/gliderlabs/docker-alpine/issues/367#issuecomment-354316460
RUN echo 'hosts: files dns' > /etc/nsswitch.conf

RUN addgroup --gid 65532 -S planner && adduser --uid 65532 -S planner -G planner

USER 65532:65532

ENV GNUPGHOME=/tmp

ENTRYPOINT [ "/sbin/tini", "--", "branch-based-planner" ]
