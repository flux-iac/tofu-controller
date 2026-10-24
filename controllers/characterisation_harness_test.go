package controllers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	kuberecorder "k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	charNamespace     = "flux-system"
	charSettleTimeout = 3 * time.Minute
	charQuietPeriod   = 3 * time.Second
)

// artifactFixture is a source artifact served by the test HTTP server.
type artifactFixture struct{ Path, Revision, Digest string } // Path is appended to server.URL()

var helloWorldArtifact = artifactFixture{"/file.tar.gz", "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb", "sha256:80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406"}

// podCreateHook, when set, is called by the reconciler's client for every Pod
// Create. A non-nil error is returned in place of the Create; nil lets it through.
var podCreateHook atomic.Pointer[func(obj client.Object) error]

// charRecorder holds the recordings of the shared manager, keyed by Terraform object.
var charRecorder = &recorder{active: map[types.NamespacedName]*golden{}}

// recorder keeps one recording per active key. Writes for other keys are dropped.
type recorder struct {
	mu     sync.Mutex
	active map[types.NamespacedName]*golden
	// lastTimes holds the raw time fields of the last object observed per key,
	// across segments, for TimesAdvanced. nil until the first observation.
	lastTimes map[types.NamespacedName]map[string]string
}

func (r *recorder) start(key types.NamespacedName, firstSegment string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active[key] = &golden{Segments: []segment{{Name: firstSegment}}}
	if r.lastTimes == nil {
		r.lastTimes = map[types.NamespacedName]map[string]string{}
	}
	delete(r.lastTimes, key)
}

// stop ends the recording for key and returns it.
func (r *recorder) stop(key types.NamespacedName) golden {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := r.active[key]
	delete(r.active, key)
	delete(r.lastTimes, key)
	if rec == nil {
		return golden{}
	}
	return *rec
}

func (r *recorder) nextSegment(key types.NamespacedName, name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := r.active[key]
	if rec == nil {
		return false
	}
	rec.Segments = append(rec.Segments, segment{Name: name})
	return true
}

// with runs f on the current segment of key, if key is being recorded.
func (r *recorder) with(key types.NamespacedName, f func(cur *segment)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec := r.active[key]; rec != nil {
		f(&rec.Segments[len(rec.Segments)-1])
	}
}

func (r *recorder) addProjection(key types.NamespacedName, p projection) {
	r.with(key, func(cur *segment) {
		if n := len(cur.Timeline); n > 0 && apiequality.Semantic.DeepEqual(cur.Timeline[n-1], p) {
			return
		}
		cur.Timeline = append(cur.Timeline, p)
	})
}

// observe records an object seen by the informer: its projection, and which of
// its time fields changed value since the previous object seen for key.
func (r *recorder) observe(key types.NamespacedName, p projection, times map[string]string) {
	r.addProjection(key, p)
	r.with(key, func(cur *segment) {
		prev, seen := r.lastTimes[key]
		r.lastTimes[key] = times
		if !seen {
			return
		}
		for name := range unionKeys(prev, times) {
			if prev[name] != times[name] && !slices.Contains(cur.TimesAdvanced, name) {
				cur.TimesAdvanced = append(cur.TimesAdvanced, name)
			}
		}
		sort.Strings(cur.TimesAdvanced)
	})
}

func unionKeys(a, b map[string]string) map[string]bool {
	out := map[string]bool{}
	for k := range a {
		out[k] = true
	}
	for k := range b {
		out[k] = true
	}
	return out
}

// addEvent appends e, collapsing consecutive identical events per segment.
func (r *recorder) addEvent(key types.NamespacedName, e recordedEvent) {
	r.with(key, func(cur *segment) {
		if n := len(cur.Events); n > 0 && cur.Events[n-1] == e {
			return
		}
		cur.Events = append(cur.Events, e)
	})
}

func (r *recorder) addResult(key types.NamespacedName, res recordedResult) {
	r.with(key, func(cur *segment) {
		if n := len(cur.Results); n > 0 && cur.Results[n-1] == res {
			return
		}
		cur.Results = append(cur.Results, res)
	})
}

