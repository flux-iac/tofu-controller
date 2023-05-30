package runner

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"
)

// Here goes your parseRenamePattern function.
func TestParseRenamePattern(t *testing.T) {
	g := NewGomegaWithT(t)

	tests := []struct {
		pattern   string
		oldKey    string
		newKey    string
		expectErr error
	}{
		{
			pattern:   "oldKey:newKey",
			oldKey:    "oldKey",
			newKey:    "newKey",
			expectErr: nil,
		},
		{
			pattern:   "onlyOldKey",
			oldKey:    "onlyOldKey",
			newKey:    "onlyOldKey",
			expectErr: nil,
		},
		{
			pattern:   "key1:key2:key3",
			oldKey:    "",
			newKey:    "",
			expectErr: errors.New("invalid rename pattern \"key1:key2:key3\""),
		},
		{
			pattern:   ":newKey",
			oldKey:    "",
			newKey:    "",
			expectErr: errors.New("invalid rename pattern old name: \":newKey\""),
		},
		{
			pattern:   "oldKey:",
			oldKey:    "",
			newKey:    "",
			expectErr: errors.New("invalid rename pattern new name: \"oldKey:\""),
		},
	}

	for _, tt := range tests {
		oldKey, newKey, err := parseRenamePattern(tt.pattern)
		if tt.expectErr == nil {
			g.Expect(err).To(BeNil())
		} else {
			g.Expect(err).To(Equal(tt.expectErr))
		}
		g.Expect(oldKey).To(Equal(tt.oldKey))
		g.Expect(newKey).To(Equal(tt.newKey))
	}
}
