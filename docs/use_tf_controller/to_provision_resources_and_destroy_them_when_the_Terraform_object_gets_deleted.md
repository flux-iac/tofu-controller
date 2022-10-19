# Use TF-controller to provision resources and destroy them when the Terraform object gets deleted

The resources provisioned by a Terraform object are not destroyed by default, and the tfstate of that Terraform object still remains in the cluster.

It means that you are safe to delete the Terraform object in the cluster and re-create it. 
If you re-create a new Terraform object with the same name, namespace and workspace, it will continue to use the tfstate inside the cluster as the starting point to renconcle.

However, you may want to destroy provisioned resources when delete the Terraform object in many scenarios.
To enable destroy resources on object deletion, set `.spec.destroyResourcesOnDeletion` to `true`.

~> **WARNING:** This feature will destroy your resources on the cloud if the Terraform object gets deleted. Please use it with cautions.

```yaml hl_lines="8"
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  approvePlan: auto
  destroyResourcesOnDeletion: true
  interval: 1m
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
```
