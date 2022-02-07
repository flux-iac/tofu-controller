package mtls

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// process should be:
// ca rotates on controller startup --if flag is specified
// creates the ca and controller cert
// then finds all runner certs and rotates them
// ca refreshes on a long interval once every 7 days
// server cert and runner certs refresh on a configurable interval
// runner certs rotate when the runner starts or after configurable interval 30 minutes

//TODO:
// 1: label tls secrets on creation that will allow updating ca values whenever the ca cert changes
// 2: add reconciler for tls labelled secrets
// 3: separate cert validate durations

const (
	certName               = "tls.crt"
	keyName                = "tls.key"
	caCertName             = "ca.crt"
	caKeyName              = "ca.key"
	certValidityDuration   = 6 * time.Hour
	rotationCheckFrequency = 30 * time.Minute
	lookaheadInterval      = 1 * time.Hour
)

var crLog = logf.Log.WithName("cert-rotation")

var _ manager.Runnable = &CertRotator{}

// KeyPairArtifacts stores cert artifacts.
type KeyPairArtifacts struct {
	Cert    *x509.Certificate
	Key     *rsa.PrivateKey
	CertPEM []byte
	KeyPEM  []byte
}

// CertRotator contains cert artifacts and a channel to close when the certs are ready.
type CertRotator struct {
	client         client.Client
	SecretKey      types.NamespacedName
	CAName         string
	CAOrganization string
	DNSName        string
	Ready          chan struct{}
}

// AddRotator adds the CertRotator to the manager
func AddRotator(ctx context.Context, mgr manager.Manager, cr *CertRotator) error {
	cr.client = mgr.GetClient()
	if err := mgr.Add(cr); err != nil {
		return err
	}

	return nil
}

// Start starts the CertRotator runnable to rotate certs and ensure the certs are ready.
func (cr *CertRotator) Start(ctx context.Context) error {
	crLog.Info("starting cert rotator controller")
	defer crLog.Info("stopping cert rotator controller")

	// explicitly rotate on the first round so that the certificate
	// can be bootstrapped, otherwise manager exits before a cert can be written
	if err := cr.refreshCertIfNeeded(); err != nil {
		crLog.Error(err, "could not refresh cert on startup")
		return err
	}

	close(cr.Ready)

	ticker := time.NewTicker(rotationCheckFrequency)

tickerLoop:
	for {
		select {
		case <-ticker.C:
			if err := cr.refreshCertIfNeeded(); err != nil {
				crLog.Error(err, "error rotating certs")
			}
		case <-ctx.Done():
			break tickerLoop
		}
	}

	ticker.Stop()
	return nil
}

// refreshCertIfNeeded returns whether there's any error when refreshing the certs if needed.
func (cr *CertRotator) refreshCertIfNeeded() error {
	refreshFn := func() (bool, error) {
		ctx := context.Background()

		secret := &corev1.Secret{}
		if err := cr.client.Get(ctx, cr.SecretKey, secret); err != nil {
			if !apierrors.IsNotFound(err) {
				return false, errors.Wrap(err, "acquiring secret to update certificates")
			}
			secret.ObjectMeta.Namespace = cr.SecretKey.Namespace
			secret.ObjectMeta.Name = cr.SecretKey.Name
			if err := cr.client.Create(ctx, secret); err != nil {
				return false, errors.Wrap(err, "creating secret to update certificates")
			}
		}

		if secret.Data == nil || !cr.validCACert(secret.Data[caCertName], secret.Data[caKeyName]) {
			crLog.Info("refreshing CA and server certs")
			if err := cr.refreshCerts(true, secret); err != nil {
				crLog.Error(err, "could not refresh CA and server certs")
				return false, nil
			}
			crLog.Info("server certs refreshed")
			return true, nil
		}

		// make sure our reconciler is initialized on startup (either this or the above refreshCerts() will call this)
		if !cr.validServerCert(secret.Data[caCertName], secret.Data[certName], secret.Data[keyName]) {
			crLog.Info("refreshing server certs")
			if err := cr.refreshCerts(false, secret); err != nil {
				crLog.Error(err, "could not refresh server certs")
				return false, nil
			}

			crLog.Info("server certs refreshed")

			return true, nil
		}

		crLog.Info("no cert refresh needed")

		return true, nil
	}

	if err := wait.ExponentialBackoff(wait.Backoff{
		Duration: 10 * time.Millisecond,
		Factor:   2,
		Jitter:   1,
		Steps:    10,
	}, refreshFn); err != nil {
		return err
	}

	return nil
}