// keys returns the keys being recorded, sorted.
func (r *recorder) keys() []types.NamespacedName {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]types.NamespacedName, 0, len(r.active))
	for k := range r.active {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// snapshot returns a deep copy of the recording for key.
func (r *recorder) snapshot(key types.NamespacedName) golden {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := r.active[key]
	if rec == nil {
		return golden{}
	}
	out := golden{Segments: make([]segment, len(rec.Segments))}
	for i, s := range rec.Segments {
		out.Segments[i] = segment{
			Name:          s.Name,
			Timeline:      append([]projection(nil), s.Timeline...),
			Events:        append([]recordedEvent(nil), s.Events...),
			Results:       append([]recordedResult(nil), s.Results...),
			TimesAdvanced: append([]string(nil), s.TimesAdvanced...),
		}
	}
	return out
}

// recordingEventRecorder records every event, then forwards it to the original recorder.
type recordingEventRecorder struct {
	kuberecorder.EventRecorder
}

func (r recordingEventRecorder) Eventf(regarding, related runtime.Object, eventtype, reason, action, note string, args ...interface{}) {
	if acc, err := apimeta.Accessor(regarding); err == nil {
		charRecorder.addEvent(
			types.NamespacedName{Namespace: acc.GetNamespace(), Name: acc.GetName()},
			recordedEvent{Type: eventtype, Reason: reason, Action: action, Message: normalise(fmt.Sprintf(note, args...))},
		)
	}
	r.EventRecorder.Eventf(regarding, related, eventtype, reason, action, note, args...)
}

// recordTimeline appends the projection of a Terraform seen by the informer.
func recordTimeline(obj interface{}) {
	if tf, ok := obj.(*infrav1.Terraform); ok {
		charRecorder.observe(client.ObjectKeyFromObject(tf), projectionOf(tf), timesOf(tf))
	}
}

// recordResult is the reconciler's observeReconcile hook.
func recordResult(req ctrl.Request, obj *infrav1.Terraform, res ctrl.Result, err error, panicked bool) {
	charRecorder.addResult(req.NamespacedName, resultOf(obj, res, err, panicked))
}

// interceptPodCreate routes Pod creates through podCreateHook when it is set.
func interceptPodCreate(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
	if hook := podCreateHook.Load(); hook != nil {
		if _, ok := obj.(*corev1.Pod); ok {
			if err := (*hook)(obj); err != nil {
				return err
			}
		}
	}
	return c.Create(ctx, obj, opts...)
}

// withoutWatch adapts the manager's client to client.WithWatch for the interceptor.
// The reconciler never watches through its client.
type withoutWatch struct{ client.Client }

func (withoutWatch) Watch(context.Context, client.ObjectList, ...client.ListOption) (watch.Interface, error) {
	return nil, errors.New("watch is not supported by the reconciler's test client")
}

// scenario drives one Terraform object through named steps and records it.
type scenario struct {
	t            *testing.T
	g            *WithT
	stem         string
	key          types.NamespacedName
	firstSegment string
}

// newScenario fails fast when a shared-reconciler toggle leaked from an earlier
// test. stem names the golden; it may be empty for tests without one.
func newScenario(t *testing.T, stem string) *scenario {
	t.Helper()
	switch {
	case reconciler.AllowBreakTheGlass:
		t.Fatal("shared reconciler: AllowBreakTheGlass leaked from an earlier test")
	case reconciler.QuotaRetryEnabled:
		t.Fatal("shared reconciler: QuotaRetryEnabled leaked from an earlier test")
	case reconciler.NoCrossNamespaceRefs:
		t.Fatal("shared reconciler: NoCrossNamespaceRefs leaked from an earlier test")
	case podCreateHook.Load() != nil:
		t.Fatal("podCreateHook leaked from an earlier test")
	case os.Getenv("INSECURE_LOCAL_RUNNER") != "1":
		t.Fatal("INSECURE_LOCAL_RUNNER must be 1")
	case len(charRecorder.keys()) != 0:
		t.Fatalf("recordings leaked from an earlier test: %v", charRecorder.keys())
	}
	s := &scenario{t: t, g: NewWithT(t), stem: stem}
	t.Cleanup(func() {
		if s.key.Name != "" {
			charRecorder.stop(s.key)
		}
	})
	return s
}

// gitRepo creates a GitRepository whose status carries artifact a; cleanup deletes it.
func (s *scenario) gitRepo(name string, a artifactFixture) *sourcev1.GitRepository {
	s.t.Helper()
	repo := &sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: charNamespace},
		Spec: sourcev1.GitRepositorySpec{
			URL:       "https://github.com/openshift-fluxv2-poc/podinfo",
			Reference: &sourcev1.GitRepositoryRef{Branch: "master"},
			Interval:  metav1.Duration{Duration: 30 * time.Second},
		},
	}
	s.g.Expect(k8sClient.Create(s.t.Context(), repo)).To(Succeed())
	s.t.Cleanup(func() { waitResourceToBeDelete(NewWithT(s.t), repo) })
	s.setArtifact(repo, a)
	return repo
}

