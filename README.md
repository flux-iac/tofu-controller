# TF-controller: GitOps Terraform at your own pace

TF-controller is an experimental controller for [Flux](https://fluxcd.io) to reconcile Terraform resources
in the GitOps way.
With the power of Flux together with Terraform, TF-controller allows you to GitOps-ify infrastructure,
and application resources, in the Kubernetes and Terraform universe, at your own pace.

"At your own pace" means you don't need to GitOps-ify everything at once.

TF-controller offers many GitOps models:
  1. **Full GitOps Automation Model:** GitOps your Terraform resources from the provision steps to the enforcement steps, like a whole EKS cluster.
  2. **Hybrid GitOps Automation Model:** GitOps parts of your existing infrastructure resources. For example, you have an existing EKS cluster.
     You can choose to GitOps only its nodegroup, or its security group.
  3. **State Enforcement Model:** You have a TFSTATE file, and you'd like to use GitOps enforce it, without changing anything else.
  4. **Drift Detection Model:** You have a TFSTATE file, and you'd like to use GitOps just for drift detection, so you can decide to do things later when a drift occurs.

## Features

  * **Full GitOps Automation for Terraform**: With setting `.spec.approvePlan=auto`, it allows a `Terraform` object
    to be reconciled and act as the representation of your Terraform resources. The TF-controller uses the spec of
    the `Terraform` object to perform `plan`, `apply` its associated Terraform resources. It then stores
    the `TFSTATE` of the applied resources as a `Secret` inside the Kubernetes cluster. After `.spec.interval` passes,
    the controller performs drift detection to check if there is a drift occurred between your live system,
    and your Terraform resources. If a drift occurs, the plan to fix that drift will be generated and applied automatically.
    _This feature is available since v0.3.0._
  * **Drift detection**: This feature is a part of the GitOps automation feature. The controller detects and fixes drift
    for your infrastructures, based on the Terraform resources and their `TFSTATE`. _This feature is available since v0.5.0._
    * Drift detection is enabled by default. You can use the field `.spec.disableDriftDetection` to disable this behaviour.
      _This feature is available since v0.7.0._
    * The Drift detection only mode, without plan or apply steps, allows you to perform read-only drift detection.
      _This feature is available since v0.8.0._
  * **Plan and Manual Approve**: This feature allows you to separate the `plan`, out of the `apply` step, just like
    the Terraform workflow you are familiar with. A good thing about this is that it is done in a GitOps way. When a plan
    is generated, the controller shows you a message like **'set approvePlan: "plan-main-123" to apply this plan.'**.
    You make change to the field `.spec.approvePlan`, commit and push to tell the TF-controller to apply the plan for you.
    With this GitOps workflow, you can optionally create and push this change to a new branch for your team member to
    review and approve too. _This feature is available since v0.6.0._

## Dependencies

|  Version   | Terraform | Source Controller | Flux v2 |
|:----------:|:---------:|:-----------------:|:-------:|
| **v0.8.0** |  v1.1.4   | v0.20.1           | v0.25.x |
|   v0.7.0   |  v1.1.3   | v0.20.1           | v0.25.x |

## Installation

Before using TF-controller, you have to install Flux by using either `flux install` or `flux bootstrap` command.
After that you can install TF-controller manually with Helm by:

```shell
# Add tf-controller helm repository to local
helm repo add tf-controller https://tf-controller.github.io/charts/

# Install tf-controller
helm upgrade -i tf-controller tf-controller/tf-controller \
    --namespace flux-system
```

For details on configurable parameters of the TF-controller chart,
please see [chart readme](https://github.com/tf-controller/charts/blob/main/charts/tf-controller/README.md).

Alternatively, you can install TF-controller via `kubectl`:

```shell
export TF_CON_VER=v0.8.0
kubectl apply -f https://github.com/chanwit/tf-controller/releases/download/${TF_CON_VER}/tf-controller.crds.yaml
kubectl apply -f https://github.com/chanwit/tf-controller/releases/download/${TF_CON_VER}/tf-controller.rbac.yaml
kubectl apply -f https://github.com/chanwit/tf-controller/releases/download/${TF_CON_VER}/tf-controller.deployment.yaml
```

## Quick start

Here's a simple example of how to GitOps your Terraform resources with TF-controller and Flux.

### Define source

First, we need to define a Source controller's source (`GitRepostory`, or `Bucket`), for example:

```yaml
apiVersion: source.toolkit.fluxcd.io/v1beta1
kind: GitRepository
metadata:
  name: helloworld
  namespace: flux-system
spec:
  interval: 30s
  url: https://github.com/tf-controller/helloworld
  ref:
    branch: main
```

### The GitOps Automation mode

The GitOps automation mode could be enabled by setting `.spec.approvePlan=auto`. In this mode, Terraform resources will be planned,
and automatically applied for you.

```yaml
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  approvePlan: "auto"
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
```

### The manual mode: plan and manual apply

For the plan & manual approval workflow, please either set `.spec.approvePlan` to be the blank value, or omit the field.

```diff
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
+ approvePlan: "" # or you can omit this field
- approvePlan: "auto"
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
```

Then the controller will tell you how to use field `.spec.approvePlan` to approve the plan.
After making change and push, it will apply the plan to create real resources.

```diff
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: hello-world
  namespace: flux-system
spec:
+ approvePlan: "plan-main-b8e362c206" # first 8 digits of a commit hash is enough
- approvePlan: ""
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
```

### The drift detection only mode: plan and apply will be skipped

To only run drift detection, skipping the plan and apply stages, set `.spec.approvePlan` to `disable`.

```yaml
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: hello-world
  namespace: flux-system
spec:
  approvePlan: "disable"
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
```

### Disable Drift Detection

Drift detection is enabled by default. Use the `.spec.disableDriftDetection` field to disable:

```yaml
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  approvePlan: "auto"
  disableDriftDetection: true
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
```

### Use with AWS EKS IRSA

AWS Elastic Kubernetes Service (EKS) offers IAM Roles for Service Accounts (IRSA) as a mechanism by which to provide
credentials for the Terraform controller.

You can use `eksctl` to associate an OIDC provider with your EKS cluster, for example:

```shell
eksctl utils associate-iam-oidc-provider --cluster CLUSTER_NAME --approve
```

Then follow the instructions [here](https://docs.aws.amazon.com/eks/latest/userguide/create-service-account-iam-policy-and-role.html)
to add a trust policy to the IAM role which grants the necessary permissions for Terraform.
Please note that if you have installed the controller following the README, then the `namespace:serviceaccountname`
will be `flux-system:tf-controller`. You'll obtain a Role ARN to use in the next step.

Finally, annotate the ServiceAccount with the obtained Role ARN in your cluster:

```shell
kubectl annotate -n flux-system serviceaccount tf-controller eks.amazon.com/role-arn=ROLE_ARN
```

### Setting Terraform Variables

**This is a breaking change of the `v1alpha1` API.**
Users who are upgrading from TF-controller <= 0.7.0 require updating `varsFrom`,
from a single object:
```yaml
  varsFrom:
    kind: ConfigMap
    name: cluster-config
```
to be an array of object, like this:
```yaml
  varsFrom:
  - kind: ConfigMap
    name: cluster-config
```

You can pass variables to Terraform using the `vars` and `varsFrom` fields.

Inline variables can be set using `vars`. The `varsFrom` field accepts a list of ConfigMaps / Secrets.
You may use the `varsKeys` property of `varsFrom` to select specific keys from the input or omit this field
to select all keys from the input source.

Note that in the case of the same variable key being passed multiple times, the controller will use
the lattermost instance of the key passed to `varsFrom`.

```yaml
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  approvePlan: "auto"
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
  vars:
  - name: region
    value: us-east-1
  - name: env
    value: dev
  - name: instanceType
    value: t3-small
  varsFrom:
  - kind: ConfigMap
    name: cluster-config
    varsKeys:
    - nodeCount
    - instanceType
  - kind: Secret
    name: cluster-creds
```

The `vars` field supports HCL string, number, bool, object and list types. For example, the following variable can be populated using the accompanying Terraform spec:

```hcl
variable "cluster_spec" {
  type = object({
      region = string
      env = string
      node_count = number
      public = bool
  })
}
```

```yaml
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  approvePlan: "auto"
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
  vars:
  - name: cluster_spec
    value:
      region: us-east-1
      env: dev
      node_count: 10
      public: false
```

### Managing Terraform State

By default, `tf-controller` will use the [Kubernetes backend](https://www.terraform.io/language/settings/backends/kubernetes) to store the Terraform statefile in cluster.

The statefile is stored in a secret named: `tfstate-default-${secretSuffix}`. The default `suffix` will be the name of the Terraform resource, however you may override this setting using `.spec.backendConfig.secretSuffix`.

You can disable the backend

#### Backup the statefile

For the following `terraform` resources:

```bash
$ kubectl get terraform

NAME       READY     STATUS         AGE
my-stack   Unknown   Initializing   28s
```

We can export the state like this:
```bash
kubectl get secret tfstate-default-my-stack -ojsonpath='{.data.tfstate}' | base64 -d | gzip -d > terraform.tfstate
```

#### Restore the statefile

To restore the statefile or import an existing statefile we can use the following operation:

```bash
gzip terraform.tfstate

NAME=my-stack

kubectl create secret \
  generic tfstate-default-${NAME} \
  --from-file=tfstate=terraform.tfstate.gz \
  --dry-run=client -o=yaml \
  | yq e '.metadata.annotations["encoding"]="gzip"' - > tfstate-default-${NAME}.yaml

kubectl apply -f tfstate-default-${NAME}.yaml
```

### Health Checks

For some resources, it may be useful to perform health checks on them to verify that they are ready to accept connection before the terraform goes into `Ready` state:

```
# main.tf

output "rdsAddress" {
  value = "mydb.xyz.us-east-1.rds.amazonaws.com"
}

output "rdsPort" {
  value = "3306"
}

output "myappURL" {
  value = "https://example.com/"
}
```

```yaml
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: helloworld
  namespace: flux-system
spec:
  approvePlan: "auto"
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
  healthChecks:
    - name: rds
      type: tcp
      address: "{{.rdsAddress}}:{{.rdsPort}}" # uses standard Go package template format to parse outputs to url
      timeout: 10s # optional, defaults to 20s
    - name: myapp
      type: http
      url: "{{.myappURL}}"
      timeout: 5s
    - name: url_not_from_output
      type: http
      url: "https://example.org"
```

## Examples
  * A Terraform GitOps with Flux to automatically reconcile your [AWS IAM Policies](https://github.com/tf-controller/aws-iam-policies).
  * GitOps an existing EKS cluster, by partially import its nodegroup and manage it with TF-controller: [An EKS scaling example](https://github.com/tf-controller/eks-scaling).

## Stargazers over time

[![Stargazers over time](https://starchart.cc/chanwit/tf-controller.svg)](https://starchart.cc/chanwit/tf-controller)

## Roadmap

### Q1 2022
  * [x] Support the GitOps way to "apply"
  * [x] Drift detection
  * [x] Support auto-apply so that the reconciliation detect drifts and always make changes
  * [x] Interop with Kustomization controller's health checks
  * [ ] Terraform outputs as Kubernetes Secrets
  * [ ] Secret and ConfigMap as input variables
  * [ ] Support the GitOps way to "plan" / "re-plan"
  * [ ] Test coverage reaching 70%

### Q2 2022
  * [ ] Support a multi-tenant model
  * [ ] Interop with Notification controller's Events and Alert
  * [ ] Write back and show plan in PRs
  * [ ] Test coverage reaching 75%

### Q3 2022
  * [ ] Performance and scalability
  * [ ] Test coverage reaching 80%
