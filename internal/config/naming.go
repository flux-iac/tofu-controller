package config

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

func GenerateUniqueHash(components ...string) string {
	name := strings.Join(components, "-")
	hash := sha256.New()
	hash.Write([]byte(name))
	sum := hash.Sum(nil)

	// Use only the first 10 bytes to avoid long names.
	// It can still collide, but less likely.
	return fmt.Sprintf("%x", sum[:10])
}

func PullRequestObjectName(name, prID string) string {
	return fmt.Sprintf("%s-pr-%s", name, prID)
}

func SourceName(tfName, sourceName, prID string) string {
	uniqueName := fmt.Sprintf("%s-%s", sourceName, GenerateUniqueHash(tfName, sourceName, prID))

	return PullRequestObjectName(uniqueName, prID)
}
