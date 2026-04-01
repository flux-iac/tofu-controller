package plan

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
)

func TestToSecretSingleChunk(t *testing.T) {
	planData := []byte("example plan data")
	plan, err := NewFromBytes("tf", "ns", "ws", "uid", "plan-id", planData)
	if err != nil {
		t.Fatalf("unexpected error creating plan: %v", err)
	}

	secrets, err := plan.ToSecret("-suffix")
	if err != nil {
		t.Fatalf("unexpected error converting to secret: %v", err)
	}

	if len(secrets) != 1 {
		t.Fatalf("expected 1 secret, got %d", len(secrets))
	}

	secret := secrets[0]
	if secret.Name != "tfplan-ws-tf-suffix" {
		t.Fatalf("secret name mismatch, got %s", secret.Name)
	}
	if secret.Namespace != "ns" {
		t.Fatalf("secret namespace mismatch, got %s", secret.Namespace)
	}
	if got := secret.Annotations["encoding"]; got != "gzip" {
		t.Fatalf("unexpected encoding annotation: %s", got)
	}
	if got := secret.Annotations[TFPlanSavedAnnotation]; got != "plan-id" {
		t.Fatalf("unexpected saved plan annotation: %s", got)
	}

	expectedHash := fmt.Sprintf("%x", sha256.Sum256(plan.bytes))
	if got := secret.Annotations[TFPlanHashAnnotation]; got != expectedHash {
		t.Fatalf("unexpected plan hash annotation: %s", got)
	}

	if len(secret.OwnerReferences) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(secret.OwnerReferences))
	}
	if secret.OwnerReferences[0].Kind != infrav1.TerraformKind {
		t.Fatalf("unexpected owner reference kind: %s", secret.OwnerReferences[0].Kind)
	}

	decoded, err := GzipDecode(secret.Data[TFPlanName])
	if err != nil {
		t.Fatalf("unable to decode plan data: %v", err)
	}
	if string(decoded) != string(planData) {
		t.Fatalf("decoded plan mismatch, got %q", string(decoded))
	}
}

func TestToSecretChunked(t *testing.T) {
	// Generate incompressible random data larger than resourceDataMaxSizeBytes (1MB)
	// so that even after gzip encoding, the result exceeds the chunk limit.
	rng := rand.New(rand.NewSource(42))
	planData := make([]byte, 2*resourceDataMaxSizeBytes)
	rng.Read(planData)

	plan, err := NewFromBytes("tf", "ns", "ws", "uid", "plan-id", planData)
	if err != nil {
		t.Fatalf("unexpected error creating plan: %v", err)
	}

	secrets, err := plan.ToSecret("")
	if err != nil {
		t.Fatalf("unexpected error converting to secret: %v", err)
	}

	if len(secrets) < 2 {
		t.Fatalf("expected multiple secrets for chunked plan, got %d", len(secrets))
	}

	for i, secret := range secrets {
		expectedName := fmt.Sprintf("tfplan-ws-tf-%d", i)
		if secret.Name != expectedName {
			t.Fatalf("unexpected secret name, got %s want %s", secret.Name, expectedName)
		}
		if got := secret.Annotations[TFPlanChunkAnnotation]; got != fmt.Sprintf("%d", i) {
			t.Fatalf("unexpected chunk annotation on secret %d: %s", i, got)
		}
		if len(secret.Data[TFPlanName]) > resourceDataMaxSizeBytes {
			t.Fatalf("secret chunk %d exceeds max size", i)
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(secret.Data[TFPlanName]))
		if secret.Annotations[TFPlanHashAnnotation] != hash {
			t.Fatalf("unexpected plan hash for chunk %d", i)
		}
	}
}

func TestToConfigMapSingleChunk(t *testing.T) {
	planData := []byte("human readable plan output")
	plan, err := NewFromBytes("tf", "ns", "ws", "uid", "plan-id", planData)
	if err != nil {
		t.Fatalf("unexpected error creating plan: %v", err)
	}

	configMaps, err := plan.ToConfigMap("-human")
	if err != nil {
		t.Fatalf("unexpected error converting to configmap: %v", err)
	}

	if len(configMaps) != 1 {
		t.Fatalf("expected 1 configmap, got %d", len(configMaps))
	}

	cm := configMaps[0]
	if cm.Name != "tfplan-ws-tf-human" {
		t.Fatalf("configmap name mismatch, got %s", cm.Name)
	}
	if got := cm.Annotations[TFPlanSavedAnnotation]; got != "plan-id" {
		t.Fatalf("unexpected saved plan annotation: %s", got)
	}

	// ConfigMaps store raw plan data (not gzip-encoded)
	if cm.Data[TFPlanName] != string(planData) {
		t.Fatalf("configmap plan data mismatch")
	}
}

func TestToConfigMapChunked(t *testing.T) {
	// Generate data larger than resourceDataMaxSizeBytes (1MB) to force chunking
	planData := make([]byte, resourceDataMaxSizeBytes+1024)
	for i := range planData {
		planData[i] = byte(i % 256)
	}

	plan, err := NewFromBytes("tf", "ns", "ws", "uid", "plan-id", planData)
	if err != nil {
		t.Fatalf("unexpected error creating plan: %v", err)
	}

	configMaps, err := plan.ToConfigMap("")
	if err != nil {
		t.Fatalf("unexpected error converting to configmaps: %v", err)
	}

	if len(configMaps) < 2 {
		t.Fatalf("expected multiple configmaps for chunked plan, got %d", len(configMaps))
	}

	for i, cm := range configMaps {
		expectedName := fmt.Sprintf("tfplan-ws-tf-%d", i)
		if cm.Name != expectedName {
			t.Fatalf("unexpected configmap name, got %s want %s", cm.Name, expectedName)
		}
		if got := cm.Annotations[TFPlanChunkAnnotation]; got != fmt.Sprintf("%d", i) {
			t.Fatalf("unexpected chunk annotation on configmap %d: %s", i, got)
		}
		if len(cm.Data[TFPlanName]) > resourceDataMaxSizeBytes {
			t.Fatalf("configmap chunk %d exceeds max size", i)
		}
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(cm.Data[TFPlanName])))
		if cm.Annotations[TFPlanHashAnnotation] != hash {
			t.Fatalf("unexpected plan hash for configmap chunk %d", i)
		}
	}
}
