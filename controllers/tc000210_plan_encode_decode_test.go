package controllers

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"testing"

	infrav1 "github.com/chanwit/tf-controller/api/v1alpha1"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000210_plan_encode_decode(t *testing.T) {
	Spec("This spec describes behaviour when encoding method is specified")

	g := NewWithT(t)

	encodeTests := []struct {
		tf      infrav1.Terraform
		tfplan  []byte
		wantErr bool
	}{
		{
			tf: infrav1.Terraform{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						"encoding": "gzip",
					},
				},
			},
			tfplan: []byte("content"),
		},
		{
			tf: infrav1.Terraform{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						"encoding": "invalid",
					},
				},
			},
			tfplan:  []byte("content"),
			wantErr: true,
		},
	}

	for _, tt := range encodeTests {
		if tt.wantErr {
			It("should error out due to invalid encoding method specified")
			_, err := reconciler.encodePlan(tt.tf, tt.tfplan)
			g.Expect(err).Should(HaveOccurred())
		} else {
			It("should encode base on the encoding method specified")
			r, err := reconciler.encodePlan(tt.tf, tt.tfplan)
			g.Expect(err).ShouldNot(HaveOccurred())

			var buf bytes.Buffer
			w := gzip.NewWriter(&buf)
			_, _ = w.Write(tt.tfplan)
			w.Close()
			g.Expect(r).Should(Equal(buf.Bytes()))
		}
	}

	decodeTests := []struct {
		tf          infrav1.Terraform
		encodedPlan []byte
		wantErr     bool
	}{
		{
			tf: infrav1.Terraform{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						"encoding": "gzip",
					},
				},
			},
			encodedPlan: []byte("\x1f\x8b\b\x00\x00\x00\x00\x00\x00\xffJ\xce\xcf+I\xcd+\x01\x04\x00\x00\xff\xff\xa90\xc5\xfe\a\x00\x00\x00"),
		},
		{
			tf: infrav1.Terraform{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						"encoding": "invalid",
					},
				},
			},
			encodedPlan: []byte("\x1f\x8b\b\x00\x00\x00\x00\x00\x00\xffJ\xce\xcf+I\xcd+\x01\x04\x00\x00\xff\xff\xa90\xc5\xfe\a\x00\x00\x00"),
			wantErr:     true,
		},
	}

	for _, tt := range decodeTests {
		if tt.wantErr {
			It("should error out due to invalid encoding method specified")
			_, err := reconciler.decodePlan(tt.tf, tt.encodedPlan)
			g.Expect(err).Should(HaveOccurred())
		} else {
			It("should encode base on the encoding method specified")
			r, err := reconciler.decodePlan(tt.tf, tt.encodedPlan)
			g.Expect(err).ShouldNot(HaveOccurred())

			re := bytes.NewReader(tt.encodedPlan)
			gr, _ := gzip.NewReader(re)
			o, _ := ioutil.ReadAll(gr)
			g.Expect(r).Should(Equal(o))
		}
	}
}
