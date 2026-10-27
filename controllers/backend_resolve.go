package controllers

import (
	"fmt"
	"regexp"
	"strings"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
)

// tfStateSecretKey is the data key the Terraform Kubernetes backend stores
// state under.
const tfStateSecretKey = "tfstate"

// backendKind is the resolved interpretation of spec.backendConfig, spec.cloud
// and DISABLE_TF_K8S_BACKEND.
type backendKind string

const (
	backendKubernetes backendKind = "kubernetes"
	backendCustom     backendKind = "custom"
	backendCloud      backendKind = "cloud"
	backendLocal      backendKind = "local"
	backendDisabled   backendKind = "disabled"
)

// resolvedBackend is the single interpretation of a Terraform's backend
// configuration.
//
// Both the HCL handed to the runner and the locator published in
// status.artifactRefs.state are derived from this one value. They used to be
// produced by two independent switches with different predicates in different
// orders, which is how two bugs arose: a state reference naming a Secret the
// backend never writes (an invented secret_suffix default), and a local
// reference published for a backend pointed at another cluster. Deriving both
// from one resolution makes that class of drift unrepresentable.
//
// To add a backend: add a kind, a case in resolveBackend, and the matching
// arms of HCL and StateLocator. The compiler then points at everything left to
// do.
type resolvedBackend struct {
	Kind backendKind

	// Kubernetes backend fields, as they will be rendered.
	SecretSuffix    string
	Namespace       string
	InClusterConfig bool
	ConfigPath      string
	Labels          map[string]string

	// renderConfigPath preserves an existing asymmetry: the explicit
	// spec.backendConfig path emits config_path even when empty, while the
	// defaulted path omits the line entirely. Kept byte-identical here so
	// this refactor changes no generated HCL; the two can be unified
	// separately, with its own test.
	renderConfigPath bool

	// CustomHCL is the user's verbatim backend block.
	CustomHCL string

	// Cloud is spec.cloud when the Terraform Cloud/Enterprise integration is
	// in use.
	Cloud *infrav1.CloudSpec
}

// resolveBackend interprets the backend configuration exactly once.
//
// disableK8sBackend mirrors DISABLE_TF_K8S_BACKEND=1 and is passed in rather
// than read from the environment so this stays a pure function.
func resolveBackend(terraform *infrav1.Terraform, disableK8sBackend bool) resolvedBackend {
	backendConfig := terraform.Spec.BackendConfig

	// spec.cloud and backendConfig.disable override whatever else is
	// configured. setupTerraform applies this override after rendering, so it
	// is evaluated first here to reach the same outcome.
	switch {
	case terraform.Spec.Cloud != nil:
		return resolvedBackend{Kind: backendCloud, Cloud: terraform.Spec.Cloud}
	case backendConfig != nil && backendConfig.Disable:
		return resolvedBackend{Kind: backendDisabled}
	}

	switch {
	case backendConfig != nil && backendConfig.CustomConfiguration != "":
		return resolvedBackend{
			Kind:      backendCustom,
			CustomHCL: backendConfig.CustomConfiguration,
		}

	case backendConfig != nil:
		// secret_suffix is rendered verbatim, including when empty. Status
		// must not substitute a default the backend will not use.
		return resolvedBackend{
			Kind:             backendKubernetes,
			SecretSuffix:     backendConfig.SecretSuffix,
			Namespace:        terraform.Namespace,
			InClusterConfig:  backendConfig.InClusterConfig,
			ConfigPath:       backendConfig.ConfigPath,
			Labels:           backendConfig.Labels,
			renderConfigPath: true,
		}

	case disableK8sBackend:
		return resolvedBackend{Kind: backendLocal}

	default:
		// No backendConfig at all: the suffix defaults to the object name and
		// the backend always uses the pod's own service account.
		return resolvedBackend{
			Kind:            backendKubernetes,
			SecretSuffix:    terraform.Name,
			Namespace:       terraform.Namespace,
			InClusterConfig: true,
		}
	}
}

// completelyDisabled reports whether the runner should be given no backend
// configuration of its own.
func (b resolvedBackend) completelyDisabled() bool {
	return b.Kind == backendCloud || b.Kind == backendDisabled
}

// HCL returns the backend configuration to write into the working directory,
// and whether anything should be written at all.
func (b resolvedBackend) HCL() (string, bool) {
	switch b.Kind {
	case backendCustom:
		return fmt.Sprintf(`
terraform {
  %v
}
`, b.CustomHCL), true

	case backendKubernetes:
		if b.renderConfigPath {
			return fmt.Sprintf(`
terraform {
  backend "kubernetes" {
    secret_suffix     = "%s"
    in_cluster_config = %v
    config_path       = "%s"
    namespace         = "%s"
    labels            = {
      %s
    }
  }
}
`,
				b.SecretSuffix,
				b.InClusterConfig,
				b.ConfigPath,
				b.Namespace,
				getLabelsAsHCL(b.Labels, 6)), true
		}

		return fmt.Sprintf(`
terraform {
  backend "kubernetes" {
    secret_suffix     = "%s"
    in_cluster_config = true
    namespace         = "%s"
    labels            = {
      %s
    }
  }
}
`,
			b.SecretSuffix,
			b.Namespace,
			getLabelsAsHCL(b.Labels, 6)), true

	case backendLocal:
		return `
terraform {
  backend "local" { }
}`, true

	case backendCloud:
		if b.Cloud == nil {
			return "", false
		}
		return b.Cloud.ToHCL(), true

	default:
		return "", false
	}
}

