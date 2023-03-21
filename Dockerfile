FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY bin/vault-webhook-linux-amd64 vault-webhook
USER nonroot:nonroot

ENTRYPOINT ["/vault-webhook"]
