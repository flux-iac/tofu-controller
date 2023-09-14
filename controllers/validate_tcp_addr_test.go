package controllers

import "testing"

func TestValidateTCPAddress(t *testing.T) {
	tests := []struct {
		address string
		valid   bool
	}{
		{"127.0.0.1:80", true},
		{"localhost:8080", true},
		{"google.com:443", true},
		{"a.b.c.d.e.f:443", true},
		{"invalid+address:80", false},
		{"invalid_address:80", false},
		{"127.0.0.1:invalid_port", false},
		{"127.0.0.1:70000", false},
		{"https://localhost:443", false},
	}

	for _, test := range tests {
		err := validateTCPAddress(test.address)
		if (err == nil) != test.valid {
			t.Errorf("validateTCPAddress(%q) = %v, want %v", test.address, err == nil, test.valid)
		}
	}
}
