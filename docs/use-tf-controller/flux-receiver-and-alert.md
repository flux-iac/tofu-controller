# Integrate with Flux Receivers and Alerts

You can customize your Flux installation to use Flux API
resources like `Receivers` and `Alerts` with third-party custom
resource definitions such as the `Terraform` API CRD.

You will need to add a patch to the `kustomization.yaml` in your Flux cluster
installation's bootstrap manifests. Find it under the `flux-system` directory.

<!-- <a id="enable-notifications-for-third-party-controllers">&nbsp;</a> -->
### Enable Notifications for Third-Party Controllers

Enable notifications for 3rd party Flux controllers such as [tf-controller](https://github.com/flux-iac/tofu-controller):

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - gotk-components.yaml
  - gotk-sync.yaml
patches:
  - patch: |
      # v1beta1
      - op: add
        path: /spec/versions/0/schema/openAPIV3Schema/properties/spec/properties/eventSources/items/properties/kind/enum/-
        value: Terraform
      # v1beta2
      - op: add
        path: /spec/versions/1/schema/openAPIV3Schema/properties/spec/properties/eventSources/items/properties/kind/enum/-
        value: Terraform
      # v1beta3
      - op: add
        path: /spec/versions/2/schema/openAPIV3Schema/properties/spec/properties/eventSources/items/properties/kind/enum/-
        value: Terraform
    target:
      kind: CustomResourceDefinition
      name:  alerts.notification.toolkit.fluxcd.io
  - patch: |
      # v1
      - op: add
        path: /spec/versions/0/schema/openAPIV3Schema/properties/spec/properties/resources/items/properties/kind/enum/-
        value: Terraform
      # v1beta1
      - op: add
        path: /spec/versions/1/schema/openAPIV3Schema/properties/spec/properties/resources/items/properties/kind/enum/-
        value: Terraform
      # v1beta2
      - op: add
        path: /spec/versions/2/schema/openAPIV3Schema/properties/spec/properties/resources/items/properties/kind/enum/-
        value: Terraform
    target:
      kind: CustomResourceDefinition
      name:  receivers.notification.toolkit.fluxcd.io
  - patch: |
      - op: add
        path: /rules/-
        value:
          apiGroups: [ 'infra.contrib.fluxcd.io' ]
          resources: [ '*' ]
          verbs: [ '*' ]
    target:
      kind: ClusterRole
      name:  crd-controller-flux-system
```

Each version of the Flux `Alert` and `Receiver` CRDs must be patched, so the JSON6902 patch statement should be repeated with a different `versions` index.

The list of CRD versions on the cluster can be queried with Kubectl.
For example, to use `jq` to get the count of versions for the `Alert` CRD:
```sh
kubectl get -ojson crd alerts.notification.toolkit.fluxcd.io | jq '.spec.versions | length'
```
