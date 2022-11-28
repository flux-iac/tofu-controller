## Use TF-controller with the ready-to-use AWS package

This document describes how to use the Weave TF-controller with the ready-to-use AWS package.
It requires TF-controller v0.13+ to run the example.

## What is a package?

A package is a collection of primitive Terraform modules that are bundled into an OCI image.
You can think of a TF-controller's package as a thin wrapper around a Terraform module provider,
and a TF-controller primitive module as a thin wrapper around a Terraform resource or a root module.

We will provide a set of ready-to-use packages for the most popular cloud providers.
Currently, we ship the AWS package only.

## AWS Package

Here is an example of how a package can be used in YAML.
Assume that we have a ready-to-use OCI image with the AWS package, and the image is tagged as `ghcr.io/tf-controller/aws/v4.32.0:v1`.

We'll use the following Terraform object definition to provision the resource.

### AWS S3 Bucket

First, we need to create a Terraform object with the `spec.sourceRef.kind` field
set to `OCIRepository` and the `spec.sourceRef.name` field set to the name of the OCIRepository object.



### AWS IAM Policy