func (cr *CertRotator) refreshCerts(refreshCA bool, secret *corev1.Secret) error {
	var caArtifacts *KeyPairArtifacts
	now := time.Now()
	begin := now.Add(-1 * time.Hour)
	end := now.Add(certValidityDuration)

	if refreshCA {
		var err error
		caArtifacts, err = cr.createCACert(begin, end)
		if err != nil {
			return err
		}
	} else {
		var err error
		caArtifacts, err = parseArtifacts(caCertName, caKeyName, secret)
		if err != nil {
			return err
		}
	}

	cert, key, err := cr.createCertPEM(caArtifacts, cr.DNSName, begin, end)
	if err != nil {
		return err
	}

	if err := cr.writeSecret(cert, key, caArtifacts, secret); err != nil {
		return err
	}

	return nil
}

func (cr *CertRotator) RefreshRunnerCertIfNeeded(hostname string, secret *corev1.Secret) error {
	var caArtifacts *KeyPairArtifacts
	now := time.Now()
	begin := now.Add(-1 * time.Hour)
	end := now.Add(certValidityDuration)

	caSecret := &corev1.Secret{}
	if err := cr.client.Get(context.Background(), cr.SecretKey, caSecret); err != nil {
		return err
	}

	caArtifacts, err := parseArtifacts(caCertName, caKeyName, caSecret)
	if err != nil {
		return err
	}

	cert, key, err := cr.createCertPEM(caArtifacts, hostname, begin, end)
	if err != nil {
		return err
	}

	if err := cr.writeSecret(cert, key, caArtifacts, secret); err != nil {
		return err
	}

	return nil
}

func (cr *CertRotator) writeSecret(cert, key []byte, caArtifacts *KeyPairArtifacts, secret *corev1.Secret) error {
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}

	secret.Data[caCertName] = caArtifacts.CertPEM
	secret.Data[caKeyName] = caArtifacts.KeyPEM
	secret.Data[certName] = cert
	secret.Data[keyName] = key

	return cr.client.Update(context.Background(), secret)
}

func buildArtifactsFromSecret(secret *corev1.Secret) (*KeyPairArtifacts, *KeyPairArtifacts, error) {
	caArtifacts, err := parseArtifacts(caCertName, caKeyName, secret)
	if err != nil {
		return nil, nil, err
	}

	certArtifacts, err := parseArtifacts(certName, keyName, secret)
	if err != nil {
		return nil, nil, err
	}

	return caArtifacts, certArtifacts, nil
}

func parseArtifacts(certName, keyName string, secret *corev1.Secret) (*KeyPairArtifacts, error) {
	certPem, ok := secret.Data[certName]
	if !ok {
		return nil, errors.New(fmt.Sprintf("Cert secret is not well-formed, missing %s", caCertName))
	}

	keyPem, ok := secret.Data[keyName]
	if !ok {
		return nil, errors.New(fmt.Sprintf("Cert secret is not well-formed, missing %s", caKeyName))
	}

	certDer, _ := pem.Decode(certPem)
	if certDer == nil {
		return nil, errors.New("bad cert")
	}

	cert, err := x509.ParseCertificate(certDer.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "while parsing CA cert")
	}

	keyDer, _ := pem.Decode(keyPem)
	if keyDer == nil {
		return nil, errors.New("bad cert")
	}

	key, err := x509.ParsePKCS1PrivateKey(keyDer.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "while parsing key")
	}

	return &KeyPairArtifacts{
		Cert:    cert,
		CertPEM: certPem,
		KeyPEM:  keyPem,
		Key:     key,
	}, nil

}

