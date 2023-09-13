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
		address string
		timeout time.Duration
		wantErr bool
	}{
		{
			name:    "testTCP",
			address: "weave.works:80",
			timeout: time.Second * 10,
			wantErr: false,
		},
		{
			name:    "testTCPInvalidPort",
			address: "localhost:81",
			timeout: time.Second * 10,
			wantErr: true,
		},
		{
			name:    "testTCPAddress",
			address: "weave.works",
			timeout: time.Second * 10,
			wantErr: true,
		},
		{
			name:    "testTCPEmptyAddress",
			address: "",
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
			url:     server.URL() + "/get",
			timeout: time.Second * 10,
			wantErr: false,
		},
		{
			name:    "testHttp400",
			url:     server.URL() + "/bad-request",
			timeout: time.Second * 10,
			wantErr: true,
		},
		{
			name:    "testInvalidHttpUrl",
			url:     "invalid.com",
			timeout: time.Second * 10,
			wantErr: true,
		},
		{
			name:    "testEmptyHttpUrl",
			url:     "",
			timeout: time.Second * 10,
			wantErr: true,
		},
	}

	for _, tt := range tcpTestCases {
		if tt.wantErr {
			It("should do tcp health checks and expecting errors")
			err := reconciler.doTCPHealthCheck(ctx, tt.name, tt.address, tt.timeout)
			g.Expect(err).Should(HaveOccurred())
		} else {
			It("should do tcp health checks and not expecting any errors")
			err := reconciler.doTCPHealthCheck(ctx, tt.name, tt.address, tt.timeout)
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
