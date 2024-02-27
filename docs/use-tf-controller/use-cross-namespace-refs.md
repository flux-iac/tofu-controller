# Using Cross-Namespace References

The Terraform CRD in the API for tofu-controller includes fields which are references to other objects:

| Name | Purpose |
|------|---------|
| .spec.sourceRef | Refers to a Flux source |
| .spec.dependsOn[*] | Each entry refers to a dependency |
| .spec.cliConfigSecretRef | Secret with `tf` config to use |

Branch Planner configuration can also have cross-namespace references:

| Name | Purpose |
|------|---------|
| .secretNamespace | Namespace of secret containing a GitHub token |
| .resources[*] | Each entry refers to a Terraform object to include in branch planning |

All of these can refer to an object in a namespace different to that of the Terraform object. However, giving access to objects in other namespaces is generally considered a security risk, so this is disallowed by default. Only references that mention the same namespace, or that omit the namespace field, will be accepted. References using a different namespace will cause tofu-controller to stop processing the Terraform object and put it in a non-Ready state.

To **allow** cross-namespace references, use the flag `--allow-cross-namespace-refs` with tofu-controller and the Branch Planner. When using the Helm chart to install or update tofu-controller and Branch Planner, the value `allowCrossNamespaceRefs` will allow cross-namespace references for both.
