# Upgrading Tofu Controller

Please follow these steps to upgrade Tofu Controller:

1. Read the latest release changelogs.
2. Check your API versions.
3. To make sure you don't get new state changes, suspend Terraform resources (`tfctl suspend --all`) to minimize the impact on live systems.
4. Back up Terraform tfstates to avoid losing data. If you're using the default backend with secrets in Kubernetes, use your backup toolset (i.e., Velero) to back up the state data.
5. Upgrade Flux first, following [the Flux documentation](https://fluxcd.io/flux/installation/upgrade/).
6. Disable [auto-approval](https://flux-iac.github.io/tofu-controller/use-tf-controller/provision-resources-and-auto-approve/) by either removing the approvePlan value or setting it to "".
7. To prevent unintentional resource deletions, set the `spec.destroyResourcesOnDeletion` flag to `false` for critical or production systems (the default value is `false`)
8. If the Flux upgrade goes well, proceed to upgrade the Tofu Controller via its image tag. Adjust the values in the HelmRelease to match the new version to which you are upgrading.
9. Check the pod logs for the Tofu Controller deployment and any runner logs in order to identify potential issues. If you check the `warnings` in the logs, you can also identify any required API changes.
10. Push the changes you made.
11. Resume your Terraform resources—either one-by-one for critical resources, or all of them with `tfctl resume --all`
12. Ensure no changes are planned for deletion. If you changed the value in step 6 from `spec.destroyResourcesOnDeletion` to `false`, resources will not be automatically removed.
13. Revert back to auto-approval mode after ensuring stability.
14. Resume any suspended Kustomization objects to restore GitOps automation.
15. Restore `spec.destroyResourcesOnDeletion`, if this has been disabled for any resources in critical or production systems.

## Removal of the v1alpha1 API

The `v1alpha1` Terraform API was deprecated in 2023 and is **removed in
v0.17.0**. From v0.17.0 the CRD only defines `v1alpha2`. Do the following
**before** upgrading to v0.17.0 or later. If your HelmRelease uses a chart
version range such as `>=0.16.6`, it upgrades automatically when v0.17.0
is published, so pin it to `0.16.x` until these steps are done:

- Change every manifest that still uses `infra.contrib.fluxcd.io/v1alpha1`,
  including those in Git, to `infra.contrib.fluxcd.io/v1alpha2`. Otherwise
  applying them fails with `no matches for kind "Terraform" in version
  "infra.contrib.fluxcd.io/v1alpha1"`.
- Follow the two steps below to clear any references to `v1alpha1` left in the
  cluster.

### 1. Clear stale `v1alpha1` managed fields

If a Terraform object was ever written via `v1alpha1` (for example by an old
`tfctl`), its `metadata.managedFields` keeps a `v1alpha1` entry. Server-side
apply of that object then fails, for example in a Flux Kustomization, with
errors such as `.spec.retryStrategy: field not declared in schema` or, after
the upgrade, `request to convert CR to an invalid group/version`. The fix below
works both before and after the upgrade.

Find the affected objects:

```shell
kubectl get terraforms -A -o json --show-managed-fields | jq -r '.items[]
  | select(any(.metadata.managedFields[]?; .apiVersion == "infra.contrib.fluxcd.io/v1alpha1"))
  | "\(.metadata.namespace) \(.metadata.name)"'
```

Reset the managed fields of each one. The next server-side apply (for example
by kustomize-controller, which force-applies) takes ownership again:

```shell
kubectl -n <namespace> patch terraform <name> --type=json \
  -p '[{"op":"replace","path":"/metadata/managedFields","value":[{}]}]'
```

### 2. Remove `v1alpha1` from the CRD's stored versions

Clusters that ran Tofu Controller before `v1alpha2` became the storage version
may still list `v1alpha1` in the CRD status. Kubernetes refuses to remove a
version from a CRD while it is listed there, so the CRD update in the upgrade
fails until this is done, with an error starting
`status.storedVersions[0]: Invalid value: "v1alpha1"`.
Check:

```shell
kubectl get crd terraforms.infra.contrib.fluxcd.io -o jsonpath='{.status.storedVersions}'
```

If the output includes `v1alpha1`, rewrite every Terraform object so it is
stored as `v1alpha2`, then update the CRD status:

```shell
kubectl get terraforms -A -o json | kubectl replace -f -
kubectl patch crd terraforms.infra.contrib.fluxcd.io --subresource=status --type=merge \
  -p '{"status":{"storedVersions":["v1alpha2"]}}'
```

If `kubectl replace` reports a conflict for an object, the controller updated
it at the same time; run the command again.

