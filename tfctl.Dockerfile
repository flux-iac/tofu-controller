FROM alpine:3.20.3@sha256:beefdbd8a1da6d2915566fde36db9db0b524eb737fc57cd1367effd16dc0d06d
USER 65532:65532
COPY tfctl /usr/local/bin/tfctl
ENTRYPOINT ["/usr/local/bin/tfctl"]