package controllers

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000240_health_check_test(t *testing.T) {
	Spec("This spec verifies health check functionalities")

	g := NewWithT(t)
	ctx := context.Background()

	tcpTestCases := []struct {
		name    string
		url     string
		timeout time.Duration
		wantErr bool
	}{
		{
			name:    "testTCP",
			url:     "weave.works:80",
			timeout: time.Second * 10,
			wantErr: false,
		},
		{

			name:    "testTCPInvalidPort",
			url:     "weave.works:81",
			timeout: time.Second * 10,
			wantErr: true,
		},
	}

	httpTestCases := []struct {
		name    string
		url     string
		timeout time.Duration
		wantErr bool
	}{
		{
			name:    "testHttp",
			url:     "https://httpbin.org/get",
			timeout: time.Second * 10,
			wantErr: false,
		},
		{
			name:    "testHttp400",
			url:     "https://httpbin.org/status/400",
			timeout: time.Second * 10,
			wantErr: true,
		},
	}

	for _, tt := range tcpTestCases {
		if tt.wantErr {
			It("should do tcp health checks and expecting errors")
			err := reconciler.doTCPHealthCheck(ctx, tt.name, tt.url, tt.timeout)
			g.Expect(err).Should(HaveOccurred())
		} else {
			It("should do tcp health checks and not expecting any errors")
			err := reconciler.doTCPHealthCheck(ctx, tt.name, tt.url, tt.timeout)
			g.Expect(err).ShouldNot(HaveOccurred())
		}
	}

	for _, tt := range httpTestCases {
		if tt.wantErr {
			It("should do http health checks and expecting errors")
			err := reconciler.doHTTPHealthCheck(ctx, tt.name, tt.url, tt.timeout)
			g.Expect(err).Should(HaveOccurred())
		} else {
			It("should do http health checks and not expecting any errors")
			err := reconciler.doHTTPHealthCheck(ctx, tt.name, tt.url, tt.timeout)
			g.Expect(err).ShouldNot(HaveOccurred())
		}
	}
}