// setArtifact sets the repository's artifact; a new revision is a new source revision.
func (s *scenario) setArtifact(repo *sourcev1.GitRepository, a artifactFixture) {
	s.t.Helper()
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := k8sClient.Get(s.t.Context(), client.ObjectKeyFromObject(repo), repo); err != nil {
			return err
		}
		now := metav1.Now()
		repo.Status = sourcev1.GitRepositoryStatus{
			ObservedGeneration: repo.Generation,
			Conditions: []metav1.Condition{{
				Type:               meta.ReadyCondition,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "GitOperationSucceed",
				Message:            "Fetched revision: " + a.Revision,
			}},
			Artifact: &meta.Artifact{
				Path:           fmt.Sprintf("gitrepository/%s/%s/%s", repo.Namespace, repo.Name, path.Base(a.Path)),
				URL:            server.URL() + a.Path,
				Revision:       a.Revision,
				Digest:         a.Digest,
				LastUpdateTime: now,
			},
		}
		return k8sClient.Status().Update(s.t.Context(), repo)
	})
	s.g.Expect(err).To(Succeed())
}

// charTerraformObject builds a Terraform in flux-system from the YAML of its spec.
func charTerraformObject(t *testing.T, name, specYAML string) *infrav1.Terraform {
	t.Helper()
	var spec infrav1.TerraformSpec
	if err := yaml.UnmarshalStrict([]byte(specYAML), &spec); err != nil {
		t.Fatalf("parsing spec of %s: %v", name, err)
	}
	return &infrav1.Terraform{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: charNamespace}, Spec: spec}
}

// terraform starts recording for the object, then creates it. Call step first to name the first segment.
func (s *scenario) terraform(name string, specYAML string) *infrav1.Terraform {
	s.t.Helper()
	if s.firstSegment == "" {
		s.t.Fatal("call step before terraform to name the first segment")
	}
	if s.key.Name != "" {
		s.t.Fatalf("scenario already records %s", s.key)
	}
	obj := charTerraformObject(s.t, name, specYAML)
	s.key = client.ObjectKeyFromObject(obj)
	charRecorder.start(s.key, s.firstSegment)
	s.g.Expect(k8sClient.Create(s.t.Context(), obj)).To(Succeed())
	s.t.Cleanup(func() { waitResourceToBeDelete(NewWithT(s.t), obj) })
	return obj
}

// step closes the current segment and opens a new one named name.
func (s *scenario) step(name string) {
	s.t.Helper()
	By("step " + name)
	if s.key.Name == "" {
		s.firstSegment = name
		return
	}
	if !charRecorder.nextSegment(s.key, name) {
		s.t.Fatalf("step %q: %s is not being recorded", name, s.key)
	}
}

func (s *scenario) get() *infrav1.Terraform {
	s.t.Helper()
	obj := &infrav1.Terraform{}
	s.g.Expect(k8sClient.Get(s.t.Context(), s.key, obj)).To(Succeed())
	return obj
}

// patch merge-patches the object's metadata and spec.
func (s *scenario) patch(mutate func(tf *infrav1.Terraform)) {
	s.t.Helper()
	obj := s.get()
	base := client.MergeFrom(obj.DeepCopy())
	mutate(obj)
	s.g.Expect(k8sClient.Patch(s.t.Context(), obj, base)).To(Succeed())
}

// patchStatus merge-patches the status as field manager "tf-controller", the way tfctl replan does.
func (s *scenario) patchStatus(mutate func(tf *infrav1.Terraform)) {
	s.t.Helper()
	obj := s.get()
	base := client.MergeFrom(obj.DeepCopy())
	mutate(obj)
	s.g.Expect(k8sClient.Status().Patch(s.t.Context(), obj, base, client.FieldOwner("tf-controller"))).To(Succeed())
}

func (s *scenario) delete() {
	s.t.Helper()
	s.g.Expect(k8sClient.Delete(s.t.Context(), s.get())).To(Succeed())
}

