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
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	certName   = "tls.crt"
	keyName    = "tls.key"
	caCertName = "ca.crt"
	caKeyName  = "ca.key"
)

var crLog = logf.Log.WithName("cert-rotation")
var _ manager.Runnable = &CertRotator{}

// PartialManager is a subset of the manager.Manager interface that is used by the CertRotator.
type PartialManager interface {
	GetConfig() *rest.Config
	GetScheme() *runtime.Scheme
	GetRESTMapper() meta.RESTMapper
	Elected() <-chan struct{}
}

// KeyPairArtifacts stores cert artifacts.
type KeyPairArtifacts struct {
	Cert       *x509.Certificate
	Key        *rsa.PrivateKey
	CertPEM    []byte
	KeyPEM     []byte
	validUntil time.Time
}

type artifact struct {
	ca         *KeyPairArtifacts
	certSecret *corev1.Secret
}

type TriggerResult struct {
	Secret *corev1.Secret
	Err    error
}

type Trigger struct {
	Namespace string
	Ready     chan *TriggerResult
}

// CertRotator contains cert artifacts and a channel to close when the certs are ready.
type CertRotator struct {
	writer client.Client
	mgr    manager.Manager

	extKeyUsages *[]x509.ExtKeyUsage
	Ready        chan struct{}

	CAName             string
	CAOrganization     string
	DNSName            string
	CAValidityDuration time.Duration
	// CertValidityDuration   time.Duration
	RotationCheckFrequency time.Duration
	LookaheadInterval      time.Duration

	TriggerCARotation             chan Trigger // trigger the CA rotation
	TriggerNamespaceTLSGeneration chan Trigger // trigger namespace TLS generation

	artifactCaches         []*artifact
	knownNamespaceTLSMap   map[string]*TriggerResult
	knownNamespaceTLSMapMu sync.Mutex
}

// GetKnownNamespaceTLS returns the TriggerResult for the given namespace.
func (cr *CertRotator) GetKnownNamespaceTLS(namespace string) (*TriggerResult, bool) {
	cr.knownNamespaceTLSMapMu.Lock()
	defer cr.knownNamespaceTLSMapMu.Unlock()
	val, ok := cr.knownNamespaceTLSMap[namespace]
	return val, ok
}

// SetKnownNamespaceTLS sets the TriggerResult for the given namespace.
func (cr *CertRotator) SetKnownNamespaceTLS(namespace string, result *TriggerResult) {
	cr.knownNamespaceTLSMapMu.Lock()
	defer cr.knownNamespaceTLSMapMu.Unlock()
	cr.knownNamespaceTLSMap[namespace] = result
}

// GetKnownNamespaces returns all the keys (namespaces) in knownNamespaceTLSMap.
func (cr *CertRotator) GetKnownNamespaces() []string {
	cr.knownNamespaceTLSMapMu.Lock()
	defer cr.knownNamespaceTLSMapMu.Unlock()
	keys := make([]string, 0, len(cr.knownNamespaceTLSMap))
	for k := range cr.knownNamespaceTLSMap {
		keys = append(keys, k)
	}
	return keys
}

// AddRotator adds the CertRotator and ReconcileWH to the manager.
func AddRotator(_ context.Context, mgr manager.Manager, cr *CertRotator) error {
	if mgr == nil || cr == nil {
		return fmt.Errorf("nil arguments")
	}

	cr.writer = mgr.GetClient() // TODO make overrideable
	cr.mgr = mgr
	cr.knownNamespaceTLSMap = make(map[string]*TriggerResult)

	if err := mgr.Add(cr); err != nil {
		return err
	}

	return nil
}

// IsCAValid checks that the CA[n-1] is valid.
func (cr *CertRotator) IsCAValid() (bool, error) {
	n := len(cr.artifactCaches)
	if n == 0 {
		return false, errors.New("no CA in the cache")
	}

	validUntil := cr.artifactCaches[n-1].ca.validUntil
	if validUntil.Before(cr.lookaheadTime()) {
		return true, nil
	}

	return false, nil
}

