# Upgrading TF-Controller

Please follow these steps to upgrade TF-Controller:

1. Read the latest release changelogs.
2. Check your API versions.
3. To make sure you don't get new state changes, suspend Terraform resources (`tfctl suspend --all`) to minimize the impact on live systems.
4. Back up Terraform tfstates to avoid losing data. If you're using the default backend with secrets in Kubernetes, use your backup toolset (i.e., Velero) to back up the state data.
5. Upgrade Flux first, following [the Flux documentation](https://fluxcd.io/flux/installation/upgrade/).
6. To prevent unintentional resource deletions, set the `spec.destroyResourcesOnDeletion` flag to `False` for critical or production systems.
7. If the Flux upgrade goes well, proceed to upgrade the TF-controller via its image tag. Adjust the values in the HelmRelease to match the new version to which you are upgrading.
8. Check your system logs to identify any potential issues.
9. Push the changes you made.
10. Resume your Terraform resources—either one-by-one for critical resources, or all of them with `tfctl resume --all`
11. Ensure no changes are planned for deletion. TF-Controller has a flag to help prevent the deletion: `spec.destroyResourcesOnDeletion`. This is set to `false` by default.
12. Revert back to auto-approval mode after ensuring stability.
13. Resume any suspended Kustomization objects to restore GitOps automation.

TF-Controller supports v1alpha1 for backward compatibility. This means that you need v1alpha2 for newer (as of September 2023) features such as:
- the branch planner
- pod sub-domain DNS resolutions
- new PodSpec fields like PriorityClass, SecurityContext, and ResourceRequirements (Limits / Requests)
