# Break the glass

## What is break the glass?

"Break the glass" refers to a troubleshooting mode specifically designed
to provide a manual solution when tofu-controller is not performing as expected. This feature is available in the Terraform controller *v0.15.0* and above.

~> **WARNING:** Please note that you cannot use this feature to fix the Terraform resources with `v1alpha1` version of the Terraform CRD.  It works only with `v1alpha2` version of the Terraform CRD.

~> **WARNING:** Please also make sure that you have enough privileges to exec pods in your namespaces. Otherwise, you will not be able to use this feature.

There are two primary methods of initiating this mode:

1. Using the `tfctl` command-line tool.
2. Setting the `spec.breakTheGlass` field to `true` in the Terraform object.

## Using `tfctl` to Break the Glass

In order to use this functionality, it needs to be enabled at the controller level; in order to do that, you can set the following Helm chart value to `true`:

```yaml
allowBreakTheGlass: true
```

After the feature is enabled, to start a one-time troubleshooting session, you can use the `tfctl break-glass` command. For instance:

```shell
tfctl break-glass hello-world
```

This command initiates a session that allows you to execute any Terraform command
to rectify the issues with your Terraform resources. It is noteworthy that this command
does not require setting the `spec.breakTheGlass` field to `true` in the Terraform object.

After resolving the issues, you can simply exit the shell. 
GitOps will then continue to reconcile the Terraform object.

## Break the glass with `spec.breakTheGlass` field

This feature is particularly useful for troubleshooting Terraform objects at their initialization stage or in situations with unexpected errors.
It is generally not recommended to use this mode routinely for fixing Terraform resources.

You can enable the 'Break the Glass' feature for every reconciliation by setting the `breakTheGlass` field to `true` in the `spec` of the Terraform object.

Here is a sample example:

```yaml
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: hello-world
  namespace: flux-system
spec:
  breakTheGlass: true
  interval: 1m
  path: ./
  sourceRef:
    kind: GitRepository
    name: helloworld
    namespace: flux-system
```