// Start starts the CertRotator runnable to rotate certs and ensure the certs are ready.
func (cr *CertRotator) Start(ctx context.Context) error {
	// Only the leader do cert rotation.
	// if we're not, we're blocked here and don't do any cert rotation until we become the leader.
	<-cr.mgr.Elected()

	// referenceTime is used to calculate the old cert GC threshold
	var referenceTime = time.Now()

	// lastGCTime is the last time we GCed old certs
	lastGCTime := referenceTime

	// gcInterval is the interval between GCs
	// we GC 6 times per rotation check interval
	var gcInterval time.Duration
	if cr.RotationCheckFrequency <= 1*time.Minute {
		gcInterval = cr.RotationCheckFrequency
	} else if cr.RotationCheckFrequency > 1*time.Minute && cr.RotationCheckFrequency <= 6*time.Minute {
		gcInterval = 1 * time.Minute
	} else {
		gcInterval = cr.RotationCheckFrequency / 6
	}

	// delete old certs in the current namespace first
	runtimeNamespace := os.Getenv("RUNTIME_NAMESPACE")
	if runtimeNamespace != "" {
		err := cr.garbageCollectTLSCertsForcefully(runtimeNamespace, referenceTime)
		if err != nil {
			crLog.Error(err, "failed to garbage collect old certs in the runtime namespace", "namespace", runtimeNamespace)
		}
	}

	cr.extKeyUsages = &[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}

	// explicitly rotate on the first round so that the certificate
	// can be bootstrapped, otherwise mgr exits before a cert can be written
	crLog.Info("starting cert rotator controller")
	defer crLog.Info("stopping cert rotator controller")
	if err := cr.refreshCertsInMemory(); err != nil {
		crLog.Error(err, "could not refresh cert on startup")
		return err
	}

	close(cr.Ready)

	ticker := time.NewTicker(cr.RotationCheckFrequency)
	gcTicker := time.NewTicker(gcInterval)

tickerLoop:
	for {
		select {
		case trigger := <-cr.TriggerCARotation:
			if err := cr.refreshCACertsIfNeeded(); err != nil {
				crLog.Error(err, "could not refresh cert")
			}
			// if no channel passing it, skip
			if trigger.Ready != nil {
				n := len(cr.artifactCaches)
				secret := cr.artifactCaches[n-1].certSecret
				trigger.Ready <- &TriggerResult{Secret: secret, Err: nil}
			}

		// triggerred every RotationCheckFrequency (for example, the default value is 30 minutes in the Helm chart)
		case <-ticker.C:
			if err := cr.refreshCACertsIfNeeded(); err != nil {
				crLog.Error(err, "could not refresh cert")
			}

			// GC: garbage collect the old CA artifacts
			for {
				if len(cr.artifactCaches) == 0 {
					break
				}

				validUntil := cr.artifactCaches[0].ca.validUntil
				// we must NOT use cr.lookaheadTime() here
				if validUntil.Before(time.Now()) {
					cr.artifactCaches = cr.artifactCaches[1:]
					for _, namespace := range cr.GetKnownNamespaces() {
						err := cr.writer.Delete(context.TODO(), &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: namespace,
								Name:      fmt.Sprintf("%s-%d", infrav1.RunnerTLSSecretName, validUntil.Unix()),
							},
						}, client.PropagationPolicy(metav1.DeletePropagationBackground))
						if err != nil {
							crLog.Error(err, "could not delete old CA artifact")
						}
					}
				} else {
					break
				}
			}

		// trigger by Terraform CR reconciliation loop
		case trigger := <-cr.TriggerNamespaceTLSGeneration:
			namespace := trigger.Namespace
			triggerResult, ok := cr.GetKnownNamespaceTLS(namespace)
			if !ok {
				secret, err := cr.generateNamespaceTLS(namespace)
				if err != nil {
					crLog.Error(err, "error generating TLS for ", "namespace", namespace)
				}
				triggerResult = &TriggerResult{Secret: secret, Err: err}
				cr.SetKnownNamespaceTLS(namespace, triggerResult)
			} else {
				crLog.Info("TLS already generated for ", "namespace", namespace)
			}

			// GC: request to collect the old TLS artifacts when we have new TLS generated
			if time.Since(lastGCTime) > gcInterval {
				namespaces := cr.GetKnownNamespaces()
				if len(namespaces) > 0 {
					err := cr.garbageCollectTLSCerts(namespaces, referenceTime)
					if err != nil {
						crLog.Error(err, "error garbage collecting TLS certs")
					}
				}

				// Update the last garbage collection time
				lastGCTime = time.Now()
			}

			trigger.Ready <- triggerResult

		case <-gcTicker.C:
			// GC: request to garbage collect the old TLS artifacts for every gcInterval
			if time.Since(lastGCTime) > gcInterval {
				namespaces := cr.GetKnownNamespaces()
				if len(namespaces) > 0 {
					err := cr.garbageCollectTLSCerts(namespaces, referenceTime)
					if err != nil {
						crLog.Error(err, "error garbage collecting TLS certs")
					}
				}

				// Update the last garbage collection time
				lastGCTime = time.Now()
			}

		case <-ctx.Done():
			break tickerLoop
		}
	}

	ticker.Stop()
	//close(cr.TriggerCARotation)
	//close(cr.TriggerNamespaceTLSGeneration)
	return nil
}

