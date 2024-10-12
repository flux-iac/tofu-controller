# Use tofu-controller to provision Terraform resources that are required health checks

For some Terraform resources, it may be useful to perform health checks on them to verify that they are ready to accept connection before the terraform goes into `Ready` state:

For example, our Terraform file is provisioned and contains the following outputs.

```hcl
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

We can use standard Go template expressions, like `${{ .rdsAddress }}`, to refer to those output values and use them to verify that the resources are up and running.

We support two types of health checks, `tcp` amd `http`. The `tcp` type allows us to verify a TCP connection, while the `http` type is for verify an HTTP URL. The default timeout of each health check is 20 seconds.

```yaml hl_lines="14-25"
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
  healthChecks:
    - name: rds
      type: tcp
      address: ${{ .rdsAddress }}:${{ .rdsPort }} 
      timeout: 10s # optional, defaults to 20s
    - name: myapp
      type: http
      url: ${{ .myappURL }}
      timeout: 5s
    - name: url_not_from_output
      type: http
      url: "https://example.org"
```
