package config

import (
	"testing"
)

func Test_ObjectKeyFromName_invalid(t *testing.T) {
	for _, s := range []string{
		"foo/bar/baz",
		"/bar",
		"foo/",
		"",
	} {
		t.Run(s, func(t *testing.T) {
			_, err := ObjectKeyFromName(s)
			if err == nil {
				t.Fatal("expected error due to invalid object name, got nil")
			}
		})
	}
}