func (cr *CertRotator) garbageCollectTLSCertsForcefully(namespace string, referenceTime time.Time) error {
	crLog.Info("startup gc: scanning old TLS artifacts", "namespace", namespace)

	secretList := &corev1.SecretList{}
	listOpts := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{infrav1.RunnerLabel: "true"}),
	}
	if err := cr.writer.List(context.TODO(), secretList, listOpts); err != nil {
		return err
	}

	crLog.Info("startup gc: found TLS artifacts", "namespace", namespace, "count", len(secretList.Items))
	count := 0
	// Filter Secrets by creation time (before referenceTime)
	for _, secret := range secretList.Items {
		if secret.CreationTimestamp.Time.Before(referenceTime) {
			crLog.Info("startup gc: deleting old TLS artifact ...", "namespace", namespace, "secret", secret.Name)
			if err := cr.writer.Delete(context.TODO(), &secret, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				crLog.Error(err, "startup gc: could not delete old TLS artifact", "namespace", namespace, "secret", secret.Name)
			} else {
				crLog.Info("startup gc: deleted old TLS artifact", "namespace", namespace, "secret", secret.Name)
				count = count + 1
			}
		}
	}

	crLog.Info("startup gc: finished deleting old TLS artifacts", "namespace", namespace, "count", count)
	return nil
}

// garbageCollectTLSCerts deletes old TLS certs that are no longer needed
func (cr *CertRotator) garbageCollectTLSCerts(namespaces []string, referenceTime time.Time) error {
	crLog.Info("gc: scanning old TLS artifacts", "namespaces", namespaces)

	// Collect all Secrets to delete across namespaces
	secretsToDelete := []*corev1.Secret{}

	// deletionThreshold is the maximum number of Secrets to delete per GC run
	const deletionThreshold = 10

	// Iterate through all namespaces
	for _, namespace := range namespaces {
		// List all Secrets in the namespace with the specified label
		secretList := &corev1.SecretList{}
		listOpts := &client.ListOptions{
			Namespace:     namespace,
			LabelSelector: labels.SelectorFromSet(map[string]string{infrav1.RunnerLabel: "true"}),
			Limit:         deletionThreshold + 2, // limit to 12 items per list request (deleteThreshold + the current one + the next one)
		}
		if err := cr.writer.List(context.TODO(), secretList, listOpts); err != nil {
			return err
		}

		// Filter Secrets by creation time (before referenceTime)
		for i := range secretList.Items {
			if secretList.Items[i].CreationTimestamp.Time.Before(referenceTime) {
				secretsToDelete = append(secretsToDelete, &secretList.Items[i])
				if len(secretsToDelete) >= deletionThreshold {
					break
				}
			}
		}
	}

	crLog.Info("gc: found TLS artifacts", "count", len(secretsToDelete))
	if len(secretsToDelete) == 0 {
		return nil
	}

	// Delete the collected Secrets and stop after deleting 10 Secrets
	deletedCounter := 0
	for _, secret := range secretsToDelete {
		if deletedCounter >= deletionThreshold {
			break
		}

		if err := cr.writer.Delete(context.TODO(), secret, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			crLog.Error(err, "gc: could not delete old TLS Secret", "namespace", secret.Namespace, "secret", secret.Name)
		} else {
			crLog.Info("gc: successfully deleted old TLS Secret", "namespace", secret.Namespace, "secret", secret.Name)
			deletedCounter++
		}
	}

	crLog.Info("gc: finished deleting old TLS artifacts", "count", deletedCounter)
	return nil
}

