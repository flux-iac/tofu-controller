# Use tofu-controller to provision resources and obtain outputs

Outputs created by Terraform can be written to a secret using `.spec.writeOutputsToSecret`.

## Write all outputs

We can specify a target secret in `.spec.writeOutputsToSecret.name`, and the controller will write all outputs to the secret by default.

```yaml hl_lines="14-15"
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  approvePlan: auto
  interval: 1m
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
  writeOutputsToSecret:
    name: helloworld-output
```

## Write outputs selectively

Choose only a subset of outputs by specifying output names you'd like to write in the `.spec.writeOutputsToSecret.outputs` array.

```yaml hl_lines="16-18"
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  approvePlan: auto
  interval: 1m
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
  writeOutputsToSecret:
    name: helloworld-output
    outputs:
    - hello_world
    - my_sensitive_data
```

## Rename outputs

Some time we'd like to use rename an output, so that it can be consumed by other Kubernetes controllers.
For example, we might retrieve a key from a Secret manager, and it's an AGE key, which must be ending with ".agekey" in the secret. In this case, we need to rename the output. 

Tofu-controller supports mapping output names using the `old_name:new_name` format.

In the following example, we renamed `age_key` output as `age.agekey` entry for the `helloworld-output` secret's data, so that other components in the GitOps pipeline could consume it.

```yaml hl_lines="16-17"
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  approvePlan: auto
  interval: 1m
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
  writeOutputsToSecret:
    name: helloworld-output
    outputs:
    - age_key:age.agekey
```
## Customize metadata of the outputted secret

Some situations require adding custom labels and annotations to the outputted secret.
As an example, operators such as [kubernetes-replicator](https://github.com/mittwald/kubernetes-replicator)
allow replicating secrets from one namespace to another but use annotations to do so.

```yaml hl_lines="16-19"
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  approvePlan: auto
  interval: 1m
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
  writeOutputsToSecret:
    name: helloworld-output
    labels:
      my-label: true
    annotations:
      my-annotation: "very long string"
      
```