// current returns a copy of the segment being recorded.
func (s *scenario) current() segment {
	rec := charRecorder.snapshot(s.key)
	if len(rec.Segments) == 0 {
		return segment{}
	}
	return rec.Segments[len(rec.Segments)-1]
}

// settle waits until the current segment has a reconcile result and pred holds
// for its latest projection, then for the recording to go quiet. Earlier segments
// are ignored, so their stale state cannot end this segment early.
func (s *scenario) settle(desc string, pred func(p projection) bool) {
	s.t.Helper()
	By("settling on " + desc)
	s.g.Eventually(func() bool {
		cur := s.current()
		if len(cur.Results) == 0 || len(cur.Timeline) == 0 {
			return false
		}
		return pred(cur.Timeline[len(cur.Timeline)-1])
	}, charSettleTimeout, interval).Should(BeTrue(), "settle: %s", desc)
	s.quiet()
}

// settleGone waits until the object is gone, then for the recording to go quiet.
func (s *scenario) settleGone() {
	s.t.Helper()
	By("settling on deletion")
	s.g.Eventually(func() bool {
		return apierrors.IsNotFound(k8sClient.Get(s.t.Context(), s.key, &infrav1.Terraform{}))
	}, charSettleTimeout, interval).Should(BeTrue(), "settle: object gone")
	s.quiet()
}

// quiet waits until nothing new is recorded for charQuietPeriod.
func (s *scenario) quiet() {
	s.t.Helper()
	size := func() int {
		n := 0
		for _, seg := range charRecorder.snapshot(s.key).Segments {
			n += len(seg.Timeline) + len(seg.Events) + len(seg.Results)
		}
		return n
	}
	deadline := time.Now().Add(charSettleTimeout)
	last, since := size(), time.Now()
	for time.Since(since) < charQuietPeriod {
		if time.Now().After(deadline) {
			s.t.Fatalf("recording for %s did not go quiet within %s", s.key, charSettleTimeout)
		}
		time.Sleep(100 * time.Millisecond)
		if n := size(); n != last {
			last, since = n, time.Now()
		}
	}
}

// assertGolden stops the recording and compares it with the golden. With -update
// it rewrites the golden only when compareGolden finds a real change, or writes
// it when missing.
func (s *scenario) assertGolden() {
	s.t.Helper()
	if s.stem == "" {
		s.t.Fatal("assertGolden: scenario has no golden stem")
	}
	got := charRecorder.stop(s.key)
	file := filepath.Join("testdata", "characterisation", s.stem+".yaml")
	if *updateGoldens {
		wrote, diffs, err := updateGolden(file, got)
		if err != nil {
			s.t.Fatal(err)
		}
		for _, d := range diffs {
			s.t.Log(d)
		}
		if wrote {
			s.t.Logf("wrote %s", file)
		} else {
			s.t.Logf("unchanged %s", file)
		}
		return
	}
	want, err := readGolden(file)
	if err != nil {
		s.t.Fatal(err)
	}
	for _, d := range compareGolden(want, got) {
		s.t.Error(d)
	}
}

// conditionOf returns the recorded condition of type typ, or the zero value.
func conditionOf(p projection, typ string) normalisedCondition {
	for _, c := range p.Conditions {
		if c.Type == typ {
			return c
		}
	}
	return normalisedCondition{}
}

// TestRecorderIgnoresOtherObjects checks that a recording only holds
// the scenario's own object, even while another Terraform reconciles alongside it.
func TestRecorderIgnoresOtherObjects(t *testing.T) {
	s := newScenario(t, "")
	repo := s.gitRepo("char-recorder-source", helloWorldArtifact)
	spec := fmt.Sprintf(`
approvePlan: auto
path: ./terraform-hello-world-example
interval: 1h
sourceRef:
  kind: GitRepository
  name: %s
  namespace: flux-system
`, repo.Name)

	s.step("create")
	s.terraform("char-recorder-a", spec)

	other := charTerraformObject(t, "char-recorder-b", spec)
	g := NewWithT(t)
	g.Expect(k8sClient.Create(t.Context(), other)).To(Succeed())
	t.Cleanup(func() { waitResourceToBeDelete(NewWithT(t), other) })

	s.settle("Ready True", func(p projection) bool {
		return conditionOf(p, meta.ReadyCondition).Status == metav1.ConditionTrue
	})

	By("waiting for the unrecorded object to finish reconciling too")
	g.Eventually(func() bool {
		got := &infrav1.Terraform{}
		if err := k8sClient.Get(t.Context(), client.ObjectKeyFromObject(other), got); err != nil {
			return false
		}
		return conditionOf(projectionOf(got), meta.ReadyCondition).Status == metav1.ConditionTrue
	}, charSettleTimeout, interval).Should(BeTrue())

	g.Expect(charRecorder.keys()).To(Equal([]types.NamespacedName{s.key}))
	rec := charRecorder.snapshot(s.key)
	g.Expect(rec.Segments).To(HaveLen(1))
	g.Expect(rec.Segments[0].Timeline).NotTo(BeEmpty())
	g.Expect(rec.Segments[0].Results).NotTo(BeEmpty())
}

