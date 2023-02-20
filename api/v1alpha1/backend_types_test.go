package v1alpha1

import (
	. "github.com/onsi/gomega"
	"testing"
)

func TestBackend_with_Local_ToHCL(t *testing.T) {
	g := NewGomegaWithT(t)
	b := &Backend{
		Local: &LocalBackend{},
	}
	hcl, err := b.ToHCL()
	g.Expect(err).To(BeNil())
	g.Expect(hcl).To(Equal(`backend "local" {}`))
}

func TestInvalidBackend(t *testing.T) {
	g := NewGomegaWithT(t)
	b := &Backend{}
	_, err := b.ToHCL()
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(Equal("at least one field of the Backend struct must be non-nil"))
}

func TestInvalidBackendWhenTwoAreSet(t *testing.T) {
	g := NewGomegaWithT(t)
	b := &Backend{
		Local: &LocalBackend{},
		S3:    &S3Backend{},
	}
	_, err := b.ToHCL()
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(Equal("only one field of the Backend struct can be non-nil at a time"))
}

func TestInvalidBackendWhenThreeAreSet(t *testing.T) {
	g := NewGomegaWithT(t)
	b := &Backend{
		Local:   &LocalBackend{},
		AzureRM: &AzureRMBackend{},
		S3:      &S3Backend{},
	}
	_, err := b.ToHCL()
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(Equal("only one field of the Backend struct can be non-nil at a time"))
}
