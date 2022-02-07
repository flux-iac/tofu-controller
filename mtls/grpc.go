package mtls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"google.golang.org/grpc/credentials"
	corev1 "k8s.io/api/core/v1"
)

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
