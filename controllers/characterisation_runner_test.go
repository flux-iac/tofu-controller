package controllers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	"github.com/fluxcd/pkg/apis/meta"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// runnerPodArtifact has the hello-world file and digest, with the revision form
// runnerPodInstance can parse. The master/<sha> form fails before the Pod create,
// so podCreateHook would never be reached.
var runnerPodArtifact = artifactFixture{helloWorldArtifact.Path, "master@sha1:b8e362c206e3d0cbb7ed22ced771a0056455a2fb", helloWorldArtifact.Digest}

// useRealRunnerPath turns off the in-process runner and installs hook for Pod
// creates. The returned func undoes both; it is also registered in t.Cleanup, so
// calling it early is safe. Call it after newScenario, which asserts the defaults.
func useRealRunnerPath(s *scenario, hook func(obj client.Object) error) (restore func()) {
	s.t.Helper()
	s.t.Setenv("INSECURE_LOCAL_RUNNER", "")
	podCreateHook.Store(&hook)
	restore = func() {
		podCreateHook.Store(nil)
		_ = os.Setenv("INSECURE_LOCAL_RUNNER", "1")
	}
	s.t.Cleanup(restore)
	return restore
}

// Test_000460_runner_create_fails records the scenario below.
// The runner Pod create fails with a plain error, so every reconcile returns it
// and controller-runtime retries with backoff. Ready is False with reason
// RunnerFailed and the error text. The failures are stopped before cleanup,
// so deletion can use the in-process runner.
func Test_000460_runner_create_fails(t *testing.T) {
	s := newScenario(t, "000460_runner_create_fails")
	// Use the default runner image even if the host sets RUNNER_POD_IMAGE.
	t.Setenv("RUNNER_POD_IMAGE", "")
	const tf = "char-000460-tf"
	repo := s.gitRepo("char-000460-source", runnerPodArtifact)
	var (
		mu     sync.Mutex
		podObj *corev1.Pod
	)
	restore := useRealRunnerPath(s, func(obj client.Object) error {
		if pod, ok := obj.(*corev1.Pod); ok {
			mu.Lock()
			if podObj == nil { // every attempt builds the same Pod; keep the first
				podObj = pod.DeepCopy()
			}
			mu.Unlock()
		}
		return errors.New("boom")
	})

	s.step("create")
	s.terraform(tf, hwSourceSpec("GitRepository", repo.Name, "flux-system", ""))
	// Registered after terraform's own cleanup, so it runs first: deletion needs the in-process runner.
	t.Cleanup(restore)
	s.settle("an error result and Ready False", func(p projection) bool {
		return conditionOf(p, meta.ReadyCondition).Status == metav1.ConditionFalse
	})
	s.g.Expect(s.current().Results).NotTo(BeEmpty())

	s.assertGolden()
	restore()

	mu.Lock()
	captured := podObj
	mu.Unlock()
	s.g.Expect(captured).NotTo(BeNil(), "the hook never saw a Pod")
	assertRunnerPodGolden(t, filepath.Join("testdata", "characterisation", "000460_runner_pod.yaml"), captured)
}

