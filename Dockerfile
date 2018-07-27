FROM scratch

ADD bin/vault-webhook vault-webhook

ENTRYPOINT ["/vault-webhook"]