// CreateCACert creates the self-signed CA cert and private key that will
// be used to sign the server certificate
func (cr *CertRotator) createCACert(begin, end time.Time) (*KeyPairArtifacts, error) {
	templ := &x509.Certificate{
		SerialNumber: big.NewInt(0),
		Subject: pkix.Name{
			CommonName:   cr.CAName,
			Organization: []string{cr.CAOrganization},
		},
		DNSNames: []string{
			cr.DNSName,
		},
		NotBefore:             begin,
		NotAfter:              end,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, errors.Wrap(err, "generating key")
	}

	der, err := x509.CreateCertificate(rand.Reader, templ, templ, key.Public(), key)
	if err != nil {
		return nil, errors.Wrap(err, "creating certificate")
	}

	certPEM, keyPEM, err := pemEncode(der, key)
	if err != nil {
		return nil, errors.Wrap(err, "encoding PEM")
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, errors.Wrap(err, "parsing certificate")
	}

	return &KeyPairArtifacts{Cert: cert, Key: key, CertPEM: certPEM, KeyPEM: keyPEM}, nil
}

// CreateCertPEM takes the results of CreateCACert and uses it to create the
// PEM-encoded public certificate and private key, respectively
func (cr *CertRotator) createCertPEM(ca *KeyPairArtifacts, hostname string, begin, end time.Time) ([]byte, []byte, error) {
	templ := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: cr.CAName,
		},
		DNSNames: []string{
			hostname,
		},
		NotBefore:             begin,
		NotAfter:              end,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, errors.Wrap(err, "generating key")
	}

	der, err := x509.CreateCertificate(rand.Reader, templ, ca.Cert, key.Public(), ca.Key)
	if err != nil {
		return nil, nil, errors.Wrap(err, "creating certificate")
	}

	certPEM, keyPEM, err := pemEncode(der, key)
	if err != nil {
		return nil, nil, errors.Wrap(err, "encoding PEM")
	}

	return certPEM, keyPEM, nil
}

// pemEncode takes a certificate and encodes it as PEM
func pemEncode(certificateDER []byte, key *rsa.PrivateKey) ([]byte, []byte, error) {
	certBuf := &bytes.Buffer{}
	if err := pem.Encode(certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: certificateDER}); err != nil {
		return nil, nil, errors.Wrap(err, "encoding cert")
	}

	keyBuf := &bytes.Buffer{}
	if err := pem.Encode(keyBuf, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}); err != nil {
		return nil, nil, errors.Wrap(err, "encoding key")
	}

	return certBuf.Bytes(), keyBuf.Bytes(), nil
}

// ValidCert verifies if the cert is valid for the given hostname and time
func ValidCert(caCert, cert, key []byte, dnsName string, at time.Time) (bool, error) {
	if len(caCert) == 0 || len(cert) == 0 || len(key) == 0 {
		return false, errors.New("empty cert")
	}

	pool := x509.NewCertPool()
	caDer, _ := pem.Decode(caCert)
	if caDer == nil {
		return false, errors.New("bad CA cert")
	}

	cac, err := x509.ParseCertificate(caDer.Bytes)
	if err != nil {
		return false, errors.Wrap(err, "parsing CA cert")
	}
	pool.AddCert(cac)

	_, err = tls.X509KeyPair(cert, key)
	if err != nil {
		return false, errors.Wrap(err, "building key pair")
	}

	b, _ := pem.Decode(cert)
	if b == nil {
		return false, errors.New("bad private key")
	}

	crt, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		return false, errors.Wrap(err, "parsing cert")
	}

	_, err = crt.Verify(x509.VerifyOptions{
		DNSName:     dnsName,
		Roots:       pool,
		CurrentTime: at,
	})
	if err != nil {
		return false, errors.Wrap(err, "verifying cert")
	}

	return true, nil
}

func (cr *CertRotator) validCACert(cert, key []byte) bool {
	valid, err := ValidCert(cert, cert, key, cr.CAName, lookaheadTime())
	if err != nil {
		return false
	}
	return valid
}

func (cr *CertRotator) validServerCert(caCert, cert, key []byte) bool {
	valid, err := ValidCert(caCert, cert, key, cr.DNSName, lookaheadTime())
	if err != nil {
		return false
	}
	return valid
}

func lookaheadTime() time.Time {
	return time.Now().Add(lookaheadInterval)
}
