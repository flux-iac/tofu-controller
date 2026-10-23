package tfctl

import (
	"bytes"
	"context"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestClientVersion(t *testing.T) {
	cli := New("abc123", "v0.16.0")
	var out bytes.Buffer
	if err := cli.ClientVersion(&out); err != nil {
		t.Fatalf("ClientVersion() error = %v", err)
	}

	want := "tfctl:\n  build: abc123\n  release: v0.16.0\n"
	if got := out.String(); got != want {
		t.Fatalf("ClientVersion() output = %q, want %q", got, want)
	}
}

func TestClientVersionWriteError(t *testing.T) {
	wantErr := errors.New("write failed")
	cli := New("abc123", "v0.16.0")
	if err := cli.ClientVersion(versionErrorWriter{err: wantErr}); !errors.Is(err, wantErr) {
		t.Fatalf("ClientVersion() error = %v, want %v", err, wantErr)
	}
}

func TestVersion(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tofu-controller",
			Namespace: "custom-namespace",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "tofu-controller",
						Image: "ghcr.io/flux-iac/tofu-controller:v0.16.1",
						Env: []corev1.EnvVar{{
							Name:  "RUNNER_POD_IMAGE",
							Value: "ghcr.io/flux-iac/tf-runner:v0.16.2",
						}},
					}},
				},
			},
		},
	}

	for _, tt := range []struct {
		name    string
		objects []client.Object
		want    string
	}{
		{
			name:    "installed controller",
			objects: []client.Object{deployment},
			want:    "tofu-controller:\n  manager: v0.16.1\n  runner: v0.16.2\ntfctl:\n  build: abc123\n  release: v0.16.0\n",
		},
		{
			name: "controller not found",
			want: "tofu-controller:\n  manager: \n  runner: \ntfctl:\n  build: abc123\n  release: v0.16.0\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli := New("abc123", "v0.16.0")
			cli.namespace = "custom-namespace"
			cli.client = fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...).Build()

			var out bytes.Buffer
			if err := cli.Version(&out); err != nil {
				t.Fatalf("Version() error = %v", err)
			}
			if got := out.String(); got != tt.want {
				t.Fatalf("Version() output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVersionClientError(t *testing.T) {
	wantErr := errors.New("connection failed")
	cli := New("abc123", "v0.16.0")
	cli.client = fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return wantErr
		},
	}).Build()

	var out bytes.Buffer
	if err := cli.Version(&out); !errors.Is(err, wantErr) {
		t.Fatalf("Version() error = %v, want %v", err, wantErr)
	}
	if out.Len() != 0 {
		t.Fatalf("Version() output = %q, want no output on client error", out.String())
	}
}

type versionErrorWriter struct {
	err error
}

func (w versionErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