func (cr *CertRotator) refreshCACertsIfNeeded() error {
	needRegeneration := false
	// if there is no CA artifact, refresh certs
	n := len(cr.artifactCaches)
	if n == 0 {
		crLog.Info("no CA in the cache")
		if err := cr.refreshCertsInMemory(); err != nil {
			return err
		} else {
			needRegeneration = true
		}
	} else if n > 0 {
		validUntil := cr.artifactCaches[n-1].ca.validUntil
		if validUntil.Before(cr.lookaheadTime()) {
			if err := cr.refreshCertsInMemory(); err != nil {
				return err
			} else {
				needRegeneration = true
			}
		}
	}

	if needRegeneration {
		// generate new certs for all namespaces
		for namespace := range cr.knownNamespaceTLSMap {
			secret, err := cr.generateNamespaceTLS(namespace)
			if err != nil {
				crLog.Error(err, "could not generate TLS for namespace")
			}
			cr.knownNamespaceTLSMap[namespace] = &TriggerResult{Secret: secret, Err: err}
		}
	}

	return nil
}

// GetRunnerTLSSecretName returns the name of the TLS Secret.
// It is used by the controller to tell the runner the name of TLS.
func (cr *CertRotator) GetRunnerTLSSecretName() (string, error) {
	n := len(cr.artifactCaches)
	if n == 0 {
		return "", errors.New("no CA in the cache")
	}

	caArtifacts := cr.artifactCaches[n-1].ca
	return fmt.Sprintf("%s-%d", infrav1.RunnerTLSSecretName, caArtifacts.validUntil.Unix()), nil
}

func (cr *CertRotator) refreshCertsInMemory() error {
	var caArtifacts *KeyPairArtifacts
	now := time.Now()
	begin := now.Add(-1 * time.Hour)
	end := now.Add(cr.CAValidityDuration)

	var err error
	caArtifacts, err = cr.createCACert(begin, end)
	if err != nil {
		return err
	}

	// create controller-side certificate
	cert, key, err := cr.createCertPEM(caArtifacts, cr.DNSName, begin, end)
	if err != nil {
		return err
	}

	secret := &corev1.Secret{
		Data: map[string][]byte{
			caCertName: caArtifacts.CertPEM,
			caKeyName:  caArtifacts.KeyPEM,
			certName:   cert,
			keyName:    key,
		},
	}

	cr.artifactCaches = append(cr.artifactCaches, &artifact{
		ca:         caArtifacts,
		certSecret: secret,
	})

	return nil
}

func buildArtifactsFromSecret(secret *corev1.Secret) (caArtifacts *KeyPairArtifacts, certArtifacts *KeyPairArtifacts, err error) {
	caArtifacts, err = parseArtifacts(caCertName, caKeyName, secret)
	if err != nil {
		return nil, nil, err
	}

	certArtifacts, err = parseArtifacts(certName, keyName, secret)
	if err != nil {
		return nil, nil, err
	}

	return caArtifacts, certArtifacts, nil
}