// TestSettleIgnoresEarlierSegments pins that settle only looks at the current
// segment and waits for one of its reconcile results, so a stale Ready True from
// an earlier segment cannot end the new segment early. It drives the recorder
// directly, so the timing is deterministic.
func TestSettleIgnoresEarlierSegments(t *testing.T) {
	s := newScenario(t, "")
	s.key = types.NamespacedName{Namespace: charNamespace, Name: "char-settle-unit"}
	charRecorder.start(s.key, "first")
	ready := projection{Generation: 1, Conditions: []normalisedCondition{{Type: meta.ReadyCondition, Status: metav1.ConditionTrue}}}
	charRecorder.addProjection(s.key, ready)
	charRecorder.addResult(s.key, recordedResult{RequeueAfter: "interval", Error: "nil"})
	s.step("second")

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.settle("Ready True", func(p projection) bool {
			return conditionOf(p, meta.ReadyCondition).Status == metav1.ConditionTrue
		})
	}()
	notDoneFor := func(d time.Duration, why string) {
		select {
		case <-done:
			t.Fatalf("settle returned early: %s", why)
		case <-time.After(d):
		}
	}

	notDoneFor(charQuietPeriod+time.Second, "the earlier segment's Ready True satisfied it")
	gen2 := ready
	gen2.Generation = 2
	charRecorder.addProjection(s.key, gen2)
	notDoneFor(charQuietPeriod+time.Second, "the current segment has no reconcile result yet")
	charRecorder.addResult(s.key, recordedResult{RequeueAfter: "interval", Error: "nil"})
	select {
	case <-done:
	case <-time.After(charQuietPeriod + 5*time.Second):
		t.Fatal("settle did not return after the current segment settled")
	}
}

// TestSettleWaitsForCurrentSegment drives a real second segment: after a spec
// patch the object still shows the old Ready True, and settle must wait for the
// reconcile of the patched generation.
func TestSettleWaitsForCurrentSegment(t *testing.T) {
	s := newScenario(t, "")
	repo := s.gitRepo("char-settle-source", helloWorldArtifact)
	s.step("apply")
	s.terraform("char-settle-touch", fmt.Sprintf(`
approvePlan: auto
path: ./terraform-hello-world-example
interval: 1h
sourceRef:
  kind: GitRepository
  name: %s
  namespace: flux-system
`, repo.Name))
	readyTrue := func(p projection) bool {
		return conditionOf(p, meta.ReadyCondition).Status == metav1.ConditionTrue
	}
	s.settle("Ready True", readyTrue)

	s.step("touch")
	s.patch(func(tf *infrav1.Terraform) { tf.Spec.Interval = metav1.Duration{Duration: 2 * time.Hour} })
	patched := s.get().Generation
	s.settle("Ready True", readyTrue)

	rec := charRecorder.snapshot(s.key)
	g := NewWithT(t)
	g.Expect(rec.Segments).To(HaveLen(2))
	touch := rec.Segments[1]
	g.Expect(touch.Results).NotTo(BeEmpty())
	g.Expect(touch.Timeline).NotTo(BeEmpty())
	last := touch.Timeline[len(touch.Timeline)-1]
	g.Expect(last.Generation).To(Equal(patched))
	g.Expect(last.ObservedGeneration).To(Equal(patched))
	g.Expect(readyTrue(last)).To(BeTrue())
}

// Helpers shared by the source, dependency and cross-namespace scenarios
// (Test_000440 to Test_000446).

// denyCrossNamespaceRefs sets NoCrossNamespaceRefs on the shared reconciler, as the
// tc000015 tests do, and restores it when the test ends. Call it after newScenario.
func denyCrossNamespaceRefs(t *testing.T) {
	t.Helper()
	original := reconciler.NoCrossNamespaceRefs
	reconciler.NoCrossNamespaceRefs = true
	t.Cleanup(func() { reconciler.NoCrossNamespaceRefs = original })
}

