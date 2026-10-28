package v1alpha2

import (
	"fmt"

	"github.com/fluxcd/pkg/apis/meta"
)

// LocatorType selects which payload of an ArtifactLocator is populated.
//
// The set is deliberately about *reachability* rather than about which product
// stores the bytes: a consumer behaves differently for an object it can Get,
// an object that exists in a cluster it cannot address, a payload outside
// Kubernetes entirely, and something that was never persisted. Adding a new
// storage target means adding a type and a payload here rather than
// overloading an existing one.
// +kubebuilder:validation:Enum=Kubernetes;ExternalCluster;External;Ephemeral;None
type LocatorType string

const (
	// LocatorKubernetes is an object in the cluster the controller is running
	// in. It can be resolved with a single client.Get.
	LocatorKubernetes LocatorType = "Kubernetes"

	// LocatorExternalCluster is a Kubernetes object that exists, but in a
	// cluster reached through a kubeconfig rather than the controller's own
	// client. It is deliberately not Kubernetes so that consumers do not
	// attempt a Get that would report "not found" for an artifact that is
	// present elsewhere.
	LocatorExternalCluster LocatorType = "ExternalCluster"

	// LocatorExternal is a payload held outside Kubernetes — an object store,
	// an HTTP backend, or Terraform Cloud.
	LocatorExternal LocatorType = "External"

	// LocatorEphemeral is a path on the runner pod's filesystem. It does not
	// survive the pod and cannot be read by anything else.
	LocatorEphemeral LocatorType = "Ephemeral"

	// LocatorNone means nothing is persisted: the backend is disabled, the
	// plan reported no changes, or outputs are not configured.
	LocatorNone LocatorType = "None"
)

// ArtifactLocator identifies where an artifact physically lives. Exactly one
// payload is set, selected by Type; None carries no payload.
//
// This follows the discriminated-union shape Flux uses for
// meta.KubeConfigReference, including CEL validation that keeps Type and the
// payload in agreement, so an invalid combination is rejected by the API
// server rather than discovered by a consumer.
// +kubebuilder:validation:XValidation:rule="self.type != 'Kubernetes' || has(self.kubernetes)",message="locator type Kubernetes requires the kubernetes payload"
// +kubebuilder:validation:XValidation:rule="self.type != 'ExternalCluster' || has(self.externalCluster)",message="locator type ExternalCluster requires the externalCluster payload"
// +kubebuilder:validation:XValidation:rule="self.type != 'External' || has(self.external)",message="locator type External requires the external payload"
// +kubebuilder:validation:XValidation:rule="self.type != 'Ephemeral' || has(self.ephemeral)",message="locator type Ephemeral requires the ephemeral payload"
// +kubebuilder:validation:XValidation:rule="self.type != 'None' || (!has(self.kubernetes) && !has(self.externalCluster) && !has(self.external) && !has(self.ephemeral))",message="locator type None must not carry a payload"
// +kubebuilder:validation:XValidation:rule="[has(self.kubernetes), has(self.externalCluster), has(self.external), has(self.ephemeral)].filter(p, p).size() <= 1",message="at most one locator payload may be set"
type ArtifactLocator struct {
	// Type selects the populated payload below.
	// +required
	Type LocatorType `json:"type"`

	// Kubernetes is set when Type is Kubernetes.
	// +optional
	Kubernetes *KubernetesLocator `json:"kubernetes,omitempty"`

	// ExternalCluster is set when Type is ExternalCluster.
	// +optional
	ExternalCluster *ExternalClusterLocator `json:"externalCluster,omitempty"`

	// External is set when Type is External.
	// +optional
	External *ExternalLocator `json:"external,omitempty"`

	// Ephemeral is set when Type is Ephemeral.
	// +optional
	Ephemeral *EphemeralLocator `json:"ephemeral,omitempty"`
}

// KubernetesLocator points at one object in this cluster, down to the data key
// holding the artifact. It embeds the Flux reference type rather than
// redeclaring apiVersion/kind/name/namespace.
type KubernetesLocator struct {
	meta.NamespacedObjectKindReference `json:",inline"`

	// Key is the entry within the object's data that holds the artifact,
	// e.g. "tfplan". Empty when the artifact spans every key, as it does for
	// the outputs Secret, which stores one key per Terraform output.
	// +optional
	Key string `json:"key,omitempty"`
}

