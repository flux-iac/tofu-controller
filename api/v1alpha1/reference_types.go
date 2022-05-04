package v1alpha1

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CrossNamespaceSourceReference contains enough information to let you locate the
// typed Kubernetes resource object at cluster level.
type CrossNamespaceSourceReference struct {
	// API version of the referent.
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind of the referent.
	// +kubebuilder:validation:Enum=GitRepository;Bucket
	// +required
	Kind string `json:"kind"`

	// Name of the referent.
	// +required
	Name string `json:"name"`

	// Namespace of the referent, defaults to the namespace of the Kubernetes resource object that contains the reference.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

func (s *CrossNamespaceSourceReference) String() string {
	if s.Namespace != "" {
		return fmt.Sprintf("%s/%s/%s", s.Kind, s.Namespace, s.Name)
	}
	return fmt.Sprintf("%s/%s", s.Kind, s.Name)
}

// VarsReference contain a reference of a Secret or a ConfigMap to generate
// variables for Terraform resources based on its data, selectively by varsKey.
type VarsReference struct {
	// Kind of the values referent, valid values are ('Secret', 'ConfigMap').
	// +kubebuilder:validation:Enum=Secret;ConfigMap
	// +required
	Kind string `json:"kind"`

	// Name of the values referent. Should reside in the same namespace as the
	// referring resource.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Name string `json:"name"`

	// VarsKeys is the data key where the values.yaml or a specific value can be
	// found at. Defaults to all keys.
	// +optional
	VarsKeys []string `json:"varsKeys,omitempty"`

	// Optional marks this VarsReference as optional. When set, a not found error
	// for the values reference is ignored, but any VarsKey or
	// transient error will still result in a reconciliation failure.
	// +optional
	Optional bool `json:"optional,omitempty"`
}

// HealthCheck contains configuration needed to perform a health check after
// terraform is applied.
type HealthCheck struct {
	// Name of the health check.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Name string `json:"name"`

	// Type of the health check, valid values are ('tcp', 'http').
	// If tcp is specified, address is required.
	// If http is specified, url is required.
	// +kubebuilder:validation:Enum=tcp;http
	// +required
	Type string `json:"type"`

	// URL to perform http health check on. Required when http type is specified.
	// Go template can be used to reference values from the terraform output
	// (e.g. https://example.org, {{.output_url}}).
	// +optional
	URL string `json:"url,omitempty"`

	// Address to perform tcp health check on. Required when tcp type is specified.
	// Go template can be used to reference values from the terraform output
	// (e.g. 127.0.0.1:8080, {{.address}}:{{.port}}).
	// +optional
	Address string `json:"address,omitempty"`

	// The timeout period at which the connection should timeout if unable to
	// complete the request.
	// When not specified, default 20s timeout is used.
	// +kubebuilder:default="20s"
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

type RunnerPod struct {

	// +optional
	Metadata RunnerPodMetadata `json:"metadata,omitempty"`
}

type RunnerPodMetadata struct {

	// Labels to add to the runner pod
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations to add to the runner pod
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

func (in HealthCheck) GetTimeout() time.Duration {
	if in.Timeout != nil {
		return in.Timeout.Duration
	}
	// set default timeout to be 20 seconds if not specified
	d, _ := time.ParseDuration("20s")
	return d
}

const (
	HealthCheckTypeTCP     = "tcp"
	HealthCheckTypeHttpGet = "http"
)