// otherNamespace creates the second namespace of a cross-namespace scenario and
// deletes it in cleanup. envtest has no namespace controller, so a deleted
// namespace stays Terminating: nothing may be created in it, and a rerun must
// tolerate it already existing.
func (s *scenario) otherNamespace(name string) {
	s.t.Helper()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := k8sClient.Create(s.t.Context(), ns); err != nil {
		s.g.Expect(apierrors.IsAlreadyExists(err)).To(BeTrue(), "creating namespace %s: %v", name, err)
	}
	s.t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), ns) })
}

// stripFinalizersOnCleanup force-deletes the scenario's Terraform in cleanup, for
// scenarios whose object may not finish a normal delete, or as a safety net where
// it should (see each caller). Call it after s.terraform, so that it runs before
// that call's own cleanup. For 441 and 443 the source is missing or denied, but
// destroy is not requested, so deletion completes and stripping is only a
// safety net.
func (s *scenario) stripFinalizersOnCleanup() {
	s.t.Helper()
	key := s.key
	s.t.Cleanup(func() {
		ctx := context.Background() // t.Context() is already cancelled in cleanups
		obj := &infrav1.Terraform{}
		if err := k8sClient.Get(ctx, key, obj); err != nil {
			return
		}
		_ = k8sClient.Delete(ctx, obj)
		NewWithT(s.t).Eventually(func() bool {
			cur := &infrav1.Terraform{}
			if err := k8sClient.Get(ctx, key, cur); err != nil {
				return apierrors.IsNotFound(err)
			}
			base := client.MergeFrom(cur.DeepCopy())
			cur.Finalizers = nil
			_ = k8sClient.Patch(ctx, cur, base)
			return false
		}, cleanupTimeoutSeconds, interval).Should(BeTrue())
	})
}

// expectStuck asserts, after a settled "hanging deletion" segment, that the
// recorded object still exists, is being deleted, and still has its finalizer.
func (s *scenario) expectStuck() {
	s.t.Helper()
	got := s.get()
	s.g.Expect(got.DeletionTimestamp).NotTo(BeNil())
	s.g.Expect(got.Finalizers).To(ContainElement(infrav1.TerraformFinalizer))
}

// gitRepoNoArtifact creates a GitRepository whose status has a Ready condition but
// no artifact (tc000014); cleanup deletes it.
func (s *scenario) gitRepoNoArtifact(name string) *sourcev1.GitRepository {
	s.t.Helper()
	repo := &sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: charNamespace},
		Spec: sourcev1.GitRepositorySpec{
			URL:       "https://github.com/openshift-fluxv2-poc/podinfo",
			Reference: &sourcev1.GitRepositoryRef{Branch: "master"},
			Interval:  metav1.Duration{Duration: 30 * time.Second},
		},
	}
	s.g.Expect(k8sClient.Create(s.t.Context(), repo)).To(Succeed())
	s.t.Cleanup(func() { waitResourceToBeDelete(NewWithT(s.t), repo) })
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := k8sClient.Get(s.t.Context(), client.ObjectKeyFromObject(repo), repo); err != nil {
			return err
		}
		repo.Status = sourcev1.GitRepositoryStatus{
			ObservedGeneration: repo.Generation,
			Conditions: []metav1.Condition{{
				Type:               meta.ReadyCondition,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				Reason:             "GitOperationSucceed",
				Message:            "Fetched revision: " + helloWorldArtifact.Revision,
			}},
		}
		return k8sClient.Status().Update(s.t.Context(), repo)
	})
	s.g.Expect(err).To(Succeed())
	return repo
}

// hwSourceSpec is the hello-world spec (interval 1h, retryInterval 20s) for the
// named source in flux-system. extra is a YAML fragment, empty or ending in a newline.
func hwSourceSpec(sourceKind, sourceName, sourceNamespace, extra string) string {
	return fmt.Sprintf(`
approvePlan: auto
path: ./terraform-hello-world-example
interval: 1h
retryInterval: 20s
%ssourceRef:
  kind: %s
  name: %s
  namespace: %s
`, extra, sourceKind, sourceName, sourceNamespace)
}

func readyIs(p projection, status metav1.ConditionStatus, reason string) bool {
	c := conditionOf(p, meta.ReadyCondition)
	return c.Status == status && c.Reason == reason
}