// ExternalClusterLocator points at a Kubernetes object in a cluster the
// controller does not have a client for. The reference is reported so the
// object can be found with the right credentials, and the kubeconfig the
// runner used is reported so a reader knows which cluster that is.
type ExternalClusterLocator struct {
	meta.NamespacedObjectKindReference `json:",inline"`

	// Key is the entry within the object's data that holds the artifact.
	// +optional
	Key string `json:"key,omitempty"`

	// KubeConfigPath is the path, inside the runner pod, of the kubeconfig
	// that was handed to the Terraform backend.
	// +optional
	KubeConfigPath string `json:"kubeConfigPath,omitempty"`

	// InClusterConfig reports whether the backend was told to use the pod's
	// own service account. It is false for this locator type by construction.
	// +optional
	InClusterConfig bool `json:"inClusterConfig,omitempty"`
}

// ExternalLocator describes an artifact held outside Kubernetes.
type ExternalLocator struct {
	// Backend is the Terraform backend that holds the artifact, e.g. "s3",
	// "gcs", "azurerm", "http", "consul", or "cloud".
	// +kubebuilder:validation:MinLength=1
	// +required
	Backend string `json:"backend"`

	// Location is a human-readable hint such as "s3://bucket/key" or
	// "cloud://org/workspace".
	//
	// It is best-effort and advisory only: for a custom backend it is parsed
	// out of user-supplied HCL whose values may be variables the controller
	// cannot resolve, so it is empty whenever no reliable hint is available.
	// Consumers must not parse it or depend on its format.
	// +optional
	Location string `json:"location,omitempty"`
}

// EphemeralLocator describes an artifact that only exists on the runner pod.
type EphemeralLocator struct {
	// Path is the location on the runner's filesystem, when known.
	// +optional
	Path string `json:"path,omitempty"`
}

// String renders a locator for logs and events. It never panics on a partially
// populated locator, so it is safe to call on status read back from the API.
func (l ArtifactLocator) String() string {
	switch l.Type {
	case LocatorKubernetes:
		if l.Kubernetes == nil {
			return string(LocatorKubernetes)
		}
		return l.Kubernetes.describe()
	case LocatorExternalCluster:
		if l.ExternalCluster == nil {
			return string(LocatorExternalCluster)
		}
		ref := KubernetesLocator{
			NamespacedObjectKindReference: l.ExternalCluster.NamespacedObjectKindReference,
			Key:                           l.ExternalCluster.Key,
		}
		if l.ExternalCluster.KubeConfigPath != "" {
			return fmt.Sprintf("%s (kubeconfig %s)", ref.describe(), l.ExternalCluster.KubeConfigPath)
		}
		return fmt.Sprintf("%s (external cluster)", ref.describe())
	case LocatorExternal:
		if l.External == nil {
			return string(LocatorExternal)
		}
		if l.External.Location != "" {
			return fmt.Sprintf("%s %s", l.External.Backend, l.External.Location)
		}
		return l.External.Backend
	case LocatorEphemeral:
		if l.Ephemeral == nil || l.Ephemeral.Path == "" {
			return "runner filesystem"
		}
		return fmt.Sprintf("runner filesystem %s", l.Ephemeral.Path)
	default:
		return string(LocatorNone)
	}
}

func (k KubernetesLocator) describe() string {
	out := k.Kind
	if k.Namespace != "" {
		out = fmt.Sprintf("%s %s/%s", k.Kind, k.Namespace, k.Name)
	} else {
		out = fmt.Sprintf("%s %s", k.Kind, k.Name)
	}
	if k.Key != "" {
		out = fmt.Sprintf("%s[%s]", out, k.Key)
	}
	return out
}

// KubernetesRef returns the in-cluster object reference and true when the
// locator addresses something this controller's client can read. Consumers in
// this repo should route every Get through this helper rather than reaching
// into the payload, so a new locator type cannot silently be treated as local.
func (l ArtifactLocator) KubernetesRef() (meta.NamespacedObjectKindReference, bool) {
	if l.Type != LocatorKubernetes || l.Kubernetes == nil {
		return meta.NamespacedObjectKindReference{}, false
	}
	return l.Kubernetes.NamespacedObjectKindReference, true
}

// NoneLocator builds the locator used when nothing is persisted.
func NoneLocator() ArtifactLocator {
	return ArtifactLocator{Type: LocatorNone}
}

// SecretReference spells out a core/v1 Secret reference. Kept in one place so
// the apiVersion and kind are not repeated at every construction site.
func SecretReference(namespace, name string) meta.NamespacedObjectKindReference {
	return meta.NamespacedObjectKindReference{
		APIVersion: "v1",
		Kind:       "Secret",
		Namespace:  namespace,
		Name:       name,
	}
}

// SecretLocator builds a Kubernetes locator for a Secret in this cluster.
func SecretLocator(namespace, name, key string) ArtifactLocator {
	return ArtifactLocator{
		Type: LocatorKubernetes,
		Kubernetes: &KubernetesLocator{
			NamespacedObjectKindReference: SecretReference(namespace, name),
			Key:                           key,
		},
	}
}
