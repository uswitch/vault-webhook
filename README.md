# Vault-webhook
Mutating webhook that injects the Vault-Creds sidecar into pods on pod creation using a custom resource for configuration.

## Usage
The webhook will do three things:
* Add a volume called `vault-creds` this is where you will find your credentials
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
```

The webhook expects there to be a volume called `vault-template` already there, this volume should be a configmap and the configmap should contain a file called `database-role.tmpl` e.g `test-readonly.tmpl` which will be used for templating your credentials. It will output the credentials to a file called `database-role` in the `vault-creds` volume.

Example Deployment:

```yaml
---
apiVersion: extensions/v1beta1
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
        - --db-creds=/creds/my-db-readonly
        volumeMounts:
          name: vault-creds
          mountPath: /creds
      volumes:
      - name: vault-template
        configMap:
          name: my-template
```
