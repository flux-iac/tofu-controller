# tfctl

`tfctl` is a command-line utility to help with tf-controller operations.

## Installation

You can download the `tfctl` binary via the GitHub releases page: [https://github.com/weaveworks/tf-controller/releases]()

```
Usage:
  tfctl [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  create      Create a Terraform resource
  delete      Delete a Terraform resource
  get         Get Terraform resources
  help        Help about any command
  install     Install the tf-controller
  plan        Plan a Terraform configuration
  reconcile   Trigger a reconcile of the provided resource
  resume      Resume reconciliation for the provided resource
  suspend     Suspend reconciliation for the provided resource
  uninstall   Uninstall the tf-controller
  version     Prints tf-controller and tfctl version information

Flags:
  -h, --help                help for tfctl
      --kubeconfig string   Path to the kubeconfig file to use for CLI requests.
  -n, --namespace string    The kubernetes namespace to use for CLI requests. (default "flux-system")
      --terraform string    The location of the terraform binary. (default "/usr/bin/terraform")

Use "tfctl [command] --help" for more information about a command.
```
