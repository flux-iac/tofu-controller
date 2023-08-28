<h1>Terraform API reference</h1>
<p>Packages:</p>
<ul class="simple">
<li>
<a href="#infra.contrib.fluxcd.io%2fv1alpha2">infra.contrib.fluxcd.io/v1alpha2</a>
</li>
</ul>
<h2 id="infra.contrib.fluxcd.io/v1alpha2">infra.contrib.fluxcd.io/v1alpha2</h2>
Resource Types:
<ul class="simple"></ul>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.BackendConfigSpec">BackendConfigSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformSpec">TerraformSpec</a>)
</p>
<p>BackendConfigSpec is for specifying configuration for Terraform&rsquo;s Kubernetes backend</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>disable</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Disable is to completely disable the backend configuration.</p>
</td>
</tr>
<tr>
<td>
<code>secretSuffix</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>inClusterConfig</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>customConfiguration</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>configPath</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>labels</code><br>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.BackendConfigsReference">BackendConfigsReference
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformSpec">TerraformSpec</a>)
</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>kind</code><br>
<em>
string
</em>
</td>
<td>
<p>Kind of the values referent, valid values are (&lsquo;Secret&rsquo;, &lsquo;ConfigMap&rsquo;).</p>
</td>
</tr>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
<p>Name of the configs referent. Should reside in the same namespace as the
referring resource.</p>
</td>
</tr>
<tr>
<td>
<code>keys</code><br>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Keys is the data key where a specific value can be found at. Defaults to all keys.</p>
</td>
</tr>
<tr>
<td>
<code>optional</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Optional marks this BackendConfigsReference as optional. When set, a not found error
for the values reference is ignored, but any Key or
transient error will still result in a reconciliation failure.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.BranchPlanner">BranchPlanner
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformSpec">TerraformSpec</a>)
</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enablePathScope</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>EnablePathScope specifies if the Branch Planner should or shouldn&rsquo;t check
if a Pull Request has changes under <code>.spec.path</code>. If enabled extra
resources will be created only if there are any changes in terraform files.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.CloudSpec">CloudSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformSpec">TerraformSpec</a>)
</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>organization</code><br>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>workspaces</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.CloudWorkspacesSpec">
CloudWorkspacesSpec
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>hostname</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>token</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.CloudWorkspacesSpec">CloudWorkspacesSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.CloudSpec">CloudSpec</a>)
</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>tags</code><br>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.CrossNamespaceSourceReference">CrossNamespaceSourceReference
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformSpec">TerraformSpec</a>)
</p>
<p>CrossNamespaceSourceReference contains enough information to let you locate the
typed Kubernetes resource object at cluster level.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>API version of the referent.</p>
</td>
</tr>
<tr>
<td>
<code>kind</code><br>
<em>
string
</em>
</td>
<td>
<p>Kind of the referent.</p>
</td>
</tr>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
<p>Name of the referent.</p>
</td>
</tr>
<tr>
<td>
<code>namespace</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Namespace of the referent, defaults to the namespace of the Kubernetes resource object that contains the reference.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.FileMapping">FileMapping
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformSpec">TerraformSpec</a>)
</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>secretRef</code><br>
<em>
<a href="https://godoc.org/github.com/fluxcd/pkg/apis/meta#SecretKeyReference">
github.com/fluxcd/pkg/apis/meta.SecretKeyReference
</a>
</em>
</td>
<td>
<p>Reference to a Secret that contains the file content</p>
</td>
</tr>
<tr>
<td>
<code>location</code><br>
<em>
string
</em>
</td>
<td>
<p>Location can be either user&rsquo;s home directory or the Terraform workspace</p>
</td>
</tr>
<tr>
<td>
<code>path</code><br>
<em>
string
</em>
</td>
<td>
<p>Path of the file - relative to the &ldquo;location&rdquo;</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.ForceUnlockEnum">ForceUnlockEnum
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TFStateSpec">TFStateSpec</a>)
</p>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.HealthCheck">HealthCheck
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformSpec">TerraformSpec</a>)
</p>
<p>HealthCheck contains configuration needed to perform a health check after
terraform is applied.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
<p>Name of the health check.</p>
</td>
</tr>
<tr>
<td>
<code>type</code><br>
<em>
string
</em>
</td>
<td>
<p>Type of the health check, valid values are (&lsquo;tcp&rsquo;, &lsquo;http&rsquo;).
If tcp is specified, address is required.
If http is specified, url is required.</p>
</td>
</tr>
<tr>
<td>
<code>url</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>URL to perform http health check on. Required when http type is specified.
Go template can be used to reference values from the terraform output
(e.g. <a href="https://example.org">https://example.org</a>, {{.output_url}}).</p>
</td>
</tr>
<tr>
<td>
<code>address</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Address to perform tcp health check on. Required when tcp type is specified.
Go template can be used to reference values from the terraform output
(e.g. 127.0.0.1:8080, {{.address}}:{{.port}}).</p>
</td>
</tr>
<tr>
<td>
<code>timeout</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The timeout period at which the connection should timeout if unable to
complete the request.
When not specified, default 20s timeout is used.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.LockStatus">LockStatus
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformStatus">TerraformStatus</a>)
</p>
<p>LockStatus defines the observed state of a Terraform State Lock</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>lastApplied</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>pending</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Pending holds the identifier of the Lock Holder to be used with Force Unlock</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.PlanStatus">PlanStatus
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformStatus">TerraformStatus</a>)
</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>lastApplied</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>pending</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>isDestroyPlan</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>isDriftDetectionPlan</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.ReadInputsFromSecretSpec">ReadInputsFromSecretSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformSpec">TerraformSpec</a>)
</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>as</code><br>
<em>
string
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.ResourceInventory">ResourceInventory
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformStatus">TerraformStatus</a>)
</p>
<p>ResourceInventory contains a list of Kubernetes resource object references that have been applied by a Kustomization.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>entries</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.ResourceRef">
[]ResourceRef
</a>
</em>
</td>
<td>
<p>Entries of Kubernetes resource object references.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.ResourceRef">ResourceRef
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.ResourceInventory">ResourceInventory</a>)
</p>
<p>ResourceRef contains the information necessary to locate a resource within a cluster.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>n</code><br>
<em>
string
</em>
</td>
<td>
<p>Terraform resource&rsquo;s name.</p>
</td>
</tr>
<tr>
<td>
<code>t</code><br>
<em>
string
</em>
</td>
<td>
<p>Type is Terraform resource&rsquo;s type</p>
</td>
</tr>
<tr>
<td>
<code>id</code><br>
<em>
string
</em>
</td>
<td>
<p>ID is the resource identifier. This is cloud-specific. For example, ARN is an ID on AWS.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.RunnerPodMetadata">RunnerPodMetadata
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.RunnerPodTemplate">RunnerPodTemplate</a>)
</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>labels</code><br>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Labels to add to the runner pod</p>
</td>
</tr>
<tr>
<td>
<code>annotations</code><br>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Annotations to add to the runner pod</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.RunnerPodSpec">RunnerPodSpec
</h3>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>image</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Runner pod image to use other than default</p>
</td>
</tr>
<tr>
<td>
<code>envFrom</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#envfromsource-v1-core">
[]Kubernetes core/v1.EnvFromSource
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of sources to populate environment variables in the container.
The keys defined within a source must be a C_IDENTIFIER. All invalid keys
will be reported as an event when the container is starting. When a key exists in multiple
sources, the value associated with the last source will take precedence.
Values defined by an Env with a duplicate key will take precedence.
Cannot be updated.</p>
</td>
</tr>
<tr>
<td>
<code>env</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#envvar-v1-core">
[]Kubernetes core/v1.EnvVar
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of environment variables to set in the container.
Cannot be updated.</p>
</td>
</tr>
<tr>
<td>
<code>nodeSelector</code><br>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Set the NodeSelector for the Runner Pod</p>
</td>
</tr>
<tr>
<td>
<code>affinity</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#affinity-v1-core">
Kubernetes core/v1.Affinity
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Set the Affinity for the Runner Pod</p>
</td>
</tr>
<tr>
<td>
<code>tolerations</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#toleration-v1-core">
[]Kubernetes core/v1.Toleration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Set the Tolerations for the Runner Pod</p>
</td>
</tr>
<tr>
<td>
<code>volumeMounts</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#volumemount-v1-core">
[]Kubernetes core/v1.VolumeMount
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Set Volume Mounts for the Runner Pod</p>
</td>
</tr>
<tr>
<td>
<code>volumes</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#volume-v1-core">
[]Kubernetes core/v1.Volume
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Set Volumes for the Runner Pod</p>
</td>
</tr>
<tr>
<td>
<code>initContainers</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#container-v1-core">
[]Kubernetes core/v1.Container
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Set up Init Containers for the Runner</p>
</td>
</tr>
<tr>
<td>
<code>hostAliases</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#hostalias-v1-core">
[]Kubernetes core/v1.HostAlias
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Set host aliases for the Runner Pod</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.RunnerPodTemplate">RunnerPodTemplate
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformSpec">TerraformSpec</a>)
</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.RunnerPodMetadata">
RunnerPodMetadata
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>spec</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#podspec-v1-core">
Kubernetes core/v1.PodSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<br/>
<br/>
<table>
<tr>
<td>
<code>volumes</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#volume-v1-core">
[]Kubernetes core/v1.Volume
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of volumes that can be mounted by containers belonging to the pod.
More info: <a href="https://kubernetes.io/docs/concepts/storage/volumes">https://kubernetes.io/docs/concepts/storage/volumes</a></p>
</td>
</tr>
<tr>
<td>
<code>initContainers</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#container-v1-core">
[]Kubernetes core/v1.Container
</a>
</em>
</td>
<td>
<p>List of initialization containers belonging to the pod.
Init containers are executed in order prior to containers being started. If any
init container fails, the pod is considered to have failed and is handled according
to its restartPolicy. The name for an init container or normal container must be
unique among all containers.
Init containers may not have Lifecycle actions, Readiness probes, Liveness probes, or Startup probes.
The resourceRequirements of an init container are taken into account during scheduling
by finding the highest request/limit for each resource type, and then using the max of
of that value or the sum of the normal containers. Limits are applied to init containers
in a similar fashion.
Init containers cannot currently be added or removed.
Cannot be updated.
More info: <a href="https://kubernetes.io/docs/concepts/workloads/pods/init-containers/">https://kubernetes.io/docs/concepts/workloads/pods/init-containers/</a></p>
</td>
</tr>
<tr>
<td>
<code>containers</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#container-v1-core">
[]Kubernetes core/v1.Container
</a>
</em>
</td>
<td>
<p>List of containers belonging to the pod.
Containers cannot currently be added or removed.
There must be at least one container in a Pod.
Cannot be updated.</p>
</td>
</tr>
<tr>
<td>
<code>ephemeralContainers</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#ephemeralcontainer-v1-core">
[]Kubernetes core/v1.EphemeralContainer
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of ephemeral containers run in this pod. Ephemeral containers may be run in an existing
pod to perform user-initiated actions such as debugging. This list cannot be specified when
creating a pod, and it cannot be modified by updating the pod spec. In order to add an
ephemeral container to an existing pod, use the pod&rsquo;s ephemeralcontainers subresource.</p>
</td>
</tr>
<tr>
<td>
<code>restartPolicy</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#restartpolicy-v1-core">
Kubernetes core/v1.RestartPolicy
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Restart policy for all containers within the pod.
One of Always, OnFailure, Never. In some contexts, only a subset of those values may be permitted.
Default to Always.
More info: <a href="https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#restart-policy">https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#restart-policy</a></p>
</td>
</tr>
<tr>
<td>
<code>terminationGracePeriodSeconds</code><br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>Optional duration in seconds the pod needs to terminate gracefully. May be decreased in delete request.
Value must be non-negative integer. The value zero indicates stop immediately via
the kill signal (no opportunity to shut down).
If this value is nil, the default grace period will be used instead.
The grace period is the duration in seconds after the processes running in the pod are sent
a termination signal and the time when the processes are forcibly halted with a kill signal.
Set this value longer than the expected cleanup time for your process.
Defaults to 30 seconds.</p>
</td>
</tr>
<tr>
<td>
<code>activeDeadlineSeconds</code><br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>Optional duration in seconds the pod may be active on the node relative to
StartTime before the system will actively try to mark it failed and kill associated containers.
Value must be a positive integer.</p>
</td>
</tr>
<tr>
<td>
<code>dnsPolicy</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#dnspolicy-v1-core">
Kubernetes core/v1.DNSPolicy
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Set DNS policy for the pod.
Defaults to &ldquo;ClusterFirst&rdquo;.
Valid values are &lsquo;ClusterFirstWithHostNet&rsquo;, &lsquo;ClusterFirst&rsquo;, &lsquo;Default&rsquo; or &lsquo;None&rsquo;.
DNS parameters given in DNSConfig will be merged with the policy selected with DNSPolicy.
To have DNS options set along with hostNetwork, you have to specify DNS policy
explicitly to &lsquo;ClusterFirstWithHostNet&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>nodeSelector</code><br>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>NodeSelector is a selector which must be true for the pod to fit on a node.
Selector which must match a node&rsquo;s labels for the pod to be scheduled on that node.
More info: <a href="https://kubernetes.io/docs/concepts/configuration/assign-pod-node/">https://kubernetes.io/docs/concepts/configuration/assign-pod-node/</a></p>
</td>
</tr>
<tr>
<td>
<code>serviceAccountName</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ServiceAccountName is the name of the ServiceAccount to use to run this pod.
More info: <a href="https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/">https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/</a></p>
</td>
</tr>
<tr>
<td>
<code>serviceAccount</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>DeprecatedServiceAccount is a depreciated alias for ServiceAccountName.
Deprecated: Use serviceAccountName instead.</p>
</td>
</tr>
<tr>
<td>
<code>automountServiceAccountToken</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>AutomountServiceAccountToken indicates whether a service account token should be automatically mounted.</p>
</td>
</tr>
<tr>
<td>
<code>nodeName</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>NodeName is a request to schedule this pod onto a specific node. If it is non-empty,
the scheduler simply schedules this pod onto that node, assuming that it fits resource
requirements.</p>
</td>
</tr>
<tr>
<td>
<code>hostNetwork</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Host networking requested for this pod. Use the host&rsquo;s network namespace.
If this option is set, the ports that will be used must be specified.
Default to false.</p>
</td>
</tr>
<tr>
<td>
<code>hostPID</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Use the host&rsquo;s pid namespace.
Optional: Default to false.</p>
</td>
</tr>
<tr>
<td>
<code>hostIPC</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Use the host&rsquo;s ipc namespace.
Optional: Default to false.</p>
</td>
</tr>
<tr>
<td>
<code>shareProcessNamespace</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Share a single process namespace between all of the containers in a pod.
When this is set containers will be able to view and signal processes from other containers
in the same pod, and the first process in each container will not be assigned PID 1.
HostPID and ShareProcessNamespace cannot both be set.
Optional: Default to false.</p>
</td>
</tr>
<tr>
<td>
<code>securityContext</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#podsecuritycontext-v1-core">
Kubernetes core/v1.PodSecurityContext
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SecurityContext holds pod-level security attributes and common container settings.
Optional: Defaults to empty.  See type description for default values of each field.</p>
</td>
</tr>
<tr>
<td>
<code>imagePullSecrets</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#localobjectreference-v1-core">
[]Kubernetes core/v1.LocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the images used by this PodSpec.
If specified, these secrets will be passed to individual puller implementations for them to use.
More info: <a href="https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod">https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod</a></p>
</td>
</tr>
<tr>
<td>
<code>hostname</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Specifies the hostname of the Pod
If not specified, the pod&rsquo;s hostname will be set to a system-defined value.</p>
</td>
</tr>
<tr>
<td>
<code>subdomain</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>If specified, the fully qualified Pod hostname will be &ldquo;<hostname>.<subdomain>.<pod namespace>.svc.<cluster domain>&rdquo;.
If not specified, the pod will not have a domainname at all.</p>
</td>
</tr>
<tr>
<td>
<code>affinity</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#affinity-v1-core">
Kubernetes core/v1.Affinity
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>If specified, the pod&rsquo;s scheduling constraints</p>
</td>
</tr>
<tr>
<td>
<code>schedulerName</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>If specified, the pod will be dispatched by specified scheduler.
If not specified, the pod will be dispatched by default scheduler.</p>
</td>
</tr>
<tr>
<td>
<code>tolerations</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#toleration-v1-core">
[]Kubernetes core/v1.Toleration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>If specified, the pod&rsquo;s tolerations.</p>
</td>
</tr>
<tr>
<td>
<code>hostAliases</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#hostalias-v1-core">
[]Kubernetes core/v1.HostAlias
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>HostAliases is an optional list of hosts and IPs that will be injected into the pod&rsquo;s hosts
file if specified. This is only valid for non-hostNetwork pods.</p>
</td>
</tr>
<tr>
<td>
<code>priorityClassName</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>If specified, indicates the pod&rsquo;s priority. &ldquo;system-node-critical&rdquo; and
&ldquo;system-cluster-critical&rdquo; are two special keywords which indicate the
highest priorities with the former being the highest priority. Any other
name must be defined by creating a PriorityClass object with that name.
If not specified, the pod priority will be default or zero if there is no
default.</p>
</td>
</tr>
<tr>
<td>
<code>priority</code><br>
<em>
int32
</em>
</td>
<td>
<em>(Optional)</em>
<p>The priority value. Various system components use this field to find the
priority of the pod. When Priority Admission Controller is enabled, it
prevents users from setting this field. The admission controller populates
this field from PriorityClassName.
The higher the value, the higher the priority.</p>
</td>
</tr>
<tr>
<td>
<code>dnsConfig</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#poddnsconfig-v1-core">
Kubernetes core/v1.PodDNSConfig
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Specifies the DNS parameters of a pod.
Parameters specified here will be merged to the generated DNS
configuration based on DNSPolicy.</p>
</td>
</tr>
<tr>
<td>
<code>readinessGates</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#podreadinessgate-v1-core">
[]Kubernetes core/v1.PodReadinessGate
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>If specified, all readiness gates will be evaluated for pod readiness.
A pod is ready when all its containers are ready AND
all conditions specified in the readiness gates have status equal to &ldquo;True&rdquo;
More info: <a href="https://git.k8s.io/enhancements/keps/sig-network/580-pod-readiness-gates">https://git.k8s.io/enhancements/keps/sig-network/580-pod-readiness-gates</a></p>
</td>
</tr>
<tr>
<td>
<code>runtimeClassName</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>RuntimeClassName refers to a RuntimeClass object in the node.k8s.io group, which should be used
to run this pod.  If no RuntimeClass resource matches the named class, the pod will not be run.
If unset or empty, the &ldquo;legacy&rdquo; RuntimeClass will be used, which is an implicit class with an
empty definition that uses the default runtime handler.
More info: <a href="https://git.k8s.io/enhancements/keps/sig-node/585-runtime-class">https://git.k8s.io/enhancements/keps/sig-node/585-runtime-class</a></p>
</td>
</tr>
<tr>
<td>
<code>enableServiceLinks</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>EnableServiceLinks indicates whether information about services should be injected into pod&rsquo;s
environment variables, matching the syntax of Docker links.
Optional: Defaults to true.</p>
</td>
</tr>
<tr>
<td>
<code>preemptionPolicy</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#preemptionpolicy-v1-core">
Kubernetes core/v1.PreemptionPolicy
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>PreemptionPolicy is the Policy for preempting pods with lower priority.
One of Never, PreemptLowerPriority.
Defaults to PreemptLowerPriority if unset.</p>
</td>
</tr>
<tr>
<td>
<code>overhead</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#resourcelist-v1-core">
Kubernetes core/v1.ResourceList
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Overhead represents the resource overhead associated with running a pod for a given RuntimeClass.
This field will be autopopulated at admission time by the RuntimeClass admission controller. If
the RuntimeClass admission controller is enabled, overhead must not be set in Pod create requests.
The RuntimeClass admission controller will reject Pod create requests which have the overhead already
set. If RuntimeClass is configured and selected in the PodSpec, Overhead will be set to the value
defined in the corresponding RuntimeClass, otherwise it will remain unset and treated as zero.
More info: <a href="https://git.k8s.io/enhancements/keps/sig-node/688-pod-overhead/README.md">https://git.k8s.io/enhancements/keps/sig-node/688-pod-overhead/README.md</a></p>
</td>
</tr>
<tr>
<td>
<code>topologySpreadConstraints</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#topologyspreadconstraint-v1-core">
[]Kubernetes core/v1.TopologySpreadConstraint
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>TopologySpreadConstraints describes how a group of pods ought to spread across topology
domains. Scheduler will schedule pods in a way which abides by the constraints.
All topologySpreadConstraints are ANDed.</p>
</td>
</tr>
<tr>
<td>
<code>setHostnameAsFQDN</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>If true the pod&rsquo;s hostname will be configured as the pod&rsquo;s FQDN, rather than the leaf name (the default).
In Linux containers, this means setting the FQDN in the hostname field of the kernel (the nodename field of struct utsname).
In Windows containers, this means setting the registry value of hostname for the registry key HKEY_LOCAL_MACHINE\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters to FQDN.
If a pod does not have FQDN, this has no effect.
Default to false.</p>
</td>
</tr>
<tr>
<td>
<code>os</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#podos-v1-core">
Kubernetes core/v1.PodOS
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Specifies the OS of the containers in the pod.
Some pod and container fields are restricted if this is set.</p>
<p>If the OS field is set to linux, the following fields must be unset:
-securityContext.windowsOptions</p>
<p>If the OS field is set to windows, following fields must be unset:
- spec.hostPID
- spec.hostIPC
- spec.hostUsers
- spec.securityContext.seLinuxOptions
- spec.securityContext.seccompProfile
- spec.securityContext.fsGroup
- spec.securityContext.fsGroupChangePolicy
- spec.securityContext.sysctls
- spec.shareProcessNamespace
- spec.securityContext.runAsUser
- spec.securityContext.runAsGroup
- spec.securityContext.supplementalGroups
- spec.containers[<em>].securityContext.seLinuxOptions
- spec.containers[</em>].securityContext.seccompProfile
- spec.containers[<em>].securityContext.capabilities
- spec.containers[</em>].securityContext.readOnlyRootFilesystem
- spec.containers[<em>].securityContext.privileged
- spec.containers[</em>].securityContext.allowPrivilegeEscalation
- spec.containers[<em>].securityContext.procMount
- spec.containers[</em>].securityContext.runAsUser
- spec.containers[*].securityContext.runAsGroup</p>
</td>
</tr>
<tr>
<td>
<code>hostUsers</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Use the host&rsquo;s user namespace.
Optional: Default to true.
If set to true or not present, the pod will be run in the host user namespace, useful
for when the pod needs a feature only available to the host user namespace, such as
loading a kernel module with CAP_SYS_MODULE.
When set to false, a new userns is created for the pod. Setting false is useful for
mitigating container breakout vulnerabilities even allowing users to run their
containers as root without actually having root privileges on the host.
This field is alpha-level and is only honored by servers that enable the UserNamespacesSupport feature.</p>
</td>
</tr>
<tr>
<td>
<code>schedulingGates</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#podschedulinggate-v1-core">
[]Kubernetes core/v1.PodSchedulingGate
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SchedulingGates is an opaque list of values that if specified will block scheduling the pod.
If schedulingGates is not empty, the pod will stay in the SchedulingGated state and the
scheduler will not attempt to schedule the pod.</p>
<p>SchedulingGates can only be set at pod creation time, and be removed only afterwards.</p>
<p>This is a beta feature enabled by the PodSchedulingReadiness feature gate.</p>
</td>
</tr>
<tr>
<td>
<code>resourceClaims</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#podresourceclaim-v1-core">
[]Kubernetes core/v1.PodResourceClaim
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ResourceClaims defines which ResourceClaims must be allocated
and reserved before the Pod is allowed to start. The resources
will be made available to those containers which consume them
by name.</p>
<p>This is an alpha field and requires enabling the
DynamicResourceAllocation feature gate.</p>
<p>This field is immutable.</p>
</td>
</tr>
</table>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.TFStateSpec">TFStateSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformSpec">TerraformSpec</a>)
</p>
<p>TFStateSpec allows the user to set ForceUnlock</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>forceUnlock</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.ForceUnlockEnum">
ForceUnlockEnum
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ForceUnlock a Terraform state if it has become locked for any reason. Defaults to <code>no</code>.</p>
<p>This is an Enum and has the expected values of:</p>
<ul>
<li>auto</li>
<li>yes</li>
<li>no</li>
</ul>
<p>WARNING: Only use <code>auto</code> in the cases where you are absolutely certain that
no other system is using this state, you could otherwise end up in a bad place
See <a href="https://www.terraform.io/language/state/locking#force-unlock">https://www.terraform.io/language/state/locking#force-unlock</a> for more
information on the terraform state lock and force unlock.</p>
</td>
</tr>
<tr>
<td>
<code>lockIdentifier</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LockIdentifier holds the Identifier required by Terraform to unlock the state
if it ever gets into a locked state.</p>
<p>You&rsquo;ll need to put the Lock Identifier in here while setting ForceUnlock to
either <code>yes</code> or <code>auto</code>.</p>
<p>Leave this empty to do nothing, set this to the value of the <code>Lock Info: ID: [value]</code>,
e.g. <code>f2ab685b-f84d-ac0b-a125-378a22877e8d</code>, to force unlock the state.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.Terraform">Terraform
</h3>
<p>Terraform is the Schema for the terraforms API</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformSpec">
TerraformSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>approvePlan</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ApprovePlan specifies name of a plan wanted to approve.
If its value is &ldquo;auto&rdquo;, the controller will automatically approve every plan.</p>
</td>
</tr>
<tr>
<td>
<code>destroy</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Destroy produces a destroy plan. Applying the plan will destroy all resources.</p>
</td>
</tr>
<tr>
<td>
<code>backendConfig</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.BackendConfigSpec">
BackendConfigSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>backendConfigsFrom</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.BackendConfigsReference">
[]BackendConfigsReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>cloud</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.CloudSpec">
CloudSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>workspace</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>vars</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.Variable">
[]Variable
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of input variables to set for the Terraform program.</p>
</td>
</tr>
<tr>
<td>
<code>varsFrom</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.VarsReference">
[]VarsReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of references to a Secret or a ConfigMap to generate variables for
Terraform resources based on its data, selectively by varsKey. Values of the later
Secret / ConfigMap with the same keys will override those of the former.</p>
</td>
</tr>
<tr>
<td>
<code>values</code><br>
<em>
<a href="https://pkg.go.dev/k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1?tab=doc#JSON">
Kubernetes pkg/apis/apiextensions/v1.JSON
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Values map to the Terraform variable &ldquo;values&rdquo;, which is an object of arbitrary values.
It is a convenient way to pass values to Terraform resources without having to define
a variable for each value. To use this feature, your Terraform file must define the variable &ldquo;values&rdquo;.</p>
</td>
</tr>
<tr>
<td>
<code>fileMappings</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.FileMapping">
[]FileMapping
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of all configuration files to be created in initialization.</p>
</td>
</tr>
<tr>
<td>
<code>interval</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<p>The interval at which to reconcile the Terraform.</p>
</td>
</tr>
<tr>
<td>
<code>retryInterval</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The interval at which to retry a previously failed reconciliation.
The default value is 15 when not specified.</p>
</td>
</tr>
<tr>
<td>
<code>path</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Path to the directory containing Terraform (.tf) files.
Defaults to &lsquo;None&rsquo;, which translates to the root path of the SourceRef.</p>
</td>
</tr>
<tr>
<td>
<code>sourceRef</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.CrossNamespaceSourceReference">
CrossNamespaceSourceReference
</a>
</em>
</td>
<td>
<p>SourceRef is the reference of the source where the Terraform files are stored.</p>
</td>
</tr>
<tr>
<td>
<code>suspend</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Suspend is to tell the controller to suspend subsequent TF executions,
it does not apply to already started executions. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>force</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Force instructs the controller to unconditionally
re-plan and re-apply TF resources. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>readInputsFromSecrets</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.ReadInputsFromSecretSpec">
[]ReadInputsFromSecretSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>writeOutputsToSecret</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.WriteOutputsToSecretSpec">
WriteOutputsToSecretSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>A list of target secrets for the outputs to be written as.</p>
</td>
</tr>
<tr>
<td>
<code>disableDriftDetection</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Disable automatic drift detection. Drift detection may be resource intensive in
the context of a large cluster or complex Terraform statefile. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>cliConfigSecretRef</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#secretreference-v1-core">
Kubernetes core/v1.SecretReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>healthChecks</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.HealthCheck">
[]HealthCheck
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of health checks to be performed.</p>
</td>
</tr>
<tr>
<td>
<code>destroyResourcesOnDeletion</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Create destroy plan and apply it to destroy terraform resources
upon deletion of this object. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>serviceAccountName</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Name of a ServiceAccount for the runner Pod to provision Terraform resources.
Default to tf-runner.</p>
</td>
</tr>
<tr>
<td>
<code>alwaysCleanupRunnerPod</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Clean the runner pod up after each reconciliation cycle</p>
</td>
</tr>
<tr>
<td>
<code>runnerTerminationGracePeriodSeconds</code><br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>Configure the termination grace period for the runner pod. Use this parameter
to allow the Terraform process to gracefully shutdown. Consider increasing for
large, complex or slow-moving Terraform managed resources.</p>
</td>
</tr>
<tr>
<td>
<code>refreshBeforeApply</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>RefreshBeforeApply forces refreshing of the state before the apply step.</p>
</td>
</tr>
<tr>
<td>
<code>runnerPodTemplate</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.RunnerPodTemplate">
RunnerPodTemplate
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>enableInventory</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>EnableInventory enables the object to store resource entries as the inventory for external use.</p>
</td>
</tr>
<tr>
<td>
<code>tfstate</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TFStateSpec">
TFStateSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>targets</code><br>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Targets specify the resource, module or collection of resources to target.</p>
</td>
</tr>
<tr>
<td>
<code>parallelism</code><br>
<em>
int32
</em>
</td>
<td>
<em>(Optional)</em>
<p>Parallelism limits the number of concurrent operations of Terraform apply step. Zero (0) means using the default value.</p>
</td>
</tr>
<tr>
<td>
<code>storeReadablePlan</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>StoreReadablePlan enables storing the plan in a readable format.</p>
</td>
</tr>
<tr>
<td>
<code>webhooks</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.Webhook">
[]Webhook
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>dependsOn</code><br>
<em>
<a href="https://godoc.org/github.com/fluxcd/pkg/apis/meta#NamespacedObjectReference">
[]github.com/fluxcd/pkg/apis/meta.NamespacedObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>enterprise</code><br>
<em>
<a href="https://pkg.go.dev/k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1?tab=doc#JSON">
Kubernetes pkg/apis/apiextensions/v1.JSON
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Enterprise is the enterprise configuration placeholder.</p>
</td>
</tr>
<tr>
<td>
<code>planOnly</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>PlanOnly specifies if the reconciliation should or should not stop at plan
phase.</p>
</td>
</tr>
<tr>
<td>
<code>breakTheGlass</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>BreakTheGlass specifies if the reconciliation should stop
and allow interactive shell in case of emergency.</p>
</td>
</tr>
<tr>
<td>
<code>branchPlanner</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.BranchPlanner">
BranchPlanner
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>BranchPlanner configuration.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformStatus">
TerraformStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.TerraformSpec">TerraformSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.Terraform">Terraform</a>)
</p>
<p>TerraformSpec defines the desired state of Terraform</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>approvePlan</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ApprovePlan specifies name of a plan wanted to approve.
If its value is &ldquo;auto&rdquo;, the controller will automatically approve every plan.</p>
</td>
</tr>
<tr>
<td>
<code>destroy</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Destroy produces a destroy plan. Applying the plan will destroy all resources.</p>
</td>
</tr>
<tr>
<td>
<code>backendConfig</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.BackendConfigSpec">
BackendConfigSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>backendConfigsFrom</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.BackendConfigsReference">
[]BackendConfigsReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>cloud</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.CloudSpec">
CloudSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>workspace</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>vars</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.Variable">
[]Variable
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of input variables to set for the Terraform program.</p>
</td>
</tr>
<tr>
<td>
<code>varsFrom</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.VarsReference">
[]VarsReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of references to a Secret or a ConfigMap to generate variables for
Terraform resources based on its data, selectively by varsKey. Values of the later
Secret / ConfigMap with the same keys will override those of the former.</p>
</td>
</tr>
<tr>
<td>
<code>values</code><br>
<em>
<a href="https://pkg.go.dev/k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1?tab=doc#JSON">
Kubernetes pkg/apis/apiextensions/v1.JSON
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Values map to the Terraform variable &ldquo;values&rdquo;, which is an object of arbitrary values.
It is a convenient way to pass values to Terraform resources without having to define
a variable for each value. To use this feature, your Terraform file must define the variable &ldquo;values&rdquo;.</p>
</td>
</tr>
<tr>
<td>
<code>fileMappings</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.FileMapping">
[]FileMapping
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of all configuration files to be created in initialization.</p>
</td>
</tr>
<tr>
<td>
<code>interval</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<p>The interval at which to reconcile the Terraform.</p>
</td>
</tr>
<tr>
<td>
<code>retryInterval</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The interval at which to retry a previously failed reconciliation.
The default value is 15 when not specified.</p>
</td>
</tr>
<tr>
<td>
<code>path</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Path to the directory containing Terraform (.tf) files.
Defaults to &lsquo;None&rsquo;, which translates to the root path of the SourceRef.</p>
</td>
</tr>
<tr>
<td>
<code>sourceRef</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.CrossNamespaceSourceReference">
CrossNamespaceSourceReference
</a>
</em>
</td>
<td>
<p>SourceRef is the reference of the source where the Terraform files are stored.</p>
</td>
</tr>
<tr>
<td>
<code>suspend</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Suspend is to tell the controller to suspend subsequent TF executions,
it does not apply to already started executions. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>force</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Force instructs the controller to unconditionally
re-plan and re-apply TF resources. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>readInputsFromSecrets</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.ReadInputsFromSecretSpec">
[]ReadInputsFromSecretSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>writeOutputsToSecret</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.WriteOutputsToSecretSpec">
WriteOutputsToSecretSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>A list of target secrets for the outputs to be written as.</p>
</td>
</tr>
<tr>
<td>
<code>disableDriftDetection</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Disable automatic drift detection. Drift detection may be resource intensive in
the context of a large cluster or complex Terraform statefile. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>cliConfigSecretRef</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#secretreference-v1-core">
Kubernetes core/v1.SecretReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>healthChecks</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.HealthCheck">
[]HealthCheck
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of health checks to be performed.</p>
</td>
</tr>
<tr>
<td>
<code>destroyResourcesOnDeletion</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Create destroy plan and apply it to destroy terraform resources
upon deletion of this object. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>serviceAccountName</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Name of a ServiceAccount for the runner Pod to provision Terraform resources.
Default to tf-runner.</p>
</td>
</tr>
<tr>
<td>
<code>alwaysCleanupRunnerPod</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Clean the runner pod up after each reconciliation cycle</p>
</td>
</tr>
<tr>
<td>
<code>runnerTerminationGracePeriodSeconds</code><br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>Configure the termination grace period for the runner pod. Use this parameter
to allow the Terraform process to gracefully shutdown. Consider increasing for
large, complex or slow-moving Terraform managed resources.</p>
</td>
</tr>
<tr>
<td>
<code>refreshBeforeApply</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>RefreshBeforeApply forces refreshing of the state before the apply step.</p>
</td>
</tr>
<tr>
<td>
<code>runnerPodTemplate</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.RunnerPodTemplate">
RunnerPodTemplate
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>enableInventory</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>EnableInventory enables the object to store resource entries as the inventory for external use.</p>
</td>
</tr>
<tr>
<td>
<code>tfstate</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TFStateSpec">
TFStateSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>targets</code><br>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Targets specify the resource, module or collection of resources to target.</p>
</td>
</tr>
<tr>
<td>
<code>parallelism</code><br>
<em>
int32
</em>
</td>
<td>
<em>(Optional)</em>
<p>Parallelism limits the number of concurrent operations of Terraform apply step. Zero (0) means using the default value.</p>
</td>
</tr>
<tr>
<td>
<code>storeReadablePlan</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>StoreReadablePlan enables storing the plan in a readable format.</p>
</td>
</tr>
<tr>
<td>
<code>webhooks</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.Webhook">
[]Webhook
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>dependsOn</code><br>
<em>
<a href="https://godoc.org/github.com/fluxcd/pkg/apis/meta#NamespacedObjectReference">
[]github.com/fluxcd/pkg/apis/meta.NamespacedObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>enterprise</code><br>
<em>
<a href="https://pkg.go.dev/k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1?tab=doc#JSON">
Kubernetes pkg/apis/apiextensions/v1.JSON
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Enterprise is the enterprise configuration placeholder.</p>
</td>
</tr>
<tr>
<td>
<code>planOnly</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>PlanOnly specifies if the reconciliation should or should not stop at plan
phase.</p>
</td>
</tr>
<tr>
<td>
<code>breakTheGlass</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>BreakTheGlass specifies if the reconciliation should stop
and allow interactive shell in case of emergency.</p>
</td>
</tr>
<tr>
<td>
<code>branchPlanner</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.BranchPlanner">
BranchPlanner
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>BranchPlanner configuration.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.TerraformStatus">TerraformStatus
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.Terraform">Terraform</a>)
</p>
<p>TerraformStatus defines the observed state of Terraform</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>ReconcileRequestStatus</code><br>
<em>
<a href="https://godoc.org/github.com/fluxcd/pkg/apis/meta#ReconcileRequestStatus">
github.com/fluxcd/pkg/apis/meta.ReconcileRequestStatus
</a>
</em>
</td>
<td>
<p>
(Members of <code>ReconcileRequestStatus</code> are embedded into this type.)
</p>
</td>
</tr>
<tr>
<td>
<code>observedGeneration</code><br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>ObservedGeneration is the last reconciled generation.</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#condition-v1-meta">
[]Kubernetes meta/v1.Condition
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>lastAppliedRevision</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The last successfully applied revision.
The revision format for Git sources is <branch|tag>/<commit-sha>.</p>
</td>
</tr>
<tr>
<td>
<code>lastAttemptedRevision</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastAttemptedRevision is the revision of the last reconciliation attempt.</p>
</td>
</tr>
<tr>
<td>
<code>lastPlannedRevision</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastPlannedRevision is the revision used by the last planning process.
The result could be either no plan change or a new plan generated.</p>
</td>
</tr>
<tr>
<td>
<code>lastPlanAt</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastPlanAt is the time when the last terraform plan was performed</p>
</td>
</tr>
<tr>
<td>
<code>lastDriftDetectedAt</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastDriftDetectedAt is the time when the last drift was detected</p>
</td>
</tr>
<tr>
<td>
<code>lastAppliedByDriftDetectionAt</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastAppliedByDriftDetectionAt is the time when the last drift was detected and
terraform apply was performed as a result</p>
</td>
</tr>
<tr>
<td>
<code>availableOutputs</code><br>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>plan</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.PlanStatus">
PlanStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>inventory</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.ResourceInventory">
ResourceInventory
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Inventory contains the list of Terraform resource object references that have been successfully applied.</p>
</td>
</tr>
<tr>
<td>
<code>lock</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.LockStatus">
LockStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.Variable">Variable
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformSpec">TerraformSpec</a>)
</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
<p>Name is the name of the variable</p>
</td>
</tr>
<tr>
<td>
<code>value</code><br>
<em>
<a href="https://pkg.go.dev/k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1?tab=doc#JSON">
Kubernetes pkg/apis/apiextensions/v1.JSON
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>valueFrom</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#envvarsource-v1-core">
Kubernetes core/v1.EnvVarSource
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.VarsReference">VarsReference
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformSpec">TerraformSpec</a>)
</p>
<p>VarsReference contain a reference of a Secret or a ConfigMap to generate
variables for Terraform resources based on its data, selectively by varsKey.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>kind</code><br>
<em>
string
</em>
</td>
<td>
<p>Kind of the values referent, valid values are (&lsquo;Secret&rsquo;, &lsquo;ConfigMap&rsquo;).</p>
</td>
</tr>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
<p>Name of the values referent. Should reside in the same namespace as the
referring resource.</p>
</td>
</tr>
<tr>
<td>
<code>varsKeys</code><br>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>VarsKeys is the data key at which a specific value can be found. Defaults to all keys.</p>
</td>
</tr>
<tr>
<td>
<code>optional</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Optional marks this VarsReference as optional. When set, a not found error
for the values reference is ignored, but any VarsKey or
transient error will still result in a reconciliation failure.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.Webhook">Webhook
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformSpec">TerraformSpec</a>)
</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>stage</code><br>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>enabled</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>url</code><br>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>payloadType</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>errorMessageTemplate</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>testExpression</code><br>
<em>
string
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha2.WriteOutputsToSecretSpec">WriteOutputsToSecretSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha2.TerraformSpec">TerraformSpec</a>)
</p>
<p>WriteOutputsToSecretSpec defines where to store outputs, and which outputs to be stored.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
<p>Name is the name of the Secret to be written</p>
</td>
</tr>
<tr>
<td>
<code>labels</code><br>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Labels to add to the outputted secret</p>
</td>
</tr>
<tr>
<td>
<code>annotations</code><br>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Annotations to add to the outputted secret</p>
</td>
</tr>
<tr>
<td>
<code>outputs</code><br>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Outputs contain the selected names of outputs to be written
to the secret. Empty array means writing all outputs, which is default.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<div class="admonition note">
<p class="last">This page was automatically generated with <code>gen-crd-api-reference-docs</code></p>
</div>
