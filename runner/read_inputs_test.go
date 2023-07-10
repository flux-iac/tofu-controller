package runner

import (
	"context"
	. "github.com/onsi/gomega"
	"testing"

	"github.com/go-logr/logr"
	"github.com/hashicorp/hcl2/hcldec"
	"github.com/hashicorp/hcl2/hclparse"
	"github.com/weaveworks/tf-controller/api/typeinfo"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReadInputsForGenerateVarsForTF(t *testing.T) {
	g := NewGomegaWithT(t)

	terraform := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "terraform-1",
			Namespace: "default",
		},
		Spec: infrav1.TerraformSpec{
			ReadInputsFromSecrets: []infrav1.ReadInputsFromSecretSpec{
				{
					Name: "secret-1",
					As:   "secret_1",
				},
			},
		},
	}

	hclParser := hclparse.NewParser()
	file, diag := hclParser.ParseHCL([]byte(`
a = 42
b = "str"
c = true
d = false
e = null
f = [1, 2, 3, 1]
g = [1, "a", true]
h = { a = 1, b = 2, c = 3 }
i = [1, 2, 3, 1]
j = { a = 1, b = 2, c = 3 }
`), "test.hcl")
	g.Expect(diag.HasErrors()).To(BeFalse())
	spec := &hcldec.ObjectSpec{
		"a": &hcldec.AttrSpec{Name: "a", Type: cty.Number},
		"b": &hcldec.AttrSpec{Name: "b", Type: cty.String},
		"c": &hcldec.AttrSpec{Name: "c", Type: cty.Bool},
		"d": &hcldec.AttrSpec{Name: "d", Type: cty.Bool},
		"e": &hcldec.AttrSpec{Name: "e", Type: cty.DynamicPseudoType},
		"f": &hcldec.AttrSpec{Name: "f", Type: cty.List(cty.Number)},
		"g": &hcldec.AttrSpec{Name: "g", Type: cty.Tuple([]cty.Type{cty.Number, cty.String, cty.Bool})},
		"h": &hcldec.AttrSpec{Name: "h", Type: cty.Map(cty.Number)},
		"i": &hcldec.AttrSpec{Name: "i", Type: cty.Set(cty.Number)},
		"j": &hcldec.AttrSpec{Name: "j", Type: cty.Object(map[string]cty.Type{"a": cty.Number, "b": cty.Number, "c": cty.Number})},
	}
	v, err := hcldec.Decode(file.Body, spec, nil)
	g.Expect(err).To(BeNil())

	data := map[string][]byte{}
	for k, vv := range v.AsValueMap() {
		if k == "b" {
			data[k] = []byte(vv.AsString())
			continue
		}

		tt, err := ctyjson.MarshalType(vv.Type())
		g.Expect(err).To(BeNil())
		data[k+typeinfo.Suffix] = tt
		raw, err := ctyjson.Marshal(vv, vv.Type())
		g.Expect(err).To(BeNil())
		data[k] = raw
	}

	fixture := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-1",
			Namespace: "default",
		},
		Data: data,
	}

	cli := fake.NewClientBuilder().WithObjects(fixture).Build()

	inputs, err2 := readInputsForGenerateVarsForTF(context.TODO(), logr.Discard(), cli, terraform)
	g.Expect(err2).To(BeNil())
	g.Expect(inputs["secret_1"]).To(Equal(map[string]interface{}{
		"a": float64(42),
		"b": "str",
		"c": true,
		"d": false,
		"e": nil,
		"f": []interface{}{float64(1), float64(2), float64(3), float64(1)},
		"g": []interface{}{float64(1), "a", true},
		"h": map[string]interface{}{"a": float64(1), "b": float64(2), "c": float64(3)},
		"i": []interface{}{float64(1), float64(2), float64(3)},
		"j": map[string]interface{}{"a": float64(1), "b": float64(2), "c": float64(3)},
	}))

}
