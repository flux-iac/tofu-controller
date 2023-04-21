package v1alpha2

import "testing"
import . "github.com/onsi/gomega"

func TestTrimString(t *testing.T) {
	g := NewGomegaWithT(t)

	g.Expect(trimString("hello world", 5)).To(Equal("hello..."))
	g.Expect(trimString("hello world", 11)).To(Equal("hello world"))
	g.Expect(trimString("", 5)).To(Equal(""))
	g.Expect(trimString("hello world", -5)).To(Equal("hel..."))
	g.Expect(trimString("hello world", 0)).To(Equal("hel..."))
}
