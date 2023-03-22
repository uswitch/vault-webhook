# Vault-webhook
Mutating webhook that injects the [Vault-Creds sidecar](https://github.com/uswitch/vault-creds) into pods on pod creation using a custom resource for configuration.

**Note**: `vault-webhook` will only inject sidecar into pods which are in namespace labelled with `vault-webhook=enabled`.

## Usage
The webhook will do four things:
* Add a volume called `vault-creds` this is where you will find your credentials
* VolumeMount the `vault-creds` volume into your existing containers
* Add an init-container called `vault-creds-<database-role>-init`
* Add a container called `vault-creds-<database-role>`

It does this by checking the service account on your pod against custom resources called DatabaseCredentialBindings.
This resource links your ServiceAccount to a Database and role
Example DatabaseCredentialBinding:
```yaml
---
apiVersion: vaultwebhook.uswitch.com/v1alpha1
kind: DatabaseCredentialBinding
metadata:
  name: mybinding
  namespace: mynamespace
spec:
  serviceAccount: my_service_account
  database: mydb
  role: readonly
  outputPath: /config #Optional: defaults to /etc/database
  outputFile: mycreds #Optional: defaults to database-role
```

The webhook expects there to be a volume called `vault-template` already there, this volume should be a configmap and it should contain a file called `database-role` e.g `mydb-readonly` which will be used for templating your credentials. It will output the credentials to a file called `/etc/database/database-role` in the `vault-creds` volume. Note that the path where the file is found and the name of the file can be changed using the `outputPath` and `outputFile` fields in the CRD respectively.

Example Deployment:

```yaml
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
  namespace: mynamespace
spec:
  replicas: 1
  template:
    metadata:
    spec:
      serviceAccountName: my_service_account
      containers:
      - name: myapp
        args:
        - --db-creds=/etc/database/mydb-readonly
      volumes:
      - name: vault-template
        configMap:
          name: my-template
```

## Args

```sh
usage: vault-webhook-linux-amd64 --vault-address=VAULT-ADDRESS --login-path=LOGIN-PATH --sidecar-image=SIDECAR-IMAGE [<flags>]

Flags:
  --help                         Show context-sensitive help (also try --help-long and --help-man).
  --vault-address=VAULT-ADDRESS  URL of vault
  --vault-ca-path=VAULT-CA-PATH  Path to the CA cert for vault
  --login-path=LOGIN-PATH        Kubernetes auth login path for vault
  --sidecar-image=SIDECAR-IMAGE  Vault-creds sidecar image to use
  --gateway-address=GATEWAY-ADDRESS
                                 URL of Push Gateway
  --secret-path-format="%s/creds/%s"
                                 The format for the path used for reading database credentials, where the first %s is the database name and the second %s is the role
  --server-address=":8443"       The address the webhook server will listen on.
```
