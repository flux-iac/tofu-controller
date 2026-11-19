package controllers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestLoadWebhookCACertPool(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	_, err := loadWebhookCACertPool(filepath.Join(dir, "missing.crt"))
	g.Expect(err).To(MatchError(ContainSubstring("failed to read webhook CA certificate")))

	notPEM := filepath.Join(dir, "not-pem.crt")
	g.Expect(os.WriteFile(notPEM, []byte("not a certificate"), 0o600)).To(Succeed())
	_, err = loadWebhookCACertPool(notPEM)
	g.Expect(err).To(MatchError(ContainSubstring("no PEM certificates found")))

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	g.Expect(err).NotTo(HaveOccurred())
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "webhook-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	g.Expect(err).NotTo(HaveOccurred())
	validPEM := filepath.Join(dir, "ca.crt")
	g.Expect(os.WriteFile(validPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)).To(Succeed())
	pool, err := loadWebhookCACertPool(validPEM)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(pool).NotTo(BeNil())
}
