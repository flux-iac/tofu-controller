package mtls

import (
	"testing"
	"time"
)

func TestShouldDeleteTLSSecret(t *testing.T) {
	ref := time.Date(2026, 11, 9, 19, 44, 23, 298000000, time.UTC).Truncate(time.Second)
	currentName := "terraform-runner.tls-1794512663"
	current := map[string]struct{}{currentName: {}}

	sameSecond := time.Date(2026, 11, 9, 19, 44, 23, 0, time.UTC) // API metav1.Time truncation
	older := sameSecond.Add(-time.Second)
	newer := sameSecond.Add(time.Second)

	tests := []struct {
		name    string
		secret  string
		created time.Time
		want    bool
	}{
		{"same-second current CA kept", currentName, sameSecond, false},
		{"same-second other secret kept by time", "terraform-runner.tls-old", sameSecond, false},
		{"older other secret deleted", "terraform-runner.tls-old", older, true},
		{"older current CA still kept", currentName, older, false},
		{"newer secret kept", "terraform-runner.tls-new", newer, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldDeleteTLSSecret(tt.secret, tt.created, ref, current)
			if got != tt.want {
				t.Fatalf("shouldDeleteTLSSecret(%q, %v) = %v, want %v", tt.secret, tt.created, got, tt.want)
			}
		})
	}
}

func TestReferenceTimeTruncationMatchesAPIServer(t *testing.T) {
	started := time.Date(2026, 11, 9, 19, 44, 23, 298000000, time.UTC)
	referenceTime := started.Truncate(time.Second)
	secretCreated := time.Date(2026, 11, 9, 19, 44, 23, 0, time.UTC) // CreationTimestamp second precision

	if secretCreated.Before(referenceTime) {
		t.Fatalf("secret created in the same second as Start must not be Before truncated referenceTime")
	}
	if !secretCreated.Before(started) {
		t.Fatalf("without truncation the nanosecond referenceTime would treat the secret as older")
	}
}
