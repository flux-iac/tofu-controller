ARG GO_VERSION=1.25
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} as builder

WORKDIR /build

COPY . .

ARG BUILD_SHA
ARG BUILD_VERSION

# Cache dependencies
RUN go mod download

# Create a user and group for the controller
RUN groupadd -g 65532 controller \
    && useradd -u 65532 -g controller -r -s /sbin/nologin controller

# Build the controller!
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0 go build -ldflags \
    " \
    -X 'main.BuildSHA=${BUILD_SHA}' \
    -X 'main.BuildVersion=${BUILD_VERSION}'\
    " -o /out/tofu-controller ./cmd/manager

# Build release container
FROM scratch

# Copy certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Copy passwd
COPY --from=builder /etc/passwd /etc/passwd

# Copy executable to scratch container
COPY --from=builder /out/tofu-controller /tofu-controller

# Setting the UID
USER 65532:65532

LABEL org.opencontainers.image.source="https://github.com/flux-iac/tofu-controller"

ENTRYPOINT ["/tofu-controller"]
