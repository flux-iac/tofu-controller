# Backup and restore a Terraform state

## Backup the tfstate

Assume that we have the `my-stack` Terraform object with its `.spec.workspace` set to "default".

```bash
kubectl get terraform

NAME       READY     STATUS         AGE
my-stack   Unknown   Initializing   28s
```

We can backup its tfstate out of the cluster, like this:

```bash
WORKSPACE=default
NAME=my-stack

kubectl get secret tfstate-${WORKSPACE}-${NAME} \
  -ojsonpath='{.data.tfstate}' \
  | base64 -d | gzip -d > terraform.tfstate
```

## Restore the tfstate

To restore the tfstate file or import an existing tfstate file to the cluster, we can use the following operation:

```bash
gzip terraform.tfstate

WORKSPACE=default
NAME=my-stack

kubectl create secret \
  generic tfstate-${WORKSPACE}-${NAME} \
  --from-file=tfstate=terraform.tfstate.gz \
  --dry-run=client -o=yaml \
  | yq e '.metadata.annotations["encoding"]="gzip"' - \
  > tfstate-${WORKSPACE}-${NAME}.yaml

kubectl apply -f tfstate-${WORKSPACE}-${NAME}.yaml
```
