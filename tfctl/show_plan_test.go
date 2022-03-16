package tfctl

import (
	"bytes"
	"context"
	"os"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type FakeClient struct {
	resource client.Object
}

func (c *FakeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	switch obj.(type) {
	case *corev1.Secret:
		obj.(*corev1.Secret).Data = c.resource.(*corev1.Secret).Data
	}
	return nil
}

func (c *FakeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return nil
}

func TestShowPlan(t *testing.T) {
	g := NewWithT(t)

	type args struct {
		ctx context.Context
		c   client.Reader
	}

	tests := []struct {
		name     string
		args     args
		resource func() client.Object
		want     string
		wantErr  bool
	}{
		{
			name: "hello-world",
			resource: func() client.Object {
				plan, err := os.ReadFile("testdata/plan.gz")
				g.Expect(err).To(BeNil())
				return &corev1.Secret{
					Data: map[string][]byte{
						"tfplan": plan,
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
			cli := &CLI{
				namespace: "default",
				terraform: "/usr/bin/terraform",
				client: &FakeClient{
					resource: tt.resource(),
				},
			}

			out := &bytes.Buffer{}

			if err := cli.ShowPlan(out, tt.name); (err != nil) != tt.wantErr {
				t.Errorf("ShowPlan() error = %v, wantErr %v", err, tt.wantErr)
			}

			g.Expect(out.String()).To(Equal(tt.want))
		})
	}
}
