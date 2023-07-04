# Use TF-controller to set variables for Terraform resources

~> **BREAKING CHANGE**: This is a breaking change of the `v1alpha1` API.

Users who are upgrading from TF-controller <= 0.7.0 require updating `varsFrom`,
from a single object:

```yaml hl_lines="2"
  varsFrom:
    kind: ConfigMap
    name: cluster-config
```

to be an array of object, like this:

```yaml hl_lines="2"
  varsFrom:
  - kind: ConfigMap
    name: cluster-config
```

## `vars` and `varsFrom`

You can pass variables to Terraform using the `vars` and `varsFrom` fields.

Inline variables can be set using `vars`. The `varsFrom` field accepts a list of ConfigMaps / Secrets.
You may use the `varsKeys` property of `varsFrom` to select specific keys from the input or omit this field
to select all keys from the input source.

Note that in the case of the same variable key being passed multiple times, the controller will use
the lattermost instance of the key passed to `varsFrom`.

```yaml hl_lines="15-20 22-28"
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

## Variable value as HCL

The `vars` field supports HCL string, number, bool, object and list types. For example, the following variable can be populated using the accompanying Terraform spec:

```hcl hl_lines="3-6"
variable "cluster_spec" {
  type = object({
      region     = string
      env        = string
      node_count = number
      public     = bool
  })
}
```

```yaml hl_lines="17-20"
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
  vars:
  - name: cluster_spec
    value:
      region: us-east-1
      env: dev
      node_count: 10
      public: false
```
