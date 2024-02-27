# Use tofu-controller with AWS EKS IRSA

AWS Elastic Kubernetes Service (EKS) offers IAM Roles for Service Accounts (IRSA) as a mechanism by which to provide
credentials to Kubernetes pods. This can be used to provide the required AWS credentials to Terraform runners
for performing plans and applies.

You can use `eksctl` to associate an OIDC provider with your EKS cluster. For example:

```shell
eksctl utils associate-iam-oidc-provider --cluster CLUSTER_NAME --approve
```

Then follow the instructions [here](https://docs.aws.amazon.com/eks/latest/userguide/create-service-account-iam-policy-and-role.html)
to add a trust policy to the IAM role which grants the necessary permissions for Terraform.
If you have installed tofu-controller following the README, then the `namespace:serviceaccountname`
will be `flux-system:tf-runner`. You'll obtain a Role ARN to use in the next step.

Finally, annotate the ServiceAccount for the `tf-runner` with the obtained Role ARN in your cluster:

```shell
kubectl annotate -n flux-system serviceaccount tf-runner eks.amazonaws.com/role-arn=ROLE_ARN
```

If deploying the `tofu-controller` via Helm, do this as follows:

```yaml hl_lines="5"
values:
  runner:
    serviceAccount:
      annotations:
        eks.amazonaws.com/role-arn: ROLE_ARN
```
