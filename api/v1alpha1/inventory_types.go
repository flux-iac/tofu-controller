package v1alpha1

// ResourceInventory contains a list of Kubernetes resource object references that have been applied by a Kustomization.
type ResourceInventory struct {
	// Entries of Kubernetes resource object references.
	Entries []ResourceRef `json:"entries"`
}

// ResourceRef contains the information necessary to locate a resource within a cluster.
type ResourceRef struct {
	// Terraform resource's name.
	Name string `json:"n"`

	// Type is Terraform resource's type
	Type string `json:"t"`

	// ID is the resource identifier. This is cloud-specific. For example, ARN is an ID on AWS.
	Identifier string `json:"id"`
}