// normalisedRunnerPodYAML renders the Pod the controller built, with run-specific
// values removed so that the output is stable:
//   - metadata.creationTimestamp, resourceVersion and uid are dropped (none is
//     set before the create, but the marshalled zero value "creationTimestamp: null" is noise);
//     status is dropped for the same reason;
//   - the owner reference uid is replaced by <uid>, as it is the Terraform object's random uid;
//   - the tls-secret-name label and the --tls-secret-name arg are replaced by
//     <tls-secret> in case the name varies between runs;
//   - HTTP_PROXY, HTTPS_PROXY and NO_PROXY (and lower-case forms) are dropped from
//     env, since the controller copies them from the host environment;
//   - env is sorted by name, since it is built from a Go map.
func normalisedRunnerPodYAML(pod *corev1.Pod) ([]byte, error) {
	p := pod.DeepCopy()
	p.CreationTimestamp = metav1.Time{}
	p.ResourceVersion = ""
	p.UID = ""
	for i := range p.OwnerReferences {
		p.OwnerReferences[i].UID = "<uid>"
	}
	const tlsLabel = "tf.weave.works/tls-secret-name"
	if _, ok := p.Labels[tlsLabel]; ok {
		p.Labels[tlsLabel] = "<tls-secret>"
	}
	drop := map[string]bool{"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true, "http_proxy": true, "https_proxy": true, "no_proxy": true}
	norm := func(cs []corev1.Container) {
		for i := range cs {
			var env []corev1.EnvVar
			for _, e := range cs[i].Env {
				if !drop[e.Name] {
					env = append(env, e)
				}
			}
			sort.SliceStable(env, func(a, b int) bool { return env[a].Name < env[b].Name })
			cs[i].Env = env
			for j, a := range cs[i].Args {
				if a == "--tls-secret-name" && j+1 < len(cs[i].Args) {
					cs[i].Args[j+1] = "<tls-secret>"
				}
				if len(a) > len("--tls-secret-name=") && a[:len("--tls-secret-name=")] == "--tls-secret-name=" {
					cs[i].Args[j] = "--tls-secret-name=<tls-secret>"
				}
			}
		}
	}
	norm(p.Spec.InitContainers)
	norm(p.Spec.Containers)
	p.Status = corev1.PodStatus{}
	b, err := yaml.Marshal(p)
	if err != nil {
		return nil, err
	}
	// Remove the null creationTimestamp the zero value marshals to.
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if md, ok := m["metadata"].(map[string]any); ok {
		delete(md, "creationTimestamp")
	}
	delete(m, "status")
	return yaml.Marshal(m)
}

// assertRunnerPodGolden has the same -update semantics as goldens: a missing file
// fails unless -update; an unchanged Pod is not rewritten.
func assertRunnerPodGolden(t *testing.T, file string, pod *corev1.Pod) {
	t.Helper()
	got, err := normalisedRunnerPodYAML(pod)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(file)
	switch {
	case err == nil && string(want) == string(got):
		if *updateGoldens {
			t.Logf("unchanged %s", file)
		}
	case *updateGoldens:
		if err := os.WriteFile(file, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", file)
	case err != nil:
		t.Fatalf("reading %s: %v (run with -args -update to create it)", file, err)
	default:
		t.Errorf("runner Pod differs from %s\n--- want\n%s\n--- got\n%s", file, want, got)
	}
}

// Test_000461_runner_quota records the scenario below.
// The runner Pod create fails with a quota Forbidden error and QuotaRetryEnabled,
// so the reconcile emits a Warning RunnerQuotaExhausted event and returns
// RequeueAfter (1h, jittered) with a nil error.
func Test_000461_runner_quota(t *testing.T) {
	s := newScenario(t, "000461_runner_quota")
	const tf = "char-000461-tf"
	repo := s.gitRepo("char-000461-source", runnerPodArtifact)
	quotaErr := apierrors.NewForbidden(schema.GroupResource{Group: "", Resource: "pods"}, "test-pod", errors.New("exceeded quota"))
	restore := useRealRunnerPath(s, func(client.Object) error { return quotaErr })
	oldDelay := reconciler.QuotaRetryDelay
	reconciler.QuotaRetryEnabled = true
	reconciler.QuotaRetryDelay = time.Hour
	t.Cleanup(func() {
		reconciler.QuotaRetryEnabled = false
		reconciler.QuotaRetryDelay = oldDelay
	})

	s.step("create")
	s.terraform(tf, hwSourceSpec("GitRepository", repo.Name, "flux-system", ""))
	// Registered after terraform's own cleanup, so it runs first: deletion needs the in-process runner.
	t.Cleanup(restore)
	s.settle("a result and Ready False", func(p projection) bool {
		return conditionOf(p, meta.ReadyCondition).Status == metav1.ConditionFalse
	})
	s.g.Expect(s.current().Events).NotTo(BeEmpty())

	s.assertGolden()
	restore()
}

// Test_000462_break_glass_not_allowed records the scenario below.
// spec.breakTheGlass is true while the controller does not allow it (the default),
// so each reconcile sets Ready Unknown Progressing "Breaking the glass is not
// allowed" and returns an error.
func Test_000462_break_glass_not_allowed(t *testing.T) {
	s := newScenario(t, "000462_break_glass_not_allowed")
	const tf = "char-000462-tf"
	repo := s.gitRepo("char-000462-source", helloWorldArtifact)

	s.step("create")
	s.terraform(tf, hwSourceSpec("GitRepository", repo.Name, "flux-system", "breakTheGlass: true\n"))
	s.settle("Ready Unknown Progressing", func(p projection) bool {
		c := conditionOf(p, meta.ReadyCondition)
		return c.Status == metav1.ConditionUnknown && c.Message == "Breaking the glass is not allowed"
	})

	s.assertGolden()
}

// Test_000463_break_glass_allowed records the scenario below.
// With AllowBreakTheGlass, the break-the-glass annotation makes the reconcile
// write /tmp/.break-glass and block, polling every 10s. The session ends the
// way tfctl ends it: the annotation is removed, then the file. The blocked reconcile
// notices the missing file and returns a "session has ended" error; the next pass
// reaches Ready True again. While the session is active nothing is returned, so
// that segment is only waited out with quiet().
func Test_000463_break_glass_allowed(t *testing.T) {
	s := newScenario(t, "000463_break_glass_allowed")
	const tf = "char-000463-tf"
	repo := s.gitRepo("char-000463-source", helloWorldArtifact)
	t.Cleanup(func() { _ = os.Remove(runner.BreakGlassFile) })
	reconciler.AllowBreakTheGlass = true
	t.Cleanup(func() { reconciler.AllowBreakTheGlass = false })
	readyTrue := func(p projection) bool {
		return conditionOf(p, meta.ReadyCondition).Status == metav1.ConditionTrue
	}

	s.step("apply")
	s.terraform(tf, hwSourceSpec("GitRepository", repo.Name, "flux-system", ""))
	// Registered after terraform's own cleanup, so it runs before the delete: if the
	// test fails mid-session, the blocked worker must be released first. Same order
	// as tfctl: annotation, then file. Both are best effort and idempotent.
	t.Cleanup(func() {
		ctx := context.Background() // t.Context() is already cancelled in cleanups
		obj := &infrav1.Terraform{}
		if err := k8sClient.Get(ctx, s.key, obj); err == nil {
			if _, ok := obj.Annotations[infrav1.BreakTheGlassAnnotation]; ok {
				base := client.MergeFrom(obj.DeepCopy())
				delete(obj.Annotations, infrav1.BreakTheGlassAnnotation)
				_ = k8sClient.Patch(ctx, obj, base)
			}
		}
		if err := os.Remove(runner.BreakGlassFile); err != nil && !os.IsNotExist(err) {
			t.Logf("removing %s: %v", runner.BreakGlassFile, err)
		}
	})
	s.settle("Ready True", readyTrue)

	s.step("session")
	s.patch(func(tf *infrav1.Terraform) {
		if tf.Annotations == nil {
			tf.Annotations = map[string]string{}
		}
		tf.Annotations[infrav1.BreakTheGlassAnnotation] = "2026-01-01T00:00:00Z"
	})
	// An annotation change alone does not trigger a reconcile (generation and
	// reconcile-request predicates), so request one, as tfctl does.
	s.requestReconcile("2026-01-01T00:00:01Z")
	s.g.Eventually(func() bool {
		got := &infrav1.Terraform{}
		if err := k8sClient.Get(t.Context(), s.key, got); err != nil {
			return false
		}
		if _, err := os.Stat(runner.BreakGlassFile); err != nil {
			return false
		}
		return strings.HasPrefix(conditionOf(projectionOf(got), meta.ReadyCondition).Message, "Breaking the glass")
	}, charSettleTimeout, interval).Should(BeTrue())
	// The reconcile is blocked, so there is no result to settle on.
	s.quiet()

	s.step("ended")
	s.patch(func(tf *infrav1.Terraform) { delete(tf.Annotations, infrav1.BreakTheGlassAnnotation) })
	s.g.Expect(os.Remove(runner.BreakGlassFile)).To(Succeed())
	// The blocked reconcile notices the missing file within 10s and returns the
	// "session has ended" error; the retry (20s) then runs a normal pass.
	s.g.Eventually(func() bool {
		// The blocked reconcile returning is the first result of this segment. Its
		// "session has ended" error is mapped to a retry with a nil error.
		return len(s.current().Results) > 0
	}, charSettleTimeout, interval).Should(BeTrue())
	s.settle("Ready True", readyTrue)

	s.assertGolden()
}
