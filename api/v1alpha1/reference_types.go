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

type HealthCheck struct {
	// Name of the health check
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Name string `json:"name"`

	// Type of the health check, valid values are ('tcp', 'httpGet', 'httpPost').
	// +kubebuilder:validation:Enum=tcp;httpGet;httpPost
	// +required
	Type string `json:"type"`

	// Url to perform the health check on.
	// +required
	URL string `json:"url"`

	// The timeout period at which the connection should timeout if unable to
	// complete the request.
	// When not specified, default 20s timeout is used.
	// +kubebuilder:default="20s"
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`
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
	HealthCheckTypeTCP      = "tcp"
	HealthCheckTypeHttpGet  = "httpGet"
	HealthCheckTypeHttpPost = "httpPost"
)
