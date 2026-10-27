package controllers

import (
	"errors"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestNormaliseKeepsRevisionsAndPlanIDs(t *testing.T) {
	in := `Plan generated: set approvePlan: "plan-master-b8e362c206" to approve this plan. rev master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb at 2026-10-23T10:11:12Z in /tmp/abc123/x uid 0f8fad5b-d9cb-469f-a165-70867728950e from http://127.0.0.1:38211/file.tar.gz`
	want := `Plan generated: set approvePlan: "plan-master-b8e362c206" to approve this plan. rev master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb at <time> in <tmp> uid <uuid> from http://<artifact-server>/file.tar.gz`
	if got := normalise(in); got != want {
		t.Fatalf("got %q", got)
	}
}

func TestResultBuckets(t *testing.T) {
	retry := 20 * time.Second
	obj := &infrav1.Terraform{Spec: infrav1.TerraformSpec{
		Interval:      metav1.Duration{Duration: time.Hour},
		RetryInterval: &metav1.Duration{Duration: retry},
	}}
	cases := []struct {
		name    string
		res     ctrl.Result
		err     error
		wantRA  string
		wantErr string
	}{
		{"interval", ctrl.Result{RequeueAfter: time.Hour}, nil, "interval", "nil"},
		{"retry", ctrl.Result{RequeueAfter: retry}, nil, "retry", "nil"},
		{"other", ctrl.Result{RequeueAfter: 7 * time.Second}, nil, "other", "nil"},
		{"none", ctrl.Result{}, nil, "none", "nil"},
		{"terminal", ctrl.Result{}, reconcile.TerminalError(errors.New("x")), "none", "terminal"},
		{"error", ctrl.Result{}, errors.New("x"), "none", "error"},
	}
	for _, c := range cases {
		got := resultOf(obj, c.res, c.err, false)
		if got.RequeueAfter != c.wantRA || got.Error != c.wantErr {
			t.Errorf("%s: got %+v", c.name, got)
		}
	}
	// Requeue next to an error is ignored by controller-runtime, so it is not recorded.
	for name, err := range map[string]error{"error": errors.New("x"), "terminal": reconcile.TerminalError(errors.New("x"))} {
		got := resultOf(obj, ctrl.Result{Requeue: true}, err, false)
		if got.Requeue || got.Error != name {
			t.Errorf("requeue with %s: got %+v", name, got)
		}
	}
	if got := resultOf(obj, ctrl.Result{Requeue: true}, nil, false); !got.Requeue {
		t.Errorf("requeue without error must be recorded: got %+v", got)
	}
	if got := resultOf(nil, ctrl.Result{RequeueAfter: time.Second}, nil, true); got.RequeueAfter != "other" || !got.Panicked {
		t.Errorf("nil obj: got %+v", got)
	}
}

func prog(typ, msg string) normalisedCondition {
	return normalisedCondition{Type: typ, Status: metav1.ConditionUnknown, Reason: "Progressing", Message: msg}
}

func proj(conds ...normalisedCondition) projection {
	return projection{Conditions: conds}
}

func goldenOf(timeline ...projection) golden {
	return golden{Segments: []segment{{Name: "s", Timeline: timeline}}}
}

func TestCompareGoldenAllowsDroppedIntermediateTimeline(t *testing.T) {
	init := proj(prog("Ready", "Initializing"))
	x := proj(normalisedCondition{Type: "Ready", Status: metav1.ConditionUnknown, Reason: "TerraformPlannedWithChanges", Message: "planned"})
	final := proj(normalisedCondition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Succeeded", Message: "done"})
	if diffs := compareGolden(goldenOf(init, x, final), goldenOf(init, final)); len(diffs) != 0 {
		t.Fatalf("unexpected diffs: %v", diffs)
	}
}

func TestCompareGoldenRejectsMilestoneReorder(t *testing.T) {
	a := proj(prog("Ready", "Initializing"))
	b := proj(prog("Ready", "Terraform Planning"))
	diffs := compareGolden(goldenOf(a, b), goldenOf(b, a))
	if len(diffs) == 0 || !strings.Contains(strings.Join(diffs, "\n"), "milestones") {
		t.Fatalf("expected a milestones diff, got %v", diffs)
	}
}

func TestCompareGoldenRejectsEventChange(t *testing.T) {
	mk := func(reason string) golden {
		return golden{Segments: []segment{{Name: "seg", Events: []recordedEvent{{Type: "Normal", Reason: reason}}}}}
	}
	diffs := compareGolden(mk("TerraformOutputsWritten"), mk("OutputsWritingFailed"))
	if len(diffs) != 1 || !strings.Contains(diffs[0], "seg") || !strings.Contains(diffs[0], "events") {
		t.Fatalf("got %v", diffs)
	}
}

func TestMissingGoldenFails(t *testing.T) {
	_, err := readGolden("testdata/characterisation/does-not-exist.yaml")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("got %v", err)
	}
}

func TestMilestonesIgnoreLongLivedReconcilingMessage(t *testing.T) {
	rec := prog("Reconciling", "Fulfilling prerequisites")
	tl := []projection{
		proj(rec),
		proj(rec, prog("Ready", "Reconciliation in progress")),
		proj(rec, prog("Ready", "Reconciliation in progress")),
		proj(rec, prog("Ready", "Initializing")),
		proj(rec, prog("Ready", "Initializing")),
	}
	got := milestones(tl)
	want := []string{"Reconciling/Fulfilling prerequisites", "Ready/Reconciliation in progress", "Ready/Initializing"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v", got)
	}
}

func TestMilestonesIgnoreNonProgressingReason(t *testing.T) {
	notProgressing := normalisedCondition{Type: "Ready", Status: metav1.ConditionUnknown, Reason: "TerraformPlannedWithChanges", Message: "Terraform Planning"}
	tl := []projection{
		proj(prog("Ready", "Initializing")),
		proj(notProgressing),
		proj(prog("Ready", "Applying")),
	}
	got := milestones(tl)
	want := []string{"Ready/Initializing", "Ready/Applying"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v", got)
	}
}

func TestMilestonesSameProjectionOrderedByType(t *testing.T) {
	// The projection holds Reconciling before Ready; milestones orders the
	// pairs that first appear together by condition type: Ready first.
	tl := []projection{proj(prog("Reconciling", "Fulfilling prerequisites"), prog("Ready", "Reconciliation in progress"))}
	got := milestones(tl)
	want := []string{"Ready/Reconciliation in progress", "Reconciling/Fulfilling prerequisites"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v", got)
	}
}

func TestGoldenRoundTrip(t *testing.T) {
	path := t.TempDir() + "/g.yaml"
	g := goldenOf(proj(prog("Ready", "Initializing")))
	g.Segments[0].Results = []recordedResult{{RequeueAfter: "none", Error: "nil"}}
	if err := writeGolden(path, g); err != nil {
		t.Fatal(err)
	}
	back, err := readGolden(path)
	if err != nil {
		t.Fatal(err)
	}
	if diffs := compareGolden(g, back); len(diffs) != 0 {
		t.Fatalf("diffs: %v", diffs)
	}
}

func TestTimesAdvanced(t *testing.T) {
	r := &recorder{active: map[types.NamespacedName]*golden{}}
	key := types.NamespacedName{Namespace: charNamespace, Name: "char-times-unit"}
	p := projection{Generation: 1}
	r.start(key, "first")
	r.observe(key, p, map[string]string{})                   // first ever observation: nothing to compare
	r.observe(key, p, map[string]string{"lastPlanAt": "t1"}) // nil -> set
	r.observe(key, p, map[string]string{"lastPlanAt": "t1"}) // unchanged
	r.nextSegment(key, "second")
	r.observe(key, p, map[string]string{"lastPlanAt": "t1"}) // same as last of previous segment
	r.nextSegment(key, "third")
	r.observe(key, p, map[string]string{"lastPlanAt": "t2", "lastSuccessfulReconcileAt": "t2"}) // vs previous segment's last
	r.observe(key, p, map[string]string{"lastPlanAt": "t2", "lastSuccessfulReconcileAt": "t2", "lastDriftDetectedAt": "t3"})
	r.observe(key, p, map[string]string{"lastPlanAt": "t2", "lastSuccessfulReconcileAt": "t2", "lastDriftDetectedAt": "t4"})
	got := r.stop(key)

	want := [][]string{{"lastPlanAt"}, nil, {"lastDriftDetectedAt", "lastPlanAt", "lastSuccessfulReconcileAt"}}
	for i, w := range want {
		if strings.Join(got.Segments[i].TimesAdvanced, ",") != strings.Join(w, ",") {
			t.Errorf("segment %d: got %v, want %v", i, got.Segments[i].TimesAdvanced, w)
		}
	}

	r.start(key, "again")
	r.observe(key, p, map[string]string{"lastPlanAt": "t9"})
	if again := r.stop(key); len(again.Segments[0].TimesAdvanced) != 0 {
		t.Errorf("a new recording compared against a stopped one: %v", again.Segments[0].TimesAdvanced)
	}

	mk := func(times ...string) golden { return golden{Segments: []segment{{Name: "seg", TimesAdvanced: times}}} }
	diffs := compareGolden(mk("lastPlanAt"), mk("lastPlanAt", "lastSuccessfulReconcileAt"))
	if len(diffs) != 1 || !strings.Contains(diffs[0], "seg") || !strings.Contains(diffs[0], "TimesAdvanced") {
		t.Fatalf("got %v", diffs)
	}
	if diffs := compareGolden(mk(), golden{Segments: []segment{{Name: "seg", TimesAdvanced: []string{}}}}); len(diffs) != 0 {
		t.Fatalf("nil and empty TimesAdvanced differ: %v", diffs)
	}
}

func TestTimesOf(t *testing.T) {
	now := metav1.Now()
	tf := &infrav1.Terraform{Status: infrav1.TerraformStatus{LastPlanAt: &now, LastDriftDetectedAt: &now}}
	got := timesOf(tf)
	if len(got) != 2 || got["lastPlanAt"] == "" || got["lastDriftDetectedAt"] == "" {
		t.Fatalf("got %v", got)
	}
}

func TestUpdateSkipsUnchangedGolden(t *testing.T) {
	path := t.TempDir() + "/g.yaml"
	init := proj(prog("Ready", "Initializing"))
	mid := proj(normalisedCondition{Type: "Ready", Status: metav1.ConditionUnknown, Reason: "TerraformPlannedWithChanges", Message: "planned"})
	final := proj(normalisedCondition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Succeeded", Message: "done"})

	// Missing golden: written.
	wrote, diffs, err := updateGolden(path, goldenOf(init, mid, final))
	if err != nil || !wrote {
		t.Fatalf("missing golden: wrote=%v err=%v", wrote, err)
	}
	before, _ := os.ReadFile(path)

	// Equal under compareGolden (an intermediate entry dropped): not rewritten.
	wrote, diffs, err = updateGolden(path, goldenOf(init, final))
	if err != nil || wrote || len(diffs) != 0 {
		t.Fatalf("unchanged golden: wrote=%v diffs=%v err=%v", wrote, diffs, err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("unchanged golden was rewritten")
	}

	// A real change: rewritten, and the compareGolden lines returned.
	changed := goldenOf(init, final)
	changed.Segments[0].Events = []recordedEvent{{Type: "Normal", Reason: "X"}}
	wrote, diffs, err = updateGolden(path, changed)
	if err != nil || !wrote || len(diffs) == 0 || !strings.Contains(diffs[0], "events") {
		t.Fatalf("changed golden: wrote=%v diffs=%v err=%v", wrote, diffs, err)
	}
	back, err := readGolden(path)
	if err != nil {
		t.Fatal(err)
	}
	if d := compareGolden(changed, back); len(d) != 0 {
		t.Fatalf("rewritten golden differs: %v", d)
	}
}

func TestEventsCollapse(t *testing.T) {
	r := &recorder{active: map[types.NamespacedName]*golden{}}
	key := types.NamespacedName{Namespace: charNamespace, Name: "char-events-unit"}
	a := recordedEvent{Type: "Warning", Reason: "ReconciliationFailed", Message: "a"}
	b := recordedEvent{Type: "Normal", Reason: "Progressing", Message: "b"}
	r.start(key, "first")
	r.addEvent(key, a)
	r.addEvent(key, a)
	r.addEvent(key, b)
	r.addEvent(key, a)
	r.addEvent(key, a)
	r.nextSegment(key, "second")
	r.addEvent(key, a) // a new segment starts its own run
	got := r.stop(key)
	if want := []recordedEvent{a, b, a}; !reflect.DeepEqual(got.Segments[0].Events, want) {
		t.Errorf("first: got %+v", got.Segments[0].Events)
	}
	if want := []recordedEvent{a}; !reflect.DeepEqual(got.Segments[1].Events, want) {
		t.Errorf("second: got %+v", got.Segments[1].Events)
	}
}

func TestSpecHashMasksTempPaths(t *testing.T) {
	mk := func(p string) *infrav1.Terraform {
		return &infrav1.Terraform{Spec: infrav1.TerraformSpec{Path: p}}
	}
	a := projectionOf(mk("/var/folders/ab/T/TestX123/001/kubecfg")).SpecHash
	b := projectionOf(mk("/tmp/TestX456/002/kubecfg")).SpecHash
	c := projectionOf(mk("./terraform")).SpecHash
	if a != b || a == c {
		t.Fatalf("got %s %s %s", a, b, c)
	}
}

func TestMilestonesCollapseRepeatedBlocks(t *testing.T) {
	cases := []struct{ in, want []string }{
		{[]string{"A", "B", "A", "B", "A", "B"}, []string{"A", "B"}},
		{[]string{"A", "B", "C", "A", "B", "C"}, []string{"A", "B", "C"}},
		{[]string{"A", "B", "A", "C"}, []string{"A", "B", "A", "C"}},
		{[]string{"A", "A", "B"}, []string{"A", "B"}},
	}
	for _, c := range cases {
		if got := collapseRepeatedBlocks(c.in); strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("collapse(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestMilestonesStillDetectReorder(t *testing.T) {
	ab := goldenOf(proj(prog("Ready", "Initializing")), proj(prog("Ready", "Applying")))
	ba := goldenOf(proj(prog("Ready", "Applying")), proj(prog("Ready", "Initializing")))
	if diffs := compareGolden(ab, ba); !strings.Contains(strings.Join(diffs, "\n"), "milestones") {
		t.Fatalf("expected a milestones diff, got %v", diffs)
	}
}
