package runner_test

import (
	"os"
	"path/filepath"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/zclconf/go-cty/cty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"

	. "github.com/onsi/gomega"
)

func secret(name string, data map[string]string) *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Data:       map[string][]byte{},
	}
	for k, v := range data {
		s.Data[k] = []byte(v)
	}
	return s
}

func rawJSON(s string) *apiextensionsv1.JSON { return &apiextensionsv1.JSON{Raw: []byte(s)} }

// generateVars runs GenerateVarsForTF for tf and returns the file it wrote.
func generateVars(t *testing.T, tf *infrav1.Terraform, objs ...client.Object) (string, error) {
	t.Helper()

	h := newHarness(t, objs...)
	wd := t.TempDir()
	id := h.newSession(t, tf, newStubTerraform(t).Path(), wd)

	_, err := h.Client.GenerateVarsForTF(t.Context(), &runner.GenerateVarsForTFRequest{TfInstance: id, WorkingDir: wd})
	if err != nil {
		return "", err
	}
	b, readErr := os.ReadFile(filepath.Join(wd, "generated.auto.tfvars.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(b), nil
}

func TestCharacterisation_GenerateVarsForTF(t *testing.T) {
	t.Run("values template", func(t *testing.T) {
		g := NewWithT(t)
		tf := newTerraform("app")
		tf.Spec.ReadInputsFromSecrets = []infrav1.ReadInputsFromSecretSpec{{Name: "in", As: "in"}}
		tf.Spec.Values = rawJSON(`{"name":"${{ .in.name }}"}`)

		out, err := generateVars(t, tf, secret("in", map[string]string{"name": "alice"}))

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(out).To(Equal(`{"values":{"name":"alice"}}`))
	})

	t.Run("vars", func(t *testing.T) {
		g := NewWithT(t)
		tf := newTerraform("app")
		tf.Spec.Vars = []infrav1.Variable{
			{Name: "n", Value: rawJSON(`3`)},
			{Name: "s", Value: rawJSON(`"x"`)},
		}

		out, err := generateVars(t, tf)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(out).To(Equal(`{"n":3,"s":"x"}`))
	})

	t.Run("varsFrom secrets with the same key", func(t *testing.T) {
		g := NewWithT(t)
		tf := newTerraform("app")
		tf.Spec.Vars = []infrav1.Variable{{Name: "k", Value: rawJSON(`"from-vars"`)}}
		tf.Spec.VarsFrom = []infrav1.VarsReference{
			{Kind: "Secret", Name: "first"},
			{Kind: "Secret", Name: "second"},
		}

		out, err := generateVars(t, tf,
			secret("first", map[string]string{"k": "one"}),
			secret("second", map[string]string{"k": "two"}))

		g.Expect(err).NotTo(HaveOccurred())
		// The last varsFrom entry wins, and varsFrom beats vars.
		g.Expect(out).To(Equal(`{"k":"two"}`))
	})

	t.Run("varsKeys rename", func(t *testing.T) {
		g := NewWithT(t)
		tf := newTerraform("app")
		tf.Spec.VarsFrom = []infrav1.VarsReference{
			{Kind: "Secret", Name: "s", VarsKeys: []string{"a:b"}},
		}

		out, err := generateVars(t, tf, secret("s", map[string]string{"a": "1", "c": "2"}))

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(out).To(Equal(`{"b":"1"}`))
	})

	t.Run("optional missing secret", func(t *testing.T) {
		g := NewWithT(t)
		tf := newTerraform("app")
		tf.Spec.VarsFrom = []infrav1.VarsReference{
			{Kind: "Secret", Name: "absent", Optional: true},
		}

		out, err := generateVars(t, tf)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(out).To(Equal(`{}`))
	})

	t.Run("optional missing secret with varsKeys", func(t *testing.T) {
		g := NewWithT(t)
		tf := newTerraform("app")
		tf.Spec.VarsFrom = []infrav1.VarsReference{
			{Kind: "Secret", Name: "absent", Optional: true, VarsKeys: []string{"a"}},
		}

		out, err := generateVars(t, tf)

		g.Expect(err).NotTo(HaveOccurred())
		// The API docs say a missing varsKey fails the reconcile. Today an
		// absent optional secret quietly yields an empty string for the key.
		g.Expect(out).To(Equal(`{"a":""}`))
	})

	t.Run("required missing secret", func(t *testing.T) {
		g := NewWithT(t)
		tf := newTerraform("app")
		tf.Spec.VarsFrom = []infrav1.VarsReference{{Kind: "Secret", Name: "absent"}}

		_, err := generateVars(t, tf)

		g.Expect(err).To(HaveOccurred())
		g.Expect(status.Code(err)).To(Equal(codes.NotFound))
		g.Expect(status.Convert(err).Message()).To(Equal(`secrets "absent" not found`))
	})

	t.Run("readInputsFromSecrets set of numbers with a duplicate", func(t *testing.T) {
		g := NewWithT(t)
		tf := newTerraform("app")
		tf.Spec.ReadInputsFromSecrets = []infrav1.ReadInputsFromSecretSpec{{Name: "in", As: "in"}}
		tf.Spec.Values = rawJSON(`{"count":"${{ len .in.nums }}","first":"${{ index .in.nums 0 }}","last":"${{ index .in.nums 1 }}"}`)

		out, err := generateVars(t, tf, secret("in", map[string]string{
			"nums":       `[1,2,2]`,
			"nums__type": `["set","number"]`,
		}))

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(out).To(Equal(`{"values":{"count":"2","first":"1","last":"2"}}`))
	})
}

func TestCharacterisation_GenerateVarsForTF_SetOfObjects(t *testing.T) {
	g := NewWithT(t)
	set := cty.SetVal([]cty.Value{
		cty.ObjectVal(map[string]cty.Value{"a": cty.StringVal("1")}),
		cty.ObjectVal(map[string]cty.Value{"a": cty.StringVal("2")}),
	})

	// A set of objects converts to a list of maps, in cty's set order. cty
	// sets are already unique, so the converter keeps every member.
	var (
		got any
		err error
	)
	g.Expect(func() { got, err = runner.CtyValueToGoValue(set) }).NotTo(Panic())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal([]any{
		map[string]any{"a": "1"},
		map[string]any{"a": "2"},
	}))
}

func TestCharacterisation_Inventory(t *testing.T) {
	t.Run("nested modules with arn preferred over id", func(t *testing.T) {
		g := NewWithT(t)
		root := &tfjson.StateModule{
			Resources: []*tfjson.StateResource{
				{Name: "a", Type: "aws_s3_bucket", AttributeValues: map[string]any{"id": "bucket-id", "arn": "arn:aws:s3:::bucket"}},
				{Name: "b", Type: "random_id", AttributeValues: map[string]any{"id": "only-id"}},
				{Name: "c", Type: "terraform_data", AttributeValues: map[string]any{}},
			},
			ChildModules: []*tfjson.StateModule{{
				Resources: []*tfjson.StateResource{
					{Name: "d", Type: "random_id", AttributeValues: map[string]any{"id": "child-id"}},
				},
				ChildModules: []*tfjson.StateModule{{
					Resources: []*tfjson.StateResource{
						{Name: "e", Type: "aws_sqs_queue", AttributeValues: map[string]any{"arn": "arn:aws:sqs:::q"}},
					},
				}},
			}},
		}

		got := runner.GetInventoryFromTerraformModule(root)

		var flat [][3]string
		for _, inv := range got {
			flat = append(flat, [3]string{inv.Name, inv.Type, inv.Identifier})
		}
		g.Expect(flat).To(Equal([][3]string{
			{"a", "aws_s3_bucket", "arn:aws:s3:::bucket"},
			{"b", "random_id", "only-id"},
			{"c", "terraform_data", ""},
			{"d", "random_id", "child-id"},
			{"e", "aws_sqs_queue", "arn:aws:sqs:::q"},
		}))
	})

	t.Run("null or non-string identifiers are skipped", func(t *testing.T) {
		g := NewWithT(t)
		root := &tfjson.StateModule{
			Resources: []*tfjson.StateResource{
				{Name: "a", Type: "t", AttributeValues: map[string]any{"id": nil}},
				{Name: "b", Type: "t", AttributeValues: map[string]any{"id": float64(42)}},
				{Name: "c", Type: "t", AttributeValues: map[string]any{"arn": nil, "id": "c-id"}},
			},
		}

		var got []*runner.Inventory
		g.Expect(func() { got = runner.GetInventoryFromTerraformModule(root) }).NotTo(Panic())

		g.Expect(got).To(HaveLen(3))
		g.Expect(got[0].Identifier).To(Equal(""))
		g.Expect(got[1].Identifier).To(Equal(""))
		// A null arn falls back to the id.
		g.Expect(got[2].Identifier).To(Equal("c-id"))
	})
}

// A secret input whose type information is bad fails the call, and neither
// the error nor anything else the runner logs carries the secret value.
func TestCharacterisation_ReadInputsBadTypeDoesNotLogValue(t *testing.T) {
	for name, data := range map[string]map[string]string{
		"type does not parse":           {"pw": "hunter2", "pw__type": `not a type`},
		"value does not match type":     {"pw": `"hunter2"`, "pw__type": `"number"`},
		"unquoted string":               {"pw": "hunter2", "pw__type": `"string"`},
		"object with an unexpected key": {"pw": `{"hunter2":"x"}`, "pw__type": `["object",{"a":"string"}]`},
	} {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			tf := newTerraform("app")
			tf.Spec.ReadInputsFromSecrets = []infrav1.ReadInputsFromSecretSpec{{Name: "in", As: "in"}}
			h := newHarness(t, secret("in", data))
			wd := t.TempDir()
			id := h.newSession(t, tf, newStubTerraform(t).Path(), wd)

			_, err := h.Client.GenerateVarsForTF(t.Context(), &runner.GenerateVarsForTFRequest{TfInstance: id, WorkingDir: wd})

			g.Expect(err).To(HaveOccurred())
			for _, text := range []string{status.Convert(err).Message(), h.Logs.String()} {
				g.Expect(text).NotTo(ContainSubstring("hunter2"))
				g.Expect(text).NotTo(ContainSubstring("'h'"))
			}
			g.Expect(h.Logs.String()).To(ContainSubstring(`"msg"="rpc failed"`))
		})
	}
}
