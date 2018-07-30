# Vault-webhook
Mutating Webhook that injects the Vault-Creds sidecar into pods on pod creation using just annotations for configuration.

## Usage
The webhook acts whenever a pod is created that has the `vault.uswitch.com/` annotations, there are two annotations that must be set:
* `vault.uswitch.com/database:`
* `vault.uswitch.com/role:`

The webhook will do three things:
* Add a volume called `vault-creds` this is where you will find your credentials
* Add an init-container called `vault-creds-<database-role>-init`
* Add a container called `vault-creds-<database-role>`

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
      annotations:
        vault.uswitch.com/database: "my-db"
        vault.uswitch.com/role: "readonly"
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
