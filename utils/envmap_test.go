package utils

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestEnvMap(t *testing.T) {
	g := NewWithT(t)
	g.Expect(EnvMap([]string{"A=a", "B=b", "C"})).To(Equal(map[string]string{"A": "a", "B": "b"}))
}
