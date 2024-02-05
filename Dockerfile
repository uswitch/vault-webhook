FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --chmod=755 bin/vault-webhook-linux-amd64 vault-webhook
USER nonroot:nonroot

ENTRYPOINT ["/vault-webhook"]
