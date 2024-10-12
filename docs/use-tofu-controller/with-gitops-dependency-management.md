# Use tofu-controller with GitOps dependency management

Tofu-controller supports GitOps dependency management.
The GitOps dependency management feature is based on the similar technique implemented in the Kustomization controller of Flux.

This means that you can use tofu-controller to provision resources that depend on other resources at the GitOps level.
For example, you can use tofu-controller to provision an S3 bucket, and then use tofu-controller to provision another resource to configure ACL for that bucket.

GitOps dependency management is different from Terraform's HCL dependency management in the way that it is not based on Terraform's mechanism, which is controlled by the Terraform binary.
Instead, it is implemented at the controller level, which means that each Terraform module is reconciled and can be managed independently, while still being able to depend on other modules.

## Create a Terraform object

Similar to the same feature in the Kustomization controller, the dependency management feature is enabled by setting the `dependsOn` field in the `Terraform` object.
The `dependsOn` field is a list of `Terraform` objects.

When the dependency is not satisfied, the Terraform object will be in the `Unknown` state, and it will be retry again every `spec.retryInterval`.
The retry interval is same as the `spec.interval` by default, and it can be configured separately by setting the `spec.retryInterval` field.

First, create a `Terraform` object to provision the S3 bucket, name it `aws-s3-bucket`.
The S3 bucket is provisioned by the Terraform module `aws_s3_bucket` in the OCI image `aws-package`.
It is configured to use the `auto-apply` mode, and write outputs to the secret `aws-s3-bucket-outputs`.

```yaml hl_lines="20-24"
---
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: aws-s3-bucket
  namespace: flux-system
spec:
  path: aws_s3_bucket
  values:
    bucket: my-tofu-controller-test-bucket
    tags:
      Environment: Dev
      Name: My bucket
  sourceRef:
    kind: OCIRepository
    name: aws-package
  approvePlan: auto
  interval: 2m
  destroyResourcesOnDeletion: true
  writeOutputsToSecret:
    name: aws-s3-bucket-outputs
    outputs:
    - arn
    - bucket
  runnerPodTemplate:
    spec:
      envFrom:
      - secretRef:
          name: aws-credentials
```

Second, create a `Terraform` object to configure ACL for the S3 bucket, name it `aws-s3-bucket-acl`.
The ACL is provisioned by the Terraform module `aws_s3_bucket_acl`, also from the OCI image `aws-package-v4.33.0`.

In the `dependsOn` field, specify the `Terraform` object that provisions the S3 bucket.
This means that the ACL will be configured only after the S3 bucket is provisioned, and has its outputs Secret written.
We can read the outputs of the S3 bucket from the Secret `aws-s3-bucket-outputs`, by specifying the `spec.readInputsFromSecrets` field.
The `spec.readInputsFromSecrets` field is a list of Secret objects. 
Its `name` field is the name of the Secret, and its `as` field is the name of variable that can be used in the `spec.values` block.

For example, the `spec.values.bucket` field in the `aws-s3-bucket-acl` Terraform object is set to `${{ .aws_s3_bucket.bucket }}`.

Please note that we use `${{` and  `}}` as the delimiters for the variable name, instead of the Helm default ones, `{{` and `}}`.

```yaml hl_lines="11 18 20-21"
---
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: aws-s3-bucket-acl
  namespace: flux-system
spec:
  path: aws_s3_bucket_acl
  values:
    acl: private
    bucket: ${{ .aws_s3_bucket.bucket }}
  sourceRef:
    kind: OCIRepository
    name: aws-package
  approvePlan: auto
  interval: 3m
  dependsOn:
  - name: aws-s3-bucket
  readInputsFromSecrets:
  - name: aws-s3-bucket-outputs
    as: aws_s3_bucket
  runnerPodTemplate:
    spec:
      envFrom:
      - secretRef:
          name: aws-credentials
```

## Avoid Kustomization controller's variable substitution

The Kustomization controller will substitute variables in the `Terraform` object, which will cause conflicts with the variable substitution in the GitOps dependency management feature.
To avoid this, we need to add the `kustomize.toolkit.fluxcd.io/substitute: disabled` annotation to the `Terraform` object.

```yaml hl_lines="8"
---
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: aws-s3-bucket-acl
  namespace: flux-system
  annotations:
    kustomize.toolkit.fluxcd.io/substitute: disabled
spec:
  path: aws_s3_bucket_acl
  values:
    acl: private
    bucket: ${{ .aws_s3_bucket.bucket }}
  sourceRef:
    kind: OCIRepository
    name: aws-package
  approvePlan: auto
  interval: 3m
  dependsOn:
  - name: aws-s3-bucket
  readInputsFromSecrets:
  - name: aws-s3-bucket-outputs
    as: aws_s3_bucket
  runnerPodTemplate:
    spec:
      envFrom:
      - secretRef:
          name: aws-credentials
```
