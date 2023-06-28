# Logging Env Variables

A Terraform Runner uses two environment variables, `DISABLE_TF_LOGS` and `ENABLE_SENSITIVE_TF_LOGS`, to control the logging behavior of the Terraform execution.

To use these environment variables, they need to be set on each Terraform Runner pod where the Terraform code is being executed.
This can typically be done by adding them to the pod's environment variables in the Terraform Runner deployment configuration.

- The `DISABLE_TF_LOGS` variable, when set to "1", will disable all Terraform output logs to stdout and stderr.
- The `ENABLE_SENSITIVE_TF_LOGS` variable, when set to "1", will enable logging of sensitive Terraform data,
such as secret variables, to the local log. However, it is important to note that for the `ENABLE_SENSITIVE_TF_LOGS` to take effect,
the `DISABLE_TF_LOGS` variable must also be set to "1".

For more information on configuring the Terraform Runner and its environment variables,
please consult the documentation on [customizing runners](https://github.com/weaveworks/tf-controller/blob/main/docs/use_tf_controller/to_provision_resources_with_customized_Runner_Pods.md) within the Weave TF-controller.