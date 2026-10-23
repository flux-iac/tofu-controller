package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestVersionWithoutCluster(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected Kubernetes request", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	unreachable := httptest.NewServer(http.NotFoundHandler())
	unreachable.Close()

	tests := []struct {
		name   string
		config string
	}{
		{name: "missing kubeconfig"},
		{name: "malformed kubeconfig", config: "[invalid yaml"},
		{name: "unreachable API", config: versionKubeconfig(unreachable.URL)},
		{name: "no API requests", config: versionKubeconfig(server.URL)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetVersionGlobals(t)
			path := writeVersionKubeconfig(t, tt.config)
			cmd := newRootCommand()
			out := &bytes.Buffer{}
			cmd.SetOut(out)
			cmd.SetArgs([]string{"version", "--kubeconfig", path})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("version failed without a cluster: %v", err)
			}
			if got, want := out.String(), "tfctl:\n  build: test-build\n  release: test-release\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("version made %d Kubernetes requests", got)
	}
}

func TestVersionRemote(t *testing.T) {
	resetVersionGlobals(t)
	var deploymentRequests atomic.Int32
	var forbidden atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var response any
		switch r.URL.Path {
		case "/api":
			response = metav1.APIVersions{Versions: []string{"v1"}}
		case "/apis":
			response = metav1.APIGroupList{Groups: []metav1.APIGroup{{
				Name:             "apps",
				Versions:         []metav1.GroupVersionForDiscovery{{GroupVersion: "apps/v1", Version: "v1"}},
				PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "apps/v1", Version: "v1"},
			}}}
		case "/apis/apps/v1":
			response = metav1.APIResourceList{
				GroupVersion: "apps/v1",
				APIResources: []metav1.APIResource{{Name: "deployments", Kind: "Deployment", Namespaced: true}},
			}
		case "/apis/apps/v1/namespaces/custom/deployments/tofu-controller":
			deploymentRequests.Add(1)
			if forbidden.Load() {
				w.WriteHeader(http.StatusForbidden)
				response = metav1.Status{Status: "Failure", Reason: metav1.StatusReasonForbidden, Code: http.StatusForbidden}
				break
			}
			response = appsv1.Deployment{
				TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
				ObjectMeta: metav1.ObjectMeta{Name: "tofu-controller", Namespace: "custom"},
				Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "tofu-controller", Image: "ghcr.io/flux-iac/tofu-controller:v0.16.0",
						Env: []corev1.EnvVar{{Name: "RUNNER_POD_IMAGE", Value: "ghcr.io/flux-iac/tf-runner:v0.16.1"}},
					}},
				}}},
			}
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("encoding response: %v", err)
		}
	}))
	defer server.Close()
	cmd := newRootCommand()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"version", "--remote", "--namespace", "custom", "--kubeconfig", writeVersionKubeconfig(t, versionKubeconfig(server.URL))})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	want := "tofu-controller:\n  manager: v0.16.0\n  runner: v0.16.1\ntfctl:\n  build: test-build\n  release: test-release\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if got := deploymentRequests.Load(); got != 1 {
		t.Fatalf("deployment requests = %d, want 1", got)
	}

	// Remote errors must still fail the command rather than fall back to local output.
	forbidden.Store(true)
	cmd = newRootCommand()
	out.Reset()
	cmd.SetOut(out)
	cmd.SetArgs([]string{"version", "--remote", "--namespace", "custom", "--kubeconfig", writeVersionKubeconfig(t, versionKubeconfig(server.URL))})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "failed to get tofu-controller deployment") {
		t.Fatalf("remote API error = %v, want a deployment lookup failure", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want no output on remote error", out.String())
	}
}

func TestClusterCommandsRequireKubeconfig(t *testing.T) {
	for _, args := range [][]string{{"version", "--remote"}, {"get"}} {
		t.Run(args[0], func(t *testing.T) {
			resetVersionGlobals(t)
			cmd := newRootCommand()
			cmd.SetArgs(append(args, "--kubeconfig", writeVersionKubeconfig(t, "")))
			if err := cmd.Execute(); err == nil {
				t.Fatal("expected a kubeconfig error")
			}
		})
	}
}

func resetVersionGlobals(t *testing.T) {
	t.Helper()
	oldConfig, oldBuild, oldVersion := kubeconfigArgs, BuildSHA, BuildVersion
	t.Cleanup(func() {
		kubeconfigArgs, BuildSHA, BuildVersion = oldConfig, oldBuild, oldVersion
	})
	kubeconfigArgs = genericclioptions.NewConfigFlags(false)
	BuildSHA, BuildVersion = "test-build", "test-release"
	t.Setenv("FLUX_SYSTEM_NAMESPACE", "")
}

func writeVersionKubeconfig(t *testing.T, config string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	if config != "" {
		if err := os.WriteFile(path, []byte(config), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func versionKubeconfig(server string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: %s
contexts:
- name: test
  context:
    cluster: test
    user: test
current-context: test
users:
- name: test
  user: {}
`, server)
}
