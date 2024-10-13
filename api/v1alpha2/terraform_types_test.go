package v1alpha2

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetRetryInterval(t *testing.T) {
	g := NewGomegaWithT(t)

	tests := []struct {
		name                  string
		terraform             Terraform
		expectedRetryInterval time.Duration
	}{
		{
			name: "default retry interval",
			terraform: Terraform{
				Spec: TerraformSpec{},
			},
			expectedRetryInterval: 15 * time.Second,
		},
		{
			name: "custom retry interval",
			terraform: Terraform{
				Spec: TerraformSpec{
					RetryInterval: &metav1.Duration{Duration: 30 * time.Second},
				},
			},
			expectedRetryInterval: 30 * time.Second,
		},
		{
			name: "exponential backoff with default retry interval",
			terraform: Terraform{
				Spec: TerraformSpec{
					RetryStrategy: ExponentialBackoff,
				},
				Status: TerraformStatus{
					ReconciliationFailures: 2,
				},
			},
			expectedRetryInterval: 60 * time.Second,
		},
		{
			name: "exponential backoff",
			terraform: Terraform{
				Spec: TerraformSpec{
					RetryStrategy: ExponentialBackoff,
					RetryInterval: &metav1.Duration{Duration: 60 * time.Second},
				},
				Status: TerraformStatus{
					ReconciliationFailures: 4,
				},
			},
			expectedRetryInterval: 960 * time.Second,
		},
		{
			name: "exponential backoff with max retry interval",
			terraform: Terraform{
				Spec: TerraformSpec{
					RetryStrategy:    ExponentialBackoff,
					RetryInterval:    &metav1.Duration{Duration: 60 * time.Second},
					MaxRetryInterval: &metav1.Duration{Duration: 45 * time.Second},
				},
				Status: TerraformStatus{
					ReconciliationFailures: 4,
				},
			},
			expectedRetryInterval: 45 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g.Expect(tt.terraform.GetRetryInterval()).To(Equal(tt.expectedRetryInterval))
		})
	}
}
