package mtls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"

	"github.com/weaveworks/tf-controller/runner"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	corev1 "k8s.io/api/core/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

// TODO: this should live somewhere else but for now it's here
func StartGRPCServer(ctx context.Context, server *runner.TerraformRunnerServer, addr string, mgr controllerruntime.Manager, rotator *CertRotator) error {
	// wait for the certs to be available and the manager to be ready
	<-rotator.Ready
	<-mgr.Elected()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil
	}

	tlsSecret := &corev1.Secret{}
	if err := mgr.GetClient().Get(ctx, rotator.SecretKey, tlsSecret); err != nil {
		return err
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
