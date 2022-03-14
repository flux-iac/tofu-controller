package mtls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"github.com/weaveworks/tf-controller/runner"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

// StartGRPCServerForTesting should be used only for testing
func StartGRPCServerForTesting(ctx context.Context, server *runner.TerraformRunnerServer, namespace string, addr string, mgr controllerruntime.Manager, rotator *CertRotator) error {
	// wait for the certs to be available and the manager to be ready
	<-rotator.Ready
	<-mgr.Elected()

	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      infrav1.RunnerTLSSecretName,
			Labels: map[string]string{
				infrav1.RunnerLabel: namespace,
			},
		},
	}

	hostname := fmt.Sprintf("*.%s.pod.cluster.local", namespace)
	if err := rotator.RefreshRunnerCertIfNeeded(ctx, hostname, tlsSecret); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil
	}

	creds, err := GetGRPCServerCredentials(tlsSecret)
	if err != nil {
		return err
	}

	grpcServer := grpc.NewServer(grpc.Creds(creds))

	// local runner, use the same client as the manager
	runner.RegisterRunnerServer(grpcServer, server)

	if err := grpcServer.Serve(listener); err != nil {
		return err
	}

	return nil
}

// GetGRPCClientCredentials returns transport credentials for a client connection
func GetGRPCClientCredentials(secret *corev1.Secret) (credentials.TransportCredentials, error) {
	ca, cert, err := buildArtifactsFromSecret(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to build artifacts from secret: %v", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(ca.CertPEM) {
		return nil, fmt.Errorf("failed to add client CA")
	}

	serverCert, err := tls.X509KeyPair(cert.CertPEM, cert.KeyPEM)
	if err != nil {
		return nil, err
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		RootCAs:      certPool,
	}

	return credentials.NewTLS(config), nil
}

// GetGRPCServerCredentials returns transport credentials for a server
func GetGRPCServerCredentials(secret *corev1.Secret) (credentials.TransportCredentials, error) {
	ca, cert, err := buildArtifactsFromSecret(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to build artifacts from secret: %v", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(ca.CertPEM) {
		return nil, fmt.Errorf("failed to add client CA")
	}

	serverCert, err := tls.X509KeyPair(cert.CertPEM, cert.KeyPEM)
	if err != nil {
		return nil, err
	}

	config := &tls.Config{
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    certPool,
	}

	return credentials.NewTLS(config), nil
}
