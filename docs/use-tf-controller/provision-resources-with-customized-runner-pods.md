# Use TF-Controller to provision resources with customized Runner Pods

## Customize Runner Pod metadata

Sometimes you need to add custom labels and annotations to the runner pod used to reconcile Terraform.
For example, for Azure AKS to grant pod active directory permissions using Azure Active Directory (AAD) Pod Identity,
a label like `aadpodidbinding: myIdentity` on the pod is required.

```yaml
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
  runnerPodTemplate:
    metadata:
      labels:
        aadpodidbinding: myIdentity
      annotations:
        company.com/abc: xyz
```

## Customize Runner Pod Image

By default, the Terraform controller uses `RUNNER_POD_IMAGE` environment variable to identify the Runner Pod's image to use. You can customize the image on the global level by updating the value of the environment variable or, you can specify an image to use per Terraform object for its reconciliation.

```yaml
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
  runnerPodTemplate:
    spec:
      image: registry.io/tf-runner:xyz
```

You can use [`runner.Dockerfile`](https://github.com/flux-iac/tofu-controller/blob/main/runner.Dockerfile) as a basis of customizing runner pod image.

## Customize Runner Pod Specifications

You can also customize various Runner Pod `spec` fields to control and configure how the Runner Pod runs. 
For example, you can configure Runner Pod `spec` affinity and tolerations if you need to run in on a specific set of nodes. Please see [RunnerPodSpec](https://weaveworks.github.io/tf-controller/References/terraform/#infra.contrib.fluxcd.io/v1alpha2.RunnerPodSpec) for a list of the configurable Runner Pod `spec` fields.
