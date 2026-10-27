package controllers

import (
	"fmt"
	"strings"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func backendTestTF(name string, mutate func(*infrav1.Terraform)) *infrav1.Terraform {
	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{Namespace: "flux-system", Name: name},
	}
	if mutate != nil {
		mutate(tf)
	}
	return tf
}

// One case per row of the storage matrix. Asserting the rendered HCL and the
// published locator from the same resolution is the point of the table: the
// two used to be derived independently, and every bug in this area was them
// disagreeing.
func TestResolveBackend_Matrix(t *testing.T) {
	cases := []struct {
		name              string
		tf                *infrav1.Terraform
		disableK8sBackend bool

		wantKind     backendKind
		wantWrite    bool
		wantHCLHas   []string
		wantLocator  infrav1.LocatorType
		wantStateRef string // expected Secret name, for locators that name one
		wantBackend  string // expected External.Backend
		wantLocation string // expected External.Location
	}{
		{
			name:         "default kubernetes backend",
			tf:           backendTestTF("hello", nil),
			wantKind:     backendKubernetes,
			wantWrite:    true,
			wantHCLHas:   []string{`backend "kubernetes"`, `secret_suffix     = "hello"`, `in_cluster_config = true`},
			wantLocator:  infrav1.LocatorKubernetes,
			wantStateRef: "tfstate-default-hello",
		},
		{
			name: "explicit secret suffix",
			tf: backendTestTF("hello", func(tf *infrav1.Terraform) {
				tf.Spec.BackendConfig = &infrav1.BackendConfigSpec{
					SecretSuffix:    "customsuffix",
					InClusterConfig: true,
				}
			}),
			wantKind:     backendKubernetes,
			wantWrite:    true,
			wantHCLHas:   []string{`secret_suffix     = "customsuffix"`},
			wantLocator:  infrav1.LocatorKubernetes,
			wantStateRef: "tfstate-default-customsuffix",
		},
		{
			name: "empty secret suffix is rendered verbatim",
			tf: backendTestTF("hello", func(tf *infrav1.Terraform) {
				tf.Spec.BackendConfig = &infrav1.BackendConfigSpec{InClusterConfig: true}
			}),
			wantKind:     backendKubernetes,
			wantWrite:    true,
			wantHCLHas:   []string{`secret_suffix     = ""`},
			wantLocator:  infrav1.LocatorKubernetes,
			wantStateRef: "tfstate-default-",
		},
		{
			name: "custom workspace",
			tf: backendTestTF("hello", func(tf *infrav1.Terraform) {
				tf.Spec.Workspace = "staging"
			}),
			wantKind:     backendKubernetes,
			wantWrite:    true,
			wantLocator:  infrav1.LocatorKubernetes,
			wantStateRef: "tfstate-staging-hello",
		},
		{
			name: "kubeconfig points at another cluster",
			tf: backendTestTF("hello", func(tf *infrav1.Terraform) {
				tf.Spec.BackendConfig = &infrav1.BackendConfigSpec{
					SecretSuffix:    "hello",
					InClusterConfig: false,
					ConfigPath:      "/etc/kube/remote.conf",
				}
			}),
			wantKind:     backendKubernetes,
			wantWrite:    true,
			wantHCLHas:   []string{`config_path       = "/etc/kube/remote.conf"`, `in_cluster_config = false`},
			wantLocator:  infrav1.LocatorExternalCluster,
			wantStateRef: "tfstate-default-hello",
		},
		{
			name: "custom s3 backend",
			tf: backendTestTF("hello", func(tf *infrav1.Terraform) {
				tf.Spec.BackendConfig = &infrav1.BackendConfigSpec{
					CustomConfiguration: `backend "s3" {
  bucket = "my-state-bucket"
  key    = "envs/prod/hello.tfstate"
  region = "us-east-1"
}`,
				}
			}),
			wantKind:     backendCustom,
			wantWrite:    true,
			wantHCLHas:   []string{`backend "s3"`, `bucket = "my-state-bucket"`},
			wantLocator:  infrav1.LocatorExternal,
			wantBackend:  "s3",
			wantLocation: "s3://my-state-bucket/envs/prod/hello.tfstate",
		},
		{
			name: "custom gcs backend",
			tf: backendTestTF("hello", func(tf *infrav1.Terraform) {
				tf.Spec.BackendConfig = &infrav1.BackendConfigSpec{
					CustomConfiguration: `backend "gcs" {
  bucket = "my-bucket"
  prefix = "tf/state"
}`,
				}
			}),
			wantKind:     backendCustom,
			wantWrite:    true,
			wantLocator:  infrav1.LocatorExternal,
			wantBackend:  "gcs",
			wantLocation: "gs://my-bucket/tf/state",
		},
		{
			name: "custom backend with bucket_prefix before bucket",
			tf: backendTestTF("hello", func(tf *infrav1.Terraform) {
				tf.Spec.BackendConfig = &infrav1.BackendConfigSpec{
					CustomConfiguration: `backend "s3" {
  bucket_prefix = "ignore-me"
  bucket        = "real-bucket"
  key           = "state.tfstate"
}`,
				}
			}),
			wantKind:     backendCustom,
			wantWrite:    true,
			wantLocator:  infrav1.LocatorExternal,
			wantBackend:  "s3",
			wantLocation: "s3://real-bucket/state.tfstate",
		},
		{
			name: "unknown custom backend yields no hint",
			tf: backendTestTF("hello", func(tf *infrav1.Terraform) {
				tf.Spec.BackendConfig = &infrav1.BackendConfigSpec{
					CustomConfiguration: `backend "consul" {
  path    = "tf/state"
  address = "consul.example.com:8500"
}`,
				}
			}),
			wantKind:     backendCustom,
			wantWrite:    true,
			wantLocator:  infrav1.LocatorExternal,
			wantBackend:  "consul",
			wantLocation: "",
		},
		{
			name: "terraform cloud",
			tf: backendTestTF("hello", func(tf *infrav1.Terraform) {
				tf.Spec.Cloud = &infrav1.CloudSpec{
					Organization: "my-org",
					Workspaces:   &infrav1.CloudWorkspacesSpec{Name: "prod"},
				}
			}),
			wantKind:     backendCloud,
			wantWrite:    true,
			wantHCLHas:   []string{"cloud {", `organization = "my-org"`},
			wantLocator:  infrav1.LocatorExternal,
			wantBackend:  "cloud",
			wantLocation: "cloud://my-org/prod",
		},
		{
			name:              "kubernetes backend disabled controller-wide",
			tf:                backendTestTF("hello", nil),
			disableK8sBackend: true,
			wantKind:          backendLocal,
			wantWrite:         true,
			wantHCLHas:        []string{`backend "local"`},
			wantLocator:       infrav1.LocatorEphemeral,
		},
		{
			name: "backend disabled on the object",
			tf: backendTestTF("hello", func(tf *infrav1.Terraform) {
				tf.Spec.BackendConfig = &infrav1.BackendConfigSpec{Disable: true}
			}),
			wantKind:    backendDisabled,
			wantWrite:   false,
			wantLocator: infrav1.LocatorNone,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			backend := resolveBackend(tc.tf, tc.disableK8sBackend)
			g.Expect(backend.Kind).To(Equal(tc.wantKind))

			hcl, write := backend.HCL()
			g.Expect(write).To(Equal(tc.wantWrite), "whether a backend config is written")
			for _, want := range tc.wantHCLHas {
				g.Expect(hcl).To(ContainSubstring(want))
			}
			if !tc.wantWrite {
				g.Expect(hcl).To(BeEmpty())
			}

			locator := backend.StateLocator(tc.tf)
			g.Expect(locator.Type).To(Equal(tc.wantLocator))

			switch locator.Type {
			case infrav1.LocatorKubernetes:
				g.Expect(locator.Kubernetes).NotTo(BeNil())
				g.Expect(locator.Kubernetes.Kind).To(Equal("Secret"))
				g.Expect(locator.Kubernetes.APIVersion).To(Equal("v1"))
				g.Expect(locator.Kubernetes.Namespace).To(Equal(tc.tf.Namespace))
				g.Expect(locator.Kubernetes.Name).To(Equal(tc.wantStateRef))
				g.Expect(locator.Kubernetes.Key).To(Equal(tfStateSecretKey))
			case infrav1.LocatorExternalCluster:
				g.Expect(locator.ExternalCluster).NotTo(BeNil())
				g.Expect(locator.ExternalCluster.Name).To(Equal(tc.wantStateRef))
				g.Expect(locator.ExternalCluster.KubeConfigPath).NotTo(BeEmpty(),
					"an external-cluster locator must say which kubeconfig addresses it")
				g.Expect(locator.Kubernetes).To(BeNil(),
					"a remote object must not be published as locally resolvable")
			case infrav1.LocatorExternal:
				g.Expect(locator.External).NotTo(BeNil())
				g.Expect(locator.External.Backend).To(Equal(tc.wantBackend))
				g.Expect(locator.External.Location).To(Equal(tc.wantLocation))
				g.Expect(locator.Kubernetes).To(BeNil())
			case infrav1.LocatorNone:
				g.Expect(locator.Kubernetes).To(BeNil())
				g.Expect(locator.External).To(BeNil())
				g.Expect(locator.Ephemeral).To(BeNil())
			}

			// Only an in-cluster locator may be treated as resolvable here.
			_, resolvable := locator.KubernetesRef()
			g.Expect(resolvable).To(Equal(locator.Type == infrav1.LocatorKubernetes))
		})
	}
}

