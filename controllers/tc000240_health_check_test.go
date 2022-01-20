package controllers

import (
	"context"
	"testing"
	"time"

	infrav1 "github.com/chanwit/tf-controller/api/v1alpha1"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000240_health_check_test(t *testing.T) {
	Spec("This spec verifies health check functionalities")

	g := NewWithT(t)
	ctx := context.Background()

	testCases := []struct {
		healthChecks []infrav1.HealthCheck
		wantErr      bool
	}{
		{
			healthChecks: []infrav1.HealthCheck{
				{
					Name: "testTCP",
					URL:  "weave.works:80",
					Type: "tcp",
				},
				{
					Name: "testHttpGet",
					URL:  "https://httpbin.org/get",
					Type: "httpGet",
				},
				{
					Name: "testHttpPost",
					URL:  "https://httpbin.org/post",
					Type: "httpPost",
				},
			},
			wantErr: false,
		},
		{
			healthChecks: []infrav1.HealthCheck{
				{
					Name:    "testTCPInvalidPort",
					URL:     "weave.works:81",
					Type:    "tcp",
					Timeout: &metav1.Duration{Duration: time.Second * 3},
				},
			},
			wantErr: true,
		},
		{
			healthChecks: []infrav1.HealthCheck{
				{
					Name: "testHttpGet400",
					URL:  "https://httpbin.org/status/400",
					Type: "httpGet",
				},
			},
			wantErr: true,
		},
		{
			healthChecks: []infrav1.HealthCheck{
				{
					Name: "testHttpPost400",
					URL:  "https://httpbin.org/status/400",
					Type: "httpPost",
				},
			},
			wantErr: true,
		},
		{
			healthChecks: []infrav1.HealthCheck{
				{
					Name: "testInvalidHealthCheckType",
					URL:  "weave.works",
					Type: "invalid",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range testCases {
		if tt.wantErr {
			It("should do health checks and expecting errors")
			err := reconciler.doHealthChecks(ctx, tt.healthChecks)
			g.Expect(err).Should(HaveOccurred())
		} else {
			It("should do health checks and not expecting any errors")
			err := reconciler.doHealthChecks(ctx, tt.healthChecks)
			g.Expect(err).ShouldNot(HaveOccurred())
		}
	}
}