func parseArtifacts(certName, keyName string, secret *corev1.Secret) (*KeyPairArtifacts, error) {
	certPem, ok := secret.Data[certName]
	if !ok {
		return nil, errors.New(fmt.Sprintf("Cert Secret is not well-formed, missing %s", caCertName))
	}

	keyPem, ok := secret.Data[keyName]
	if !ok {
		return nil, errors.New(fmt.Sprintf("Cert Secret is not well-formed, missing %s", caKeyName))
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

// createCACert creates the self-signed CA cert and private key that will
// be used to sign the server certificate
func (cr *CertRotator) createCACert(begin, end time.Time) (*KeyPairArtifacts, error) {
	certTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(0),
		Subject: pkix.Name{
			CommonName:   cr.CAName,
			Organization: []string{cr.CAOrganization},
		},
		DNSNames: []string{
			cr.CAName,
		},
		NotBefore:             begin,
		NotAfter:              end,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, errors.Wrap(err, "generating key")
	}
	der, err := x509.CreateCertificate(rand.Reader, certTemplate, certTemplate, key.Public(), key)
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

	return &KeyPairArtifacts{Cert: cert, Key: key, CertPEM: certPEM, KeyPEM: keyPEM, validUntil: end}, nil
}

// createCertPEM takes the results of createCACert and uses it to create the
// PEM-encoded public certificate and private key, respectively.
func (cr *CertRotator) createCertPEM(ca *KeyPairArtifacts, hostname string, begin, end time.Time) ([]byte, []byte, error) {
	dnsNames := []string{hostname}
	if os.Getenv("INSECURE_LOCAL_RUNNER") == "1" {
		dnsNames = append(dnsNames, "localhost")
	}

	certTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: cr.DNSName,
		},
		DNSNames:              dnsNames,
		NotBefore:             begin,
		NotAfter:              end,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, errors.Wrap(err, "generating key")
	}
	der, err := x509.CreateCertificate(rand.Reader, certTemplate, ca.Cert, key.Public(), ca.Key)
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

func (cr *CertRotator) lookaheadTime() time.Time {
	return time.Now().Add(cr.LookaheadInterval)
}

func (cr *CertRotator) validServerCert(caCert, cert, key []byte) bool {
	valid, err := ValidCert(caCert, cert, key, cr.DNSName, cr.extKeyUsages, cr.lookaheadTime())
	if err != nil {
		return false
	}
	return valid
}

func (cr *CertRotator) validCACert(cert, key []byte) bool {
	valid, err := ValidCert(cert, cert, key, cr.CAName, nil, cr.lookaheadTime())
	if err != nil {
		return false
	}
	return valid
}

func (cr *CertRotator) generateNamespaceTLS(namespace string) (*corev1.Secret, error) {
	n := len(cr.artifactCaches)
	// get last artifact cache
	artifactCache := cr.artifactCaches[n-1]
	caArtifacts := artifactCache.ca

	hostname := fmt.Sprintf("*.%s.pod.cluster.local", namespace)
	cert, key, err := cr.createCertPEM(caArtifacts, hostname, time.Now().Add(-1*time.Hour), caArtifacts.validUntil)
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("%s-%d", infrav1.RunnerTLSSecretName, caArtifacts.validUntil.Unix())
	tlsCertSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels: map[string]string{
				infrav1.RunnerLabel: "true",
			},
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			caCertName: caArtifacts.CertPEM,
			caKeyName:  caArtifacts.KeyPEM,
			certName:   cert,
			keyName:    key,
		},
	}

	if err := cr.writer.Create(context.TODO(), tlsCertSecret); err != nil {
		return nil, err
	}

	return tlsCertSecret, nil
}

func (cr *CertRotator) ResetCACache() {
	cr.artifactCaches = []*artifact{}
}

func ValidCert(caCert, cert, key []byte, dnsName string, keyUsages *[]x509.ExtKeyUsage, at time.Time) (bool, error) {
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

	opt := x509.VerifyOptions{
		DNSName:     dnsName,
		Roots:       pool,
		CurrentTime: at,
	}
	if keyUsages != nil {
		opt.KeyUsages = *keyUsages
	}

	_, err = crt.Verify(opt)
	if err != nil {
		return false, errors.Wrap(err, "verifying cert")
	}
	return true, nil
}
