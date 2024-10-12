# tfctl

`tfctl` is a command-line utility to help with tofu-controller operations.

## Installation

To install `tfctl` via Homebrew, run the following command:

```shell
brew install weaveworks/tap/tfctl
```

You can also download the `tfctl` binary via the GitHub releases page: [https://github.com/flux-iac/tofu-controller/releases](https://github.com/flux-iac/tofu-controller/releases).

```
Usage:
  tfctl [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  create      Create a Terraform resource
  delete      Delete a Terraform resource
  get         Get Terraform resources
  help        Help about any command
  install     Install the tofu-controller
  plan        Plan a Terraform configuration
  reconcile   Trigger a reconcile of the provided resource
  resume      Resume reconciliation for the provided resource
  suspend     Suspend reconciliation for the provided resource
  uninstall   Uninstall the tofu-controller
  version     Prints tofu-controller and tfctl version information

Flags:
  -h, --help                help for tfctl
      --kubeconfig string   Path to the kubeconfig file to use for CLI requests.
  -n, --namespace string    The kubernetes namespace to use for CLI requests. (default "flux-system")
      --terraform string    The location of the terraform binary. (default "/usr/bin/terraform")

Use "tfctl [command] --help" for more information about a command.
```

## Shell completion

It works the same way as flux CLI:

With **bash**:

```shell
# ~/.bashrc or ~/.profile
command -v tfctl >/dev/null && . <(tfctl completion bash)
```

With **fish**:

```shell
tfctl completion fish > ~/.config/fish/completions/tfctl.fish
```

With **powershell**:

```shell
# Windows

cd "$env:USERPROFILE\Documents\WindowsPowerShell\Modules"
tfctl completion powershell >> tfctl-completion.ps1

# Linux

cd "${XDG_CONFIG_HOME:-"$HOME/.config/"}/powershell/modules"
tfctl completion powershell >> tfctl-completions.ps1
```

With **zsh**:

```shell
# ~/.zshrc or ~/.profile
command -v tfctl >/dev/null && . <(tfctl completion zsh)
```
