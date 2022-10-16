# Use TF-controller with GitOps dependency management

TF-controller supports GitOps dependency management.
The GitOps dependency management feature is based on the Kustomization controller of Flux.

This means that you can use TF-controller to provision resources that depend on other resources at the GitOps level.
For example, you can use TF-controller to provision an S3 bucket, and then use TF-controller to provision another resource to configure ACL for that bucket.

## Create a Terraform object

Similar to the same feature in the Kustomization controller, the dependency management feature is enabled by setting the `dependsOn` field in the `Terraform` object.
The `dependsOn` field is a list of `Terraform` objects.

First, create a `Terraform` object to provision the S3 bucket, name it `aws-s3-bucket`.
The S3 bucket is provisioned by the Terraform module `aws_s3_bucket` in the OCI image `aws-package-v4.33.0`.
It is configured to use the `auto-apply` mode, and write outputs to the secret `aws-s3-bucket-outputs`.

```yaml hl_lines="20-24"
---
apiVersion: infra.contrib.fluxcd.io/v1alpha1
kind: Terraform
metadata:
  name: aws-s3-bucket
  namespace: flux-system
spec:
  path: aws_s3_bucket
  values:
    bucket: my-tf-controller-test-bucket
    tags:
      Environment: Dev
      Name: My bucket
  sourceRef:
    kind: OCIRepository
    name: aws-package-v4.33.0
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
apiVersion: infra.contrib.fluxcd.io/v1alpha1
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
    name: aws-package-v4.33.0
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