// The drift guard: for every Kubernetes-backed configuration, the Secret named
// in status must be the Secret the rendered HCL will actually produce. This
// reads secret_suffix back out of the HCL instead of restating the expectation,
// so a change to either side that is not mirrored in the other fails here.
func TestResolveBackend_LocatorMatchesRenderedHCL(t *testing.T) {
	configs := []*infrav1.Terraform{
		backendTestTF("hello", nil),
		backendTestTF("hello", func(tf *infrav1.Terraform) {
			tf.Spec.BackendConfig = &infrav1.BackendConfigSpec{SecretSuffix: "customsuffix", InClusterConfig: true}
		}),
		backendTestTF("hello", func(tf *infrav1.Terraform) {
			tf.Spec.BackendConfig = &infrav1.BackendConfigSpec{InClusterConfig: true}
		}),
		backendTestTF("hello", func(tf *infrav1.Terraform) {
			tf.Spec.Workspace = "staging"
		}),
		backendTestTF("hello", func(tf *infrav1.Terraform) {
			tf.Spec.BackendConfig = &infrav1.BackendConfigSpec{
				SecretSuffix: "remote", ConfigPath: "/etc/kube/remote.conf",
			}
		}),
	}

	for i, tf := range configs {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			g := NewWithT(t)

			backend := resolveBackend(tf, false)
			g.Expect(backend.Kind).To(Equal(backendKubernetes))

			hcl, write := backend.HCL()
			g.Expect(write).To(BeTrue())

			// Derive the Secret name from the HCL the runner will execute.
			suffix := extractHCLString(hcl, "secret_suffix")
			namespace := extractHCLString(hcl, "namespace")
			if !strings.Contains(hcl, `secret_suffix     = ""`) {
				g.Expect(suffix).NotTo(BeEmpty())
			}
			expected := fmt.Sprintf("tfstate-%s-%s", tf.WorkspaceName(), suffix)

			locator := backend.StateLocator(tf)
			switch locator.Type {
			case infrav1.LocatorKubernetes:
				g.Expect(locator.Kubernetes.Name).To(Equal(expected))
				g.Expect(locator.Kubernetes.Namespace).To(Equal(namespace))
			case infrav1.LocatorExternalCluster:
				g.Expect(locator.ExternalCluster.Name).To(Equal(expected))
				g.Expect(locator.ExternalCluster.Namespace).To(Equal(namespace))
			default:
				t.Fatalf("kubernetes backend produced locator type %q", locator.Type)
			}
		})
	}
}