// StateLocator describes where this backend keeps Terraform state.
func (b resolvedBackend) StateLocator(terraform *infrav1.Terraform) infrav1.ArtifactLocator {
	switch b.Kind {
	case backendKubernetes:
		name := fmt.Sprintf("tfstate-%s-%s", terraform.WorkspaceName(), b.SecretSuffix)

		// An explicit kubeconfig means the backend talks to whatever cluster
		// that file addresses, which the controller's own client may not be
		// able to reach. Reporting a plain Kubernetes locator here would name
		// a Secret that does not exist in this cluster, in the same shape as
		// one that does — a consumer could not tell a missing artifact from a
		// remote one.
		if b.ConfigPath != "" {
			return infrav1.ArtifactLocator{
				Type: infrav1.LocatorExternalCluster,
				ExternalCluster: &infrav1.ExternalClusterLocator{
					NamespacedObjectKindReference: infrav1.SecretReference(b.Namespace, name),
					Key:                           tfStateSecretKey,
					KubeConfigPath:                b.ConfigPath,
					InClusterConfig:               b.InClusterConfig,
				},
			}
		}

		return infrav1.SecretLocator(b.Namespace, name, tfStateSecretKey)

	case backendCustom:
		return infrav1.ArtifactLocator{
			Type: infrav1.LocatorExternal,
			External: &infrav1.ExternalLocator{
				Backend:  parseBackendType(b.CustomHCL),
				Location: parseBackendExternalLocation(b.CustomHCL),
			},
		}

	case backendCloud:
		return infrav1.ArtifactLocator{
			Type: infrav1.LocatorExternal,
			External: &infrav1.ExternalLocator{
				Backend:  string(backendCloud),
				Location: cloudLocation(b.Cloud),
			},
		}

	case backendLocal:
		return infrav1.ArtifactLocator{
			Type:      infrav1.LocatorEphemeral,
			Ephemeral: &infrav1.EphemeralLocator{},
		}

	default:
		return infrav1.NoneLocator()
	}
}

// cloudLocation renders an advisory hint for a Terraform Cloud workspace.
func cloudLocation(cloud *infrav1.CloudSpec) string {
	if cloud == nil || cloud.Organization == "" {
		return ""
	}

	hint := "cloud://" + cloud.Organization
	if ws := cloud.Workspaces; ws != nil && ws.Name != "" {
		hint += "/" + ws.Name
	}

	return hint
}

// parseBackendType extracts the backend type label from a backend HCL block,
// e.g. `backend "s3" { ... }` -> "s3". Returns "unknown" when the type cannot
// be determined; the controller never errors on this best-effort hint.
func parseBackendType(hcl string) string {
	idx := strings.Index(hcl, `backend "`)
	if idx < 0 {
		return "unknown"
	}
	rest := hcl[idx+len(`backend "`):]
	end := strings.IndexByte(rest, '"')
	if end <= 0 {
		return "unknown"
	}
	return rest[:end]
}

// parseBackendExternalLocation produces a hint for well-known backends in
// custom HCL. It never panics and returns "" when no recognisable hint is
// present — callers must treat the result as advisory only, since the values
// may be Terraform variables this parser cannot resolve.
func parseBackendExternalLocation(hcl string) string {
	switch parseBackendType(hcl) {
	case "s3":
		bucket := extractHCLString(hcl, "bucket")
		key := extractHCLString(hcl, "key")
		if bucket == "" {
			return ""
		}
		if key != "" {
			return fmt.Sprintf("s3://%s/%s", bucket, key)
		}
		return "s3://" + bucket
	case "gcs":
		bucket := extractHCLString(hcl, "bucket")
		prefix := extractHCLString(hcl, "prefix")
		if bucket == "" {
			return ""
		}
		if prefix != "" {
			return fmt.Sprintf("gs://%s/%s", bucket, prefix)
		}
		return "gs://" + bucket
	case "http", "remote":
		return extractHCLString(hcl, "address")
	}
	return ""
}

// extractHCLString returns the value of `<key> = "<value>"` from an HCL
// snippet. Returns "" when the key is absent or the value is not a quoted
// literal — callers must tolerate that.
//
// The key must appear as a whole attribute name, so looking up "bucket" does
// not match `bucket_prefix = "..."`.
func extractHCLString(hcl, key string) string {
	re, err := regexp.Compile(`(?:^|[\s{])` + regexp.QuoteMeta(key) + `\s*=\s*"([^"]*)"`)
	if err != nil {
		return ""
	}

	match := re.FindStringSubmatch(hcl)
	if match == nil {
		return ""
	}

	return match[1]
}
