package v1alpha1

import (
	"fmt"
	"reflect"
)

type Backend struct {
	Local      *LocalBackend      `json:"local,omitempty"`
	Remote     *RemoteBackend     `json:"remote,omitempty"`
	AzureRM    *AzureRMBackend    `json:"azurerm,omitempty"`
	Consul     *ConsulBackend     `json:"consul,omitempty"`
	COS        *COSBackend        `json:"cos,omitempty"`
	GCS        *GCSBackend        `json:"gcs,omitempty"`
	HTTP       *HTTPBackend       `json:"http,omitempty"`
	Kubernetes *KubernetesBackend `json:"kubernetes,omitempty"`
	OSS        *OSSBackend        `json:"oss,omitempty"`
	PG         *PGBackend         `json:"pg,omitempty"`
	S3         *S3Backend         `json:"s3,omitempty"`
}

// +kubebuilder:object:generate=false
type BackendToHCL interface {
	ToHCL() (string, error)
}

func (b *Backend) ToHCL() (string, error) {
	bb, err := checkBackendFields(*b)
	if err != nil {
		return "", err
	}
	return bb.ToHCL()
}

func checkBackendFields(b Backend) (BackendToHCL, error) {
	count := 0
	var config BackendToHCL
	val := reflect.ValueOf(b)
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		if !field.IsNil() {
			count++
			if count > 1 {
				return nil, fmt.Errorf("only one field of the Backend struct can be non-nil at a time")
			}
			config = field.Interface().(BackendToHCL)
		}
	}
	if count == 0 {
		return nil, fmt.Errorf("at least one field of the Backend struct must be non-nil")
	}
	return config, nil
}

// LocalBackend Configuration variables
// The following configuration options are supported:
//
// path - (Optional) The path to the tfstate file. This defaults to "terraform.tfstate" relative to the root module by default.
// workspace_dir - (Optional) The path to non-default workspaces.
type LocalBackend struct {
	// +kubebuilder:validation:Optional
	Path string `json:"path,omitempty"`

	// +kubebuilder:validation:Optional
	WorkspaceDir string `json:"workspace_dir,omitempty"`
}

func (b *LocalBackend) ToHCL() (string, error) {
	buf := newWriter()
	buf.W("terraform {")
	buf.W("  backend %q {", "local")
	if b.Path != "" {
		buf.W("    path = %q", b.Path)
	}
	if b.WorkspaceDir != "" {
		buf.W("    workspace_dir = %q", b.WorkspaceDir)
	}
	buf.W("  }")
	buf.W("}")
	return buf.String(), nil
}

// RemoteBackend The following configuration options are supported:
//
// hostname - (Optional) The remote backend hostname to connect to. Defaults to app.terraform.io.
//
// organization - (Required) The name of the organization containing the targeted workspace(s).
//
// token - (Optional) The token used to authenticate with the remote backend. We recommend omitting the token from the configuration, and instead using terraform login or manually configuring credentials in the CLI config file.
//
// workspaces - (Required) A block specifying which remote workspace(s) to use. The workspaces block supports the following keys:
//
//	name - (Optional) The full name of one remote workspace. When configured, only the default workspace can be used. This option conflicts with prefix.
//	prefix - (Optional) A prefix used in the names of one or more remote workspaces, all of which can be used with this configuration. The full workspace names are used in Terraform Cloud, and the short names (minus the prefix) are used on the command line for Terraform CLI workspaces. If omitted, only the default workspace can be used. This option conflicts with name.
type RemoteBackend struct {
	// +kubebuilder:validation:Optional
	Hostname string `json:"hostname,omitempty"`

	// +kubebuilder:validation:Required
	Organization string `json:"organization,omitempty"`

	// +kubebuilder:validation:Optional
	Token string `json:"token,omitempty"`

	// +kubebuilder:validation:Required
	Workspaces *RemoteBackendWorkspaces `json:"workspaces,omitempty"`
}

