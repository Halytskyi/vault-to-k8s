# Deployment

## Remote cluster

Change environment variables in [deployment.yaml](deployment.yaml) and credentials for AppRole auth in [secret.yaml](secret.yaml).

```bash
kubectl apply -f .
```

## Local tests

```bash
GOOS=linux go build -o ./app .
docker build -t in-cluster .
kubectl run --generator=run-pod/v1 --rm -i vault-to-k8s -n <my-ns> \
  --serviceaccount=vault-to-k8s \
  --image=in-cluster \
  --image-pull-policy=Never \
  --restart=Never \
  --env VAULT_NAMESPACE=my-vault-namespace \
  --env VAULT_ADDR=https://vault.url.net \
  --env AUTH_METHOD=approle \
  --env APPROLE_ROLE_ID=<role-id> \
  --env APPROLE_SECRET_ID_WRAPPED_TOKEN=<secret-id> \
  --env DEBUG=true \
  --env NUM_WORKERS=5 \
  --env SYNC_INTERVAL=30 \
  --env K8S_CLUSTER_NAME="my-k8s-cluster" \
  --env SECRETS_PATH_VAULT="mydir/k8s/dev" \
  --env TOKEN_ROTATION_INTERVAL=1800 \
  --env APPROLE_SECRETID_ROTATION_INTERVAL=7000 \
  --env NON_VERSIONING_NAMESPACES="default"
```
