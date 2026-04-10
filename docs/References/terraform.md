
# API Reference

## Packages
- [infra.contrib.fluxcd.io/v1alpha2](#infracontribfluxcdiov1alpha2)


## infra.contrib.fluxcd.io/v1alpha2


Package v1alpha2 contains API Schema definitions for the infra v1alpha2 API group


### Resource Types
- [Terraform](#terraform)

### BackendConfigSpec

BackendConfigSpec is for specifying configuration for Terraform's Kubernetes backend

_Appears in:_
- [TerraformSpec](#terraformspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `disable` _boolean_ | Disable is to completely disable the backend configuration. |  | Optional: \{\} <br /> |
| `secretSuffix` _string_ |  |  | Optional: \{\} <br /> |
| `inClusterConfig` _boolean_ |  |  | Optional: \{\} <br /> |
| `customConfiguration` _string_ |  |  | Optional: \{\} <br /> |
| `configPath` _string_ |  |  | Optional: \{\} <br /> |
| `labels` _object (keys:string, values:string)_ |  |  | Optional: \{\} <br /> |


### BackendConfigsReference

_Appears in:_
- [TerraformSpec](#terraformspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kind` _string_ | Kind of the values referent, valid values are ('Secret', 'ConfigMap'). |  | Enum: [Secret ConfigMap] <br />Required: \{\} <br /> |
| `name` _string_ | Name of the configs referent. Should reside in the same namespace as the<br />referring resource. |  | MaxLength: 253 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `keys` _string array_ | Keys is the data key where a specific value can be found at. Defaults to all keys. |  | Optional: \{\} <br /> |
| `optional` _boolean_ | Optional marks this BackendConfigsReference as optional. When set, a not found error<br />for the values reference is ignored, but any Key or<br />transient error will still result in a reconciliation failure. |  | Optional: \{\} <br /> |


### BranchPlanner

_Appears in:_
- [TerraformSpec](#terraformspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enablePathScope` _boolean_ | EnablePathScope specifies if the Branch Planner should or shouldn't check<br />if a Pull Request has changes under `.spec.path`. If enabled extra<br />resources will be created only if there are any changes in terraform files. |  | Optional: \{\} <br /> |


### CloudSpec

_Appears in:_
- [TerraformSpec](#terraformspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `organization` _string_ |  |  | Required: \{\} <br /> |
| `workspaces` _[CloudWorkspacesSpec](#cloudworkspacesspec)_ |  |  | Required: \{\} <br /> |
| `hostname` _string_ |  |  | Optional: \{\} <br /> |
| `token` _string_ |  |  | Optional: \{\} <br /> |


### CloudWorkspacesSpec

_Appears in:_
- [CloudSpec](#cloudspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  | Optional: \{\} <br /> |
| `tags` _string array_ |  |  | Optional: \{\} <br /> |


### CrossNamespaceSourceReference

CrossNamespaceSourceReference contains enough information to let you locate the
typed Kubernetes resource object at cluster level.

_Appears in:_
- [TerraformSpec](#terraformspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | API version of the referent. |  | Optional: \{\} <br /> |
| `kind` _string_ | Kind of the referent. |  | Enum: [GitRepository Bucket OCIRepository] <br />Required: \{\} <br /> |
| `name` _string_ | Name of the referent. |  | Required: \{\} <br /> |
| `namespace` _string_ | Namespace of the referent, defaults to the namespace of the Kubernetes resource object that contains the reference. |  | Optional: \{\} <br /> |


### FileMapping

_Appears in:_
- [TerraformSpec](#terraformspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `secretRef` _[SecretKeyReference](https://pkg.go.dev/github.com/fluxcd/pkg/apis/meta#SecretKeyReference)_ | Reference to a Secret that contains the file content |  |  |
| `location` _string_ | Location can be either user's home directory or the Terraform workspace |  | Enum: [home workspace] <br />Required: \{\} <br /> |
| `path` _string_ | Path of the file - relative to the "location" |  | Pattern: `^(.?[/_a-zA-Z0-9]\{1,\})*$` <br />Required: \{\} <br /> |


### ForceUnlockEnum

_Underlying type:_ _string_

_Appears in:_
- [TFStateSpec](#tfstatespec)

| Value | Description |
| --- | --- |
| `auto` |  |
| `yes` |  |
| `no` |  |


### HealthCheck

HealthCheck contains configuration needed to perform a health check after
terraform is applied.

_Appears in:_
- [TerraformSpec](#terraformspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the health check. |  | MaxLength: 253 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `type` _string_ | Type of the health check, valid values are ('tcp', 'http').<br />If tcp is specified, address is required.<br />If http is specified, url is required. |  | Enum: [tcp http] <br />Required: \{\} <br /> |
| `url` _string_ | URL to perform http health check on. Required when http type is specified.<br />Go template can be used to reference values from the terraform output<br />(e.g. https://example.org, \{\{.output_url\}\}). |  | Optional: \{\} <br /> |
| `address` _string_ | Address to perform tcp health check on. Required when tcp type is specified.<br />Go template can be used to reference values from the terraform output<br />(e.g. 127.0.0.1:8080, \{\{.address\}\}:\{\{.port\}\}). |  | Optional: \{\} <br /> |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#duration-v1-meta)_ | The timeout period at which the connection should timeout if unable to<br />complete the request.<br />When not specified, default 20s timeout is used. | 20s | Optional: \{\} <br /> |


### LockStatus

LockStatus defines the observed state of a Terraform State Lock

_Appears in:_
- [TerraformStatus](#terraformstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `lastApplied` _string_ |  |  | Optional: \{\} <br /> |
| `pending` _string_ | Pending holds the identifier of the Lock Holder to be used with Force Unlock |  | Optional: \{\} <br /> |


### PlanStatus

_Appears in:_
- [TerraformStatus](#terraformstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `lastApplied` _string_ |  |  | Optional: \{\} <br /> |
| `pending` _string_ |  |  | Optional: \{\} <br /> |
| `isDestroyPlan` _boolean_ |  |  | Optional: \{\} <br /> |
| `isDriftDetectionPlan` _boolean_ |  |  | Optional: \{\} <br /> |


### ReadInputsFromSecretSpec

_Appears in:_
- [TerraformSpec](#terraformspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  | Required: \{\} <br /> |
| `as` _string_ |  |  | Required: \{\} <br /> |


### Remediation

_Appears in:_
- [TerraformSpec](#terraformspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `retries` _integer_ | Retries is the number of retries that should be attempted on failures<br />before bailing. Defaults to '0', a negative integer denotes unlimited<br />retries. |  | Optional: \{\} <br /> |


### ResourceInventory

ResourceInventory contains a list of Kubernetes resource object references that have been applied by a Kustomization.

_Appears in:_
- [TerraformStatus](#terraformstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `entries` _[ResourceRef](#resourceref) array_ | Entries of Kubernetes resource object references. |  |  |


### ResourceRef

ResourceRef contains the information necessary to locate a resource within a cluster.

_Appears in:_
- [ResourceInventory](#resourceinventory)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `n` _string_ | Terraform resource's name. |  |  |
| `t` _string_ | Type is Terraform resource's type |  |  |
| `id` _string_ | ID is the resource identifier. This is cloud-specific. For example, ARN is an ID on AWS. |  |  |


### RetryStrategyEnum

_Underlying type:_ _string_

_Appears in:_
- [TerraformSpec](#terraformspec)

| Value | Description |
| --- | --- |
| `StaticInterval` |  |
| `ExponentialBackoff` |  |


### RunnerPodMetadata

_Appears in:_
- [RunnerPodTemplate](#runnerpodtemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `labels` _object (keys:string, values:string)_ | Labels to add to the runner pod |  | Optional: \{\} <br /> |
| `annotations` _object (keys:string, values:string)_ | Annotations to add to the runner pod |  | Optional: \{\} <br /> |


### RunnerPodSpec

_Appears in:_
- [RunnerPodTemplate](#runnerpodtemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `image` _string_ | Runner pod image to use other than default |  | Optional: \{\} <br /> |
| `envFrom` _[EnvFromSource](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#envfromsource-v1-core) array_ | List of sources to populate environment variables in the container.<br />The keys defined within a source must be a C_IDENTIFIER. All invalid keys<br />will be reported as an event when the container is starting. When a key exists in multiple<br />sources, the value associated with the last source will take precedence.<br />Values defined by an Env with a duplicate key will take precedence.<br />Cannot be updated. |  | Optional: \{\} <br /> |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#envvar-v1-core) array_ | List of environment variables to set in the container.<br />Cannot be updated. |  | Optional: \{\} <br /> |
| `nodeSelector` _object (keys:string, values:string)_ | Set the NodeSelector for the Runner Pod |  | Optional: \{\} <br /> |
| `affinity` _[Affinity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#affinity-v1-core)_ | Set the Affinity for the Runner Pod |  | Optional: \{\} <br /> |
| `tolerations` _[Toleration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#toleration-v1-core) array_ | Set the Tolerations for the Runner Pod |  | Optional: \{\} <br /> |
| `volumeMounts` _[VolumeMount](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#volumemount-v1-core) array_ | Set Volume Mounts for the Runner Pod |  | Optional: \{\} <br /> |
| `volumes` _[Volume](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#volume-v1-core) array_ | Set Volumes for the Runner Pod |  | Optional: \{\} <br /> |
| `initContainers` _[Container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#container-v1-core) array_ | Set up Init Containers for the Runner |  | Optional: \{\} <br /> |
| `hostAliases` _[HostAlias](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#hostalias-v1-core) array_ | Set host aliases for the Runner Pod |  | Optional: \{\} <br /> |
| `priorityClassName` _string_ | Set PriorityClassName for the Runner Pod container |  | Optional: \{\} <br /> |
| `securityContext` _[SecurityContext](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#securitycontext-v1-core)_ | Set SecurityContext for the Runner Pod container |  | Optional: \{\} <br /> |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#resourcerequirements-v1-core)_ | Set Resources for the Runner Pod container |  | Optional: \{\} <br /> |


### RunnerPodTemplate

_Appears in:_
- [TerraformSpec](#terraformspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[RunnerPodMetadata](#runnerpodmetadata)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[RunnerPodSpec](#runnerpodspec)_ |  |  | Optional: \{\} <br /> |


### TFStateSpec

TFStateSpec allows the user to set ForceUnlock

_Appears in:_
- [TerraformSpec](#terraformspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `forceUnlock` _[ForceUnlockEnum](#forceunlockenum)_ | ForceUnlock a Terraform state if it has become locked for any reason. Defaults to `no`.<br />This is an Enum and has the expected values of:<br />- auto<br />- yes<br />- no<br />WARNING: Only use `auto` in the cases where you are absolutely certain that<br />no other system is using this state, you could otherwise end up in a bad place<br />See https://www.terraform.io/language/state/locking#force-unlock for more<br />information on the terraform state lock and force unlock. | no | Enum: [yes no auto] <br />Optional: \{\} <br /> |
| `lockIdentifier` _string_ | LockIdentifier holds the Identifier required by Terraform to unlock the state<br />if it ever gets into a locked state.<br />You'll need to put the Lock Identifier in here while setting ForceUnlock to<br />either `yes` or `auto`.<br />Leave this empty to do nothing, set this to the value of the `Lock Info: ID: [value]`,<br />e.g. `f2ab685b-f84d-ac0b-a125-378a22877e8d`, to force unlock the state. |  | Optional: \{\} <br /> |
| `lockTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#duration-v1-meta)_ | LockTimeout is a Duration string that instructs Terraform to retry acquiring a lock for the specified period of<br />time before returning an error. The duration syntax is a number followed by a time unit letter, such as `3s` for<br />three seconds.<br />Defaults to `0s` which will behave as though `LockTimeout` was not set | 0s | Optional: \{\} <br /> |


### Terraform

Terraform is the Schema for the terraforms API

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `infra.contrib.fluxcd.io/v1alpha2` | | |
| `kind` _string_ | `Terraform` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[TerraformSpec](#terraformspec)_ |  |  |  |
| `status` _[TerraformStatus](#terraformstatus)_ |  | \{ observedGeneration:-1 \} |  |


### TerraformSpec

TerraformSpec defines the desired state of Terraform

_Appears in:_
- [Terraform](#terraform)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `approvePlan` _string_ | ApprovePlan specifies name of a plan wanted to approve.<br />If its value is "auto", the controller will automatically approve every plan. |  | Optional: \{\} <br /> |
| `destroy` _boolean_ | Destroy produces a destroy plan. Applying the plan will destroy all resources. |  | Optional: \{\} <br /> |
| `backendConfig` _[BackendConfigSpec](#backendconfigspec)_ |  |  | Optional: \{\} <br /> |
| `backendConfigsFrom` _[BackendConfigsReference](#backendconfigsreference) array_ |  |  | Optional: \{\} <br /> |
| `cloud` _[CloudSpec](#cloudspec)_ |  |  | Optional: \{\} <br /> |
| `workspace` _string_ |  | default | Optional: \{\} <br /> |
| `vars` _[Variable](#variable) array_ | List of input variables to set for the Terraform program. |  | Optional: \{\} <br /> |
| `varsFrom` _[VarsReference](#varsreference) array_ | List of references to a Secret or a ConfigMap to generate variables for<br />Terraform resources based on its data, selectively by varsKey. Values of the later<br />Secret / ConfigMap with the same keys will override those of the former. |  | Optional: \{\} <br /> |
| `values` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#json-v1-apiextensions-k8s-io)_ | Values map to the Terraform variable "values", which is an object of arbitrary values.<br />It is a convenient way to pass values to Terraform resources without having to define<br />a variable for each value. To use this feature, your Terraform file must define the variable "values". |  | Optional: \{\} <br /> |
| `tfVarsFiles` _string array_ | TfVarsFiles loads all given .tfvars files. It copycats the -var-file functionality. |  | Optional: \{\} <br /> |
| `fileMappings` _[FileMapping](#filemapping) array_ | List of all configuration files to be created in initialization. |  | Optional: \{\} <br /> |
| `interval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#duration-v1-meta)_ | The interval at which to reconcile the Terraform. |  | Required: \{\} <br /> |
| `retryInterval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#duration-v1-meta)_ | The interval at which to retry a previously failed reconciliation.<br />The default value is 15 when not specified. |  | Optional: \{\} <br /> |
| `retryStrategy` _[RetryStrategyEnum](#retrystrategyenum)_ | The strategy to use when retrying a previously failed reconciliation.<br />The default strategy is StaticInterval and the retry interval is based on the RetryInterval value.<br />The ExponentialBackoff strategy uses the formula: 2^reconciliationFailures * RetryInterval with a<br />maximum requeue duration of MaxRetryInterval. | StaticInterval | Enum: [StaticInterval ExponentialBackoff] <br />Optional: \{\} <br /> |
| `maxRetryInterval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#duration-v1-meta)_ | The maximum requeue duration after  a previously failed reconciliation.<br />Only applicable when RetryStrategy is set to ExponentialBackoff.<br />The default value is 24 hours when not specified. |  | Optional: \{\} <br /> |
| `path` _string_ | Path to the directory containing Terraform (.tf) files.<br />Defaults to 'None', which translates to the root path of the SourceRef. |  | Optional: \{\} <br /> |
| `sourceRef` _[CrossNamespaceSourceReference](#crossnamespacesourcereference)_ | SourceRef is the reference of the source where the Terraform files are stored. |  | Required: \{\} <br /> |
| `suspend` _boolean_ | Suspend is to tell the controller to suspend subsequent TF executions,<br />it does not apply to already started executions. Defaults to false. |  | Optional: \{\} <br /> |
| `force` _boolean_ | Force instructs the controller to unconditionally<br />re-plan and re-apply TF resources. Defaults to false. | false | Optional: \{\} <br /> |
| `readInputsFromSecrets` _[ReadInputsFromSecretSpec](#readinputsfromsecretspec) array_ |  |  | Optional: \{\} <br /> |
| `writeOutputsToSecret` _[WriteOutputsToSecretSpec](#writeoutputstosecretspec)_ | A list of target secrets for the outputs to be written as. |  | Optional: \{\} <br /> |
| `disableDriftDetection` _boolean_ | Disable automatic drift detection. Drift detection may be resource intensive in<br />the context of a large cluster or complex Terraform statefile. Defaults to false. | false | Optional: \{\} <br /> |
| `cliConfigSecretRef` _[SecretReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#secretreference-v1-core)_ |  |  | Optional: \{\} <br /> |
| `healthChecks` _[HealthCheck](#healthcheck) array_ | List of health checks to be performed. |  | Optional: \{\} <br /> |
| `destroyResourcesOnDeletion` _boolean_ | Create destroy plan and apply it to destroy terraform resources<br />upon deletion of this object. Defaults to false. | false | Optional: \{\} <br /> |
| `serviceAccountName` _string_ | Name of a ServiceAccount for the runner Pod to provision Terraform resources.<br />Default to tf-runner. | tf-runner | Optional: \{\} <br /> |
| `alwaysCleanupRunnerPod` _boolean_ | Clean the runner pod up after each reconciliation cycle | true | Optional: \{\} <br /> |
| `runnerTerminationGracePeriodSeconds` _integer_ | Configure the termination grace period for the runner pod. Use this parameter<br />to allow the Terraform process to gracefully shutdown. Consider increasing for<br />large, complex or slow-moving Terraform managed resources. | 30 | Optional: \{\} <br /> |
| `upgradeOnInit` _boolean_ | UpgradeOnInit configures to upgrade modules and providers on initialization of a stack | true | Optional: \{\} <br /> |
| `refreshBeforeApply` _boolean_ | RefreshBeforeApply forces refreshing of the state before the apply step. | false | Optional: \{\} <br /> |
| `runnerPodTemplate` _[RunnerPodTemplate](#runnerpodtemplate)_ |  |  | Optional: \{\} <br /> |
| `enableInventory` _boolean_ | EnableInventory enables the object to store resource entries as the inventory for external use. |  | Optional: \{\} <br /> |
| `tfstate` _[TFStateSpec](#tfstatespec)_ |  |  | Optional: \{\} <br /> |
| `targets` _string array_ | Targets specify the resource, module or collection of resources to target. |  | Optional: \{\} <br /> |
| `parallelism` _integer_ | Parallelism limits the number of concurrent operations of Terraform apply step. Zero (0) means using the default value. | 0 | Optional: \{\} <br /> |
| `storeReadablePlan` _string_ | StoreReadablePlan enables storing the plan in a readable format. | none | Enum: [none json human] <br />Optional: \{\} <br /> |
| `webhooks` _[Webhook](#webhook) array_ |  |  | Optional: \{\} <br /> |
| `dependsOn` _[NamespacedObjectReference](https://pkg.go.dev/github.com/fluxcd/pkg/apis/meta#NamespacedObjectReference) array_ |  |  | Optional: \{\} <br /> |
| `enterprise` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#json-v1-apiextensions-k8s-io)_ | Enterprise is the enterprise configuration placeholder. |  | Optional: \{\} <br /> |
| `planOnly` _boolean_ | PlanOnly specifies if the reconciliation should or should not stop at plan<br />phase. |  | Optional: \{\} <br /> |
| `breakTheGlass` _boolean_ | BreakTheGlass specifies if the reconciliation should stop<br />and allow interactive shell in case of emergency. |  | Optional: \{\} <br /> |
| `branchPlanner` _[BranchPlanner](#branchplanner)_ | BranchPlanner configuration. |  | Optional: \{\} <br /> |
| `remediation` _[Remediation](#remediation)_ | Remediation specifies what the controller should do when reconciliation<br />fails. The default is to not perform any action. |  | Optional: \{\} <br /> |


### TerraformStatus

TerraformStatus defines the observed state of Terraform

_Appears in:_
- [Terraform](#terraform)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `lastHandledReconcileAt` _string_ | LastHandledReconcileAt holds the value of the most recent<br />reconcile request value, so a change of the annotation value<br />can be detected. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the last reconciled generation. |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#condition-v1-meta) array_ |  |  | Optional: \{\} <br /> |
| `lastAppliedRevision` _string_ | The last successfully applied revision.<br />The revision format for Git sources is <branch\|tag>/<commit-sha>. |  | Optional: \{\} <br /> |
| `lastAttemptedRevision` _string_ | LastAttemptedRevision is the revision of the last reconciliation attempt. |  | Optional: \{\} <br /> |
| `lastPlannedRevision` _string_ | LastPlannedRevision is the revision used by the last planning process.<br />The result could be either no plan change or a new plan generated. |  | Optional: \{\} <br /> |
| `lastPlanAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)_ | LastPlanAt is the time when the last terraform plan was performed |  | Optional: \{\} <br /> |
| `lastDriftDetectedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)_ | LastDriftDetectedAt is the time when the last drift was detected |  | Optional: \{\} <br /> |
| `lastAppliedByDriftDetectionAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)_ | LastAppliedByDriftDetectionAt is the time when the last drift was detected and<br />terraform apply was performed as a result |  | Optional: \{\} <br /> |
| `availableOutputs` _string array_ |  |  | Optional: \{\} <br /> |
| `plan` _[PlanStatus](#planstatus)_ |  |  | Optional: \{\} <br /> |
| `inventory` _[ResourceInventory](#resourceinventory)_ | Inventory contains the list of Terraform resource object references that have been successfully applied. |  | Optional: \{\} <br /> |
| `lock` _[LockStatus](#lockstatus)_ |  |  | Optional: \{\} <br /> |
| `reconciliationFailures` _integer_ | ReconciliationFailures is the number of reconciliation<br />failures since the last success or update. |  | Optional: \{\} <br /> |


### Variable

_Appears in:_
- [TerraformSpec](#terraformspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the variable |  | Required: \{\} <br /> |
| `value` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#json-v1-apiextensions-k8s-io)_ |  |  | Optional: \{\} <br /> |
| `valueFrom` _[EnvVarSource](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#envvarsource-v1-core)_ |  |  | Optional: \{\} <br /> |


### VarsReference

VarsReference contain a reference of a Secret or a ConfigMap to generate
variables for Terraform resources based on its data, selectively by varsKey.

_Appears in:_
- [TerraformSpec](#terraformspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kind` _string_ | Kind of the values referent, valid values are ('Secret', 'ConfigMap'). |  | Enum: [Secret ConfigMap] <br />Required: \{\} <br /> |
| `name` _string_ | Name of the values referent. Should reside in the same namespace as the<br />referring resource. |  | MaxLength: 253 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `varsKeys` _string array_ | VarsKeys is the data key at which a specific value can be found. Defaults to all keys. |  | Optional: \{\} <br /> |
| `optional` _boolean_ | Optional marks this VarsReference as optional. When set, a not found error<br />for the values reference is ignored, but any VarsKey or<br />transient error will still result in a reconciliation failure. |  | Optional: \{\} <br /> |


### Webhook

_Appears in:_
- [TerraformSpec](#terraformspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `stage` _string_ |  | post-planning | Enum: [post-planning] <br />Required: \{\} <br /> |
| `enabled` _boolean_ |  | true | Optional: \{\} <br /> |
| `url` _string_ |  |  | Required: \{\} <br /> |
| `payloadType` _string_ |  | SpecAndPlan | Optional: \{\} <br /> |
| `errorMessageTemplate` _string_ |  |  | Optional: \{\} <br /> |
| `testExpression` _string_ |  |  | Required: \{\} <br /> |


### WriteOutputsToSecretSpec

WriteOutputsToSecretSpec defines where to store outputs, and which outputs to be stored.

_Appears in:_
- [TerraformSpec](#terraformspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the Secret to be written |  | Required: \{\} <br /> |
| `labels` _object (keys:string, values:string)_ | Labels to add to the outputted secret |  | Optional: \{\} <br /> |
| `annotations` _object (keys:string, values:string)_ | Annotations to add to the outputted secret |  | Optional: \{\} <br /> |
| `outputs` _string array_ | Outputs contain the selected names of outputs to be written<br />to the secret. Empty array means writing all outputs, which is default. |  | Optional: \{\} <br /> |

