package tfctl

import (
	"bytes"
	"context"
	"os"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/hashicorp/go-version"
	hc "github.com/hashicorp/hc-install"
	"github.com/hashicorp/hc-install/fs"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/src"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestShowPlan(t *testing.T) {
	g := NewWithT(t)

	type args struct {
		ctx context.Context
		c   client.Reader
	}

	tests := []struct {
		name      string
		args      args
		resources func() []client.Object
		want      string
		wantErr   bool
	}{
		{
			name: "hello-world",
			resources: func() []client.Object {
				plan, err := os.ReadFile("testdata/plan.gz")
				g.Expect(err).To(BeNil())
				return []client.Object{
					&infrav1.Terraform{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "hello-world",
							Namespace: "default",
						},
						Status: infrav1.TerraformStatus{
							Plan: infrav1.PlanStatus{
								Pending: "plan-pending",
							},
						},
					},
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "tfplan-default-hello-world",
							Namespace: "default",
						},
						Data: map[string][]byte{
							"tfplan": plan,
						},
					},
				}
			},
			want: `
Changes to Outputs:
  + hello = "world"

You can apply this plan to save these new output values to the Terraform
state, without changing any real infrastructure.
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := hc.NewInstaller()

			v1_1_7 := version.Must(version.NewVersion("1.1.7"))

			tfPath, err := i.Ensure(context.Background(), []src.Source{
				&fs.ExactVersion{
					Product: product.Terraform,
					Version: v1_1_7,
				},
			})

			if err != nil {
				t.Errorf("ShowPlan() error = %v", err)
			}

			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			_ = infrav1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.resources()...).
				Build()

			cli := &CLI{
				namespace: "default",
				terraform: tfPath,
				client:    fakeClient,
			}

			out := &bytes.Buffer{}

			if err := cli.ShowPlan(out, tt.name); (err != nil) != tt.wantErr {
				t.Errorf("ShowPlan() error = %v, wantErr %v", err, tt.wantErr)
			}

			g.Expect(out.String()).To(Equal(tt.want))
		})
	}
}