func TestParseBackendType(t *testing.T) {
	g := NewWithT(t)
	g.Expect(parseBackendType(`backend "s3" { bucket = "x" }`)).To(Equal("s3"))
	g.Expect(parseBackendType(`backend "remote" {}`)).To(Equal("remote"))
	g.Expect(parseBackendType(`no backend block here`)).To(Equal("unknown"))
	g.Expect(parseBackendType(`backend "" {}`)).To(Equal("unknown"))
}

func TestExtractHCLString(t *testing.T) {
	g := NewWithT(t)
	hcl := `backend "s3" {
  bucket = "my-bucket"
  region = "us-east-1"
}`
	g.Expect(extractHCLString(hcl, "bucket")).To(Equal("my-bucket"))
	g.Expect(extractHCLString(hcl, "region")).To(Equal("us-east-1"))
	g.Expect(extractHCLString(hcl, "missing")).To(BeEmpty())
}

func TestExtractHCLString_WholeKeyOnly(t *testing.T) {
	g := NewWithT(t)
	hcl := `backend "s3" {
  bucket_prefix = "not-the-bucket"
  bucket        = "the-bucket"
}`
	// "bucket" must not be satisfied by the bucket_prefix line that precedes it.
	g.Expect(extractHCLString(hcl, "bucket")).To(Equal("the-bucket"))
	g.Expect(extractHCLString(hcl, "bucket_prefix")).To(Equal("not-the-bucket"))
	// A key that only exists as a substring of another key is absent.
	g.Expect(extractHCLString(hcl, "prefix")).To(BeEmpty())
}

func TestExtractHCLString_SingleLineBlock(t *testing.T) {
	g := NewWithT(t)
	g.Expect(extractHCLString(`backend "s3" { bucket = "inline" }`, "bucket")).To(Equal("inline"))
}

func TestCloudLocation(t *testing.T) {
	g := NewWithT(t)
	g.Expect(cloudLocation(nil)).To(BeEmpty())
	g.Expect(cloudLocation(&infrav1.CloudSpec{})).To(BeEmpty())
	g.Expect(cloudLocation(&infrav1.CloudSpec{Organization: "org"})).To(Equal("cloud://org"))
	g.Expect(cloudLocation(&infrav1.CloudSpec{
		Organization: "org",
		Workspaces:   &infrav1.CloudWorkspacesSpec{Name: "ws"},
	})).To(Equal("cloud://org/ws"))
}