type RemoteBackendWorkspaces struct {
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`

	// +kubebuilder:validation:Optional
	Prefix string `json:"prefix,omitempty"`
}

func (b *RemoteBackend) ToHCL() (string, error) {
	if b.Workspaces == nil {
		return "", fmt.Errorf("workspaces configuration is required for RemoteBackend")
	}

	buf := newWriter()
	buf.W("terraform {")
	buf.W("  backend %q {", "remote")
	if b.Hostname != "" {
		buf.W("    hostname = %q", b.Hostname)
	}
	if b.Organization != "" {
		buf.W("    organization = %q", b.Organization)
	}
	if b.Token != "" {
		buf.W("    token = %q", b.Token)
	}
	if b.Workspaces != nil {
		buf.W("    workspaces {")
		if b.Workspaces.Name != "" && b.Workspaces.Prefix != "" {
			return "", fmt.Errorf("only one of name or prefix can be set in Workspaces configuration")
		} else if b.Workspaces.Name != "" {
			buf.W("      name = %q", b.Workspaces.Name)
		} else if b.Workspaces.Prefix != "" {
			buf.W("      prefix = %q", b.Workspaces.Prefix)
		} else {
			return "", fmt.Errorf("either name or prefix is required in Workspaces configuration")
		}
		buf.W("    }")
	}
	buf.W("  }")
	buf.W("}")
	return buf.String(), nil
}

// AzureRMBackend The following configuration options are supported:
// storage_account_name - (Required) The Name of the Storage Account.
// container_name - (Required) The Name of the Storage Container within the Storage Account.
// key - (Required) The name of the Blob used to retrieve/store Terraform's State file inside the Storage Container.
// environment - (Optional) The Azure Environment which should be used. This can also be sourced from the ARM_ENVIRONMENT environment variable. Possible values are public, china, german, stack and usgovernment. Defaults to public.
// endpoint - (Optional) The Custom Endpoint for Azure Resource Manager. This can also be sourced from the ARM_ENDPOINT environment variable.
//
//	NOTE: An endpoint should only be configured when using Azure Stack.
//
// snapshot - (Optional) Should the Blob used to store the Terraform Statefile be snapshotted before use? Defaults to false. This value can also be sourced from the ARM_SNAPSHOT environment variable.
// =============
// When authenticating using the Managed Service Identity (MSI) - the following fields are also supported:
// resource_group_name - (Required) The Name of the Resource Group in which the Storage Account exists.
// msi_endpoint - (Optional) The path to a custom Managed Service Identity endpoint which is automatically determined if not specified. This can also be sourced from the ARM_MSI_ENDPOINT environment variable.
// subscription_id - (Optional) The Subscription ID in which the Storage Account exists. This can also be sourced from the ARM_SUBSCRIPTION_ID environment variable.
// tenant_id - (Optional) The Tenant ID in which the Subscription exists. This can also be sourced from the ARM_TENANT_ID environment variable.
// use_msi - (Optional) Should Managed Service Identity authentication be used? This can also be sourced from the ARM_USE_MSI environment variable.
// =============
// When authenticating using a Service Principal with OpenID Connect (OIDC) - the following fields are also supported:
// oidc_request_url - (Optional) The URL for the OIDC provider from which to request an ID token. This can also be sourced from the ARM_OIDC_REQUEST_URL or ACTIONS_ID_TOKEN_REQUEST_URL environment variables.
// oidc_request_token - (Optional) The bearer token for the request to the OIDC provider. This can also be sourced from the ARM_OIDC_REQUEST_TOKEN or ACTIONS_ID_TOKEN_REQUEST_TOKEN environment variables.
// oidc_token - (Optional) The ID token when authenticating using OpenID Connect (OIDC). This can also be sourced from the ARM_OIDC_TOKEN environment variable.
// oidc_token_file_path - (Optional) The path to a file containing an ID token when authenticating using OpenID Connect (OIDC). This can also be sourced from the ARM_OIDC_TOKEN_FILE_PATH environment variable.
// use_oidc - (Optional) Should OIDC authentication be used? This can also be sourced from the ARM_USE_OIDC environment variable.
// =============
// When authenticating using a SAS Token associated with the Storage Account - the following fields are also supported:
// sas_token - (Optional) The SAS Token used to access the Blob Storage Account. This can also be sourced from the ARM_SAS_TOKEN environment variable.
// =============
// When authenticating using the Storage Account's Access Key - the following fields are also supported:
// access_key - (Optional) The Access Key used to access the Blob Storage Account. This can also be sourced from the ARM_ACCESS_KEY environment variable.
// =============
// When authenticating using AzureAD Authentication - the following fields are also supported:
// use_azuread_auth - (Optional) Should AzureAD Authentication be used to access the Blob Storage Account. This can also be sourced from the ARM_USE_AZUREAD environment variable.
// Note: When using AzureAD for Authentication to Storage you also need to ensure the Storage Blob Data Owner role is assigned.
// =============
// When authenticating using a Service Principal with a Client Certificate - the following fields are also supported:
// resource_group_name - (Required) The Name of the Resource Group in which the Storage Account exists.
// client_id - (Optional) The Client ID of the Service Principal. This can also be sourced from the ARM_CLIENT_ID environment variable.
// client_certificate_password - (Optional) The password associated with the Client Certificate specified in client_certificate_path. This can also be sourced from the ARM_CLIENT_CERTIFICATE_PASSWORD environment variable.
// client_certificate_path - (Optional) The path to the PFX file used as the Client Certificate when authenticating as a Service Principal. This can also be sourced from the ARM_CLIENT_CERTIFICATE_PATH environment variable.
// subscription_id - (Optional) The Subscription ID in which the Storage Account exists. This can also be sourced from the ARM_SUBSCRIPTION_ID environment variable.
// tenant_id - (Optional) The Tenant ID in which the Subscription exists. This can also be sourced from the ARM_TENANT_ID environment variable.
// =============
// When authenticating using a Service Principal with a Client Secret - the following fields are also supported:
// resource_group_name - (Required) The Name of the Resource Group in which the Storage Account exists.
// client_id - (Optional) The Client ID of the Service Principal. This can also be sourced from the ARM_CLIENT_ID environment variable.
// client_secret - (Optional) The Client Secret of the Service Principal. This can also be sourced from the ARM_CLIENT_SECRET environment variable.
// subscription_id - (Optional) The Subscription ID in which the Storage Account exists. This can also be sourced from the ARM_SUBSCRIPTION_ID environment variable.
// tenant_id - (Optional) The Tenant ID in which the Subscription exists. This can also be sourced from the ARM_TENANT_ID environment variable.
type AzureRMBackend struct {
	StorageAccountName        string `json:"storage_account_name"`
	ContainerName             string `json:"container_name"`
	Key                       string `json:"key"`
	Environment               string `json:"environment,omitempty"`
	Endpoint                  string `json:"endpoint,omitempty"`
	Snapshot                  bool   `json:"snapshot,omitempty"`
	ResourceGroupName         string `json:"resource_group_name,omitempty"`
	MsiEndpoint               string `json:"msi_endpoint,omitempty"`
	SubscriptionID            string `json:"subscription_id,omitempty"`
	TenantID                  string `json:"tenant_id,omitempty"`
	UseMsi                    bool   `json:"use_msi,omitempty"`
	OidcRequestURL            string `json:"oidc_request_url,omitempty"`
	OidcRequestToken          string `json:"oidc_request_token,omitempty"`
	OidcToken                 string `json:"oidc_token,omitempty"`
	OidcTokenFilePath         string `json:"oidc_token_file_path,omitempty"`
	UseOidc                   bool   `json:"use_oidc,omitempty"`
	SasToken                  string `json:"sas_token,omitempty"`
	AccessKey                 string `json:"access_key,omitempty"`
	UseAzureadAuth            bool   `json:"use_azuread_auth,omitempty"`
	ClientID                  string `json:"client_id,omitempty"`
	ClientCertificatePassword string `json:"client_certificate_password,omitempty"`
	ClientCertificatePath     string `json:"client_certificate_path,omitempty"`
}

func (b *AzureRMBackend) ToHCL() (string, error) {
	buf := newWriter()
	buf.W("terraform {")
	buf.W("  backend %q {", "azurerm")
	buf.W("    storage_account_name = %q", b.StorageAccountName)
	buf.W("    container_name = %q", b.ContainerName)
	buf.W("    key = %q", b.Key)
	if b.AccessKey != "" {
		buf.W("    access_key = %q", b.AccessKey)
	}
	if b.UseMsi {
		if b.ResourceGroupName == "" || b.MsiEndpoint == "" || b.SubscriptionID == "" || b.TenantID == "" {
			return "", fmt.Errorf("all fields required for useMsi are not present: resource_group_name, msi_endpoint, subscription_id, tenant_id")
		}
		buf.W("    resource_group_name = %q", b.ResourceGroupName)
		buf.W("    msi_endpoint = %q", b.MsiEndpoint)
		buf.W("    subscription_id = %q", b.SubscriptionID)
		buf.W("    tenant_id = %q", b.TenantID)
	}
	buf.W("  }")
	buf.W("}")
	return buf.String(), nil
}

type ConsulBackend struct {
}

func (b *ConsulBackend) ToHCL() (string, error) {
	return "consul", nil
}

type COSBackend struct {
}

func (b *COSBackend) ToHCL() (string, error) {
	return "cos", nil
}

type GCSBackend struct {
}

func (b *GCSBackend) ToHCL() (string, error) {
	return "gcs", nil
}

type HTTPBackend struct {
}

func (b *HTTPBackend) ToHCL() (string, error) {
	return "http", nil
}

type KubernetesBackend struct {
}

func (b *KubernetesBackend) ToHCL() (string, error) {
	return "kubernetes", nil
}

type OSSBackend struct {
}

func (b *OSSBackend) ToHCL() (string, error) {
	return "oss", nil
}

type PGBackend struct {
}

func (b *PGBackend) ToHCL() (string, error) {
	return "pg", nil
}

type S3Backend struct {
}

func (b *S3Backend) ToHCL() (string, error) {
	return "s3", nil
}
