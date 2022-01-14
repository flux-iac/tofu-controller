package controllers

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"testing"

	. "github.com/onsi/gomega"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000210_plan_encode_decode_test(t *testing.T) {
	Spec("This spec verifies gzip encoding and decoding func")

	g := NewWithT(t)

	encodeTests := []struct {
		tfplan []byte
	}{
		{
			tfplan: []byte("content"),
		},
	}

	for _, tt := range encodeTests {
		It("should encode the terraform plan")
		r, err := reconciler.gzipEncode(tt.tfplan)
		g.Expect(err).ShouldNot(HaveOccurred())

		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		_, _ = w.Write(tt.tfplan)
		w.Close()
		g.Expect(r).Should(Equal(buf.Bytes()))
	}

	decodeTests := []struct {
		encodedPlan []byte
	}{
		{
			encodedPlan: []byte("\x1f\x8b\b\x00\x00\x00\x00\x00\x00\xffJ\xce\xcf+I\xcd+\x01\x04\x00\x00\xff\xff\xa90\xc5\xfe\a\x00\x00\x00"),
		},
	}

	for _, tt := range decodeTests {
		It("should decode the encoded terraform plan")
		r, err := reconciler.gzipDecode(tt.encodedPlan)
		g.Expect(err).ShouldNot(HaveOccurred())

		re := bytes.NewReader(tt.encodedPlan)
		gr, _ := gzip.NewReader(re)
		o, _ := ioutil.ReadAll(gr)
		g.Expect(r).Should(Equal(o))
	}
}
