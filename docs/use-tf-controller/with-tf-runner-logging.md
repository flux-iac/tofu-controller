# Control the logging behavior of Terraform Runner

A Terraform Runner uses two environment variables, `DISABLE_TF_LOGS` and `ENABLE_SENSITIVE_TF_LOGS`, to control the logging behavior of the Terraform execution.

To use these environment variables, they need to be set on each Terraform Runner pod where the Terraform code is being executed.
This can typically be done by adding them to the pod's environment variables in the Terraform Runner deployment configuration.

- The `DISABLE_TF_LOGS` variable, when set to "1", will disable all Terraform output logs to stdout and stderr.
- The `ENABLE_SENSITIVE_TF_LOGS` variable, when set to "1", will enable logging of sensitive Terraform data,
such as secret variables, to the local log. However, it is important to note that for the `ENABLE_SENSITIVE_TF_LOGS` to take effect,
the `DISABLE_TF_LOGS` variable must also be set to "1".

## The Default Logging Behavior
- By default, the logging level for the `tf-runner` is configured at the `info` level.
- The `DISABLE_TF_LOGS` variable is not activated as part of the default settings.
- The `ENABLE_SENSITIVE_TF_LOGS` variable remains inactive in the default configuration.
- Calls to `ShowPlan` and `ShowPlanRaw` on the runner are not logged by default.
- For `Plan` calls made on the runner, error messages are sanitized as a part of the default configuration.

For more information on configuring the Terraform Runner and its environment variables,
please consult the documentation on [customizing runners](provision-resources-with-customized-runner-pods.md) within the Weave TF-controller.
