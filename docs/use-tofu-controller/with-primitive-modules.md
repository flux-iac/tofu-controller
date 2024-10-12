# Use tofu-controller with primitive modules

This document describes how to use tofu-controller with a primitive module.
It requires tofu-controller v0.13+ to run the example.

## What is a primitive module?

It's a Terraform module that contains only a single resource.

  * A Terraform primitive module must contains the "values" variable.
  * The "values" variable must be an object with fields of optional types.
  * The module must be placed under a directory, which is named after the resource.
  * The directory can optionally contain other files, for example the .terraform.lock.hcl.
  * We call a set of primitive modules bundled into an OCI image, a package.

## Hello World Primitive Module

Here is an example of how a primitive module can be defined in YAML.
Assume that we have a ready-to-use OCI image with a primitive module for the imaginary resource `aws_hello_world`,
and the image is tagged as `ghcr.io/flux-iac/hello-primitive-modules/v4.32.0:v1`.

We'll use the following Terraform object definition to provision the resource.

First, we need to create a Terraform object with the `spec.sourceRef.kind` field 
set to `OCIRepository` and the `spec.sourceRef.name` field set to the name of the OCIRepository object.

Second, we need to set the `spec.path` field to the name of the resource, in this case `aws_hello_world`.

Third, we need to set the `spec.values` field to the values of the resource. This is a YAML object that will be converted to an HCL variable, and passed to the Terraform module.

Finally, we need to set the `spec.approvePlan` field to `auto` to automatically approve the plan.

```yaml hl_lines="19-25"
---
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: OCIRepository
metadata:
  name: hello-package-v4.32.0
  namespace: flux-system
spec:
  interval: 30s
  url: oci://ghcr.io/flux-iac/hello-primitive-modules/v4.32.0
  ref:
    tag: v1
---
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: hello-world
  namespace: flux-system
spec:
  path: aws_hello_world
  values:
    greeting: Hi
    subject: my world
  sourceRef:
    kind: OCIRepository
    name: hello-package-v4.32.0
  interval: 1h0m
  approvePlan: auto
```
