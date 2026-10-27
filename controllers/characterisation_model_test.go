package controllers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"
)

// updateGoldens rewrites the characterisation goldens instead of comparing.
var updateGoldens = flag.Bool("update", false, "rewrite characterisation goldens")

// projection is the observable slice of a Terraform object recorded at one point in time.
type projection struct {
	Generation             int64
	ObservedGeneration     int64
	SpecHash               string // first 12 hex chars of sha256(json(spec))
	Finalizers             []string
	Conditions             []normalisedCondition
	Plan                   infrav1.PlanStatus
	LastAttemptedRevision  string
	LastAppliedRevision    string
	LastPlannedRevision    string
	LastPlannedGeneration  int64
	ReconciliationFailures int64
	AvailableOutputs       []string
	InventoryEntries       int // -1 when status.inventory is nil
	Lock                   infrav1.LockStatus
	TimesSet               []string
	LastHandledReconcileAt string
}

type recordedEvent struct{ Type, Reason, Action, Message string }

type recordedResult struct {
	RequeueAfter string // "none" | "interval" | "retry" | "other"
	Requeue      bool
	Error        string // "nil" | "terminal" | "error"
	Panicked     bool
}

type segment struct {
	Name     string
	Timeline []projection
	Events   []recordedEvent
	Results  []recordedResult
	// TimesAdvanced is the sorted set of time fields (json names) whose value
	// changed between consecutive observed objects during the segment. The first
	// observation compares against the last object of the previous segment.
	TimesAdvanced []string
}

type golden struct{ Segments []segment }

var recordedConditionTypes = map[string]bool{
	meta.ReadyCondition:              true,
	meta.ReconcilingCondition:        true,
	meta.StalledCondition:            true,
	infrav1.ConditionTypePlan:        true,
	infrav1.ConditionTypeApply:       true,
	infrav1.ConditionTypeOutput:      true,
	infrav1.ConditionTypeHealthCheck: true,
	infrav1.ConditionTypeStateLocked: true,
}

var milestoneMessages = map[string]bool{
	"Fulfilling prerequisites":   true,
	"Reconciliation in progress": true,
	"Deletion in progress":       true,
	"Initializing":               true,
	"Terraform Planning":         true,
	"Applying":                   true,
	"Apply started":              true,
	"Breaking the glass ...":     true,
}

func projectionOf(obj *infrav1.Terraform) projection {
	specJSON, _ := json.Marshal(obj.Spec)
	// A spec may hold the artifact server's URL (webhooks, health checks), whose
	// port differs per run. Mask only that, so the hash stays stable.
	specJSON = rePort.ReplaceAll(specJSON, []byte("<artifact-server>"))
	// Temp paths (the drift kubeconfig in t.TempDir()) differ per run too.
	specJSON = reTmp.ReplaceAll(specJSON, []byte("<tmp>"))
	sum := sha256.Sum256(specJSON)

	var conds []normalisedCondition
	for _, c := range normaliseConditions(obj.Status.Conditions) {
		if !recordedConditionTypes[c.Type] {
			continue
		}
		c.Message = normalise(c.Message)
		conds = append(conds, c)
	}
	sort.SliceStable(conds, func(i, j int) bool { return conds[i].Type < conds[j].Type })

	entries := -1
	if obj.Status.Inventory != nil {
		entries = len(obj.Status.Inventory.Entries)
	}

	var times []string
	for name, set := range map[string]bool{
		"lastPlanAt":                    obj.Status.LastPlanAt != nil,
		"lastSuccessfulReconcileAt":     obj.Status.LastSuccessfulReconcileAt != nil,
		"lastDriftDetectedAt":           obj.Status.LastDriftDetectedAt != nil,
		"lastAppliedByDriftDetectionAt": obj.Status.LastAppliedByDriftDetectionAt != nil,
	} {
		if set {
			times = append(times, name)
		}
	}
	sort.Strings(times)

	return projection{
		Generation:             obj.Generation,
		ObservedGeneration:     obj.Status.ObservedGeneration,
		SpecHash:               hex.EncodeToString(sum[:])[:12],
		Finalizers:             append([]string(nil), obj.Finalizers...),
		Conditions:             conds,
		Plan:                   obj.Status.Plan,
		LastAttemptedRevision:  obj.Status.LastAttemptedRevision,
		LastAppliedRevision:    obj.Status.LastAppliedRevision,
		LastPlannedRevision:    obj.Status.LastPlannedRevision,
		LastPlannedGeneration:  obj.Status.LastPlannedGeneration,
		ReconciliationFailures: obj.Status.ReconciliationFailures,
		AvailableOutputs:       append([]string(nil), obj.Status.AvailableOutputs...),
		InventoryEntries:       entries,
		Lock:                   obj.Status.Lock,
		TimesSet:               times,
		LastHandledReconcileAt: obj.Status.LastHandledReconcileAt,
	}
}

// timesOf returns the raw values of the time fields that are set, by json name.
// They are kept outside the projection; only which of them advanced is recorded.
func timesOf(obj *infrav1.Terraform) map[string]string {
	out := map[string]string{}
	for name, t := range map[string]*metav1.Time{
		"lastPlanAt":                    obj.Status.LastPlanAt,
		"lastSuccessfulReconcileAt":     obj.Status.LastSuccessfulReconcileAt,
		"lastDriftDetectedAt":           obj.Status.LastDriftDetectedAt,
		"lastAppliedByDriftDetectionAt": obj.Status.LastAppliedByDriftDetectionAt,
	} {
		if t != nil {
			out[name] = t.UTC().Format(time.RFC3339Nano)
		}
	}
	return out
}

func resultOf(obj *infrav1.Terraform, res ctrl.Result, err error, panicked bool) recordedResult {
	r := recordedResult{RequeueAfter: "none", Requeue: res.Requeue, Error: "nil", Panicked: panicked}
	if res.RequeueAfter != 0 {
		switch {
		case obj != nil && res.RequeueAfter == obj.Spec.Interval.Duration:
			r.RequeueAfter = "interval"
		case obj != nil && res.RequeueAfter == obj.GetRetryInterval():
			r.RequeueAfter = "retry"
		default:
			r.RequeueAfter = "other"
		}
	}
	if err != nil {
		r.Requeue = false // controller-runtime ignores Requeue when an error is returned
		if isTerminal(err) {
			r.Error = "terminal"
		} else {
			r.Error = "error"
		}
	}
	return r
}

func isTerminal(err error) bool {
	return errors.Is(err, reconcile.TerminalError(nil))
}

var (
	reTime = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})`)
	reTmp  = regexp.MustCompile(`(?:/tmp/|/var/folders/|/private/var/folders/)[^\s"']*`)
	reUUID = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	rePort = regexp.MustCompile(`127\.0\.0\.1:\d+`)
)

// normalise masks the values that differ between runs and nothing else.
func normalise(s string) string {
	s = reTime.ReplaceAllString(s, "<time>")
	s = reTmp.ReplaceAllString(s, "<tmp>")
	s = reUUID.ReplaceAllString(s, "<uuid>")
	return rePort.ReplaceAllString(s, "<artifact-server>")
}

// milestones returns the ordered (conditionType, message) pairs that newly appear
// as Progressing milestones. Pairs within one projection are ordered by condition type.
func milestones(timeline []projection) []string {
	var out []string
	prev := map[string]bool{}
	for _, p := range timeline {
		cur := map[string]bool{}
		var fresh []string
		for _, c := range p.Conditions {
			if c.Reason != "Progressing" || !milestoneMessages[c.Message] {
				continue
			}
			key := c.Type + "/" + c.Message
			cur[key] = true
			if !prev[key] {
				fresh = append(fresh, key)
			}
		}
		sort.SliceStable(fresh, func(i, j int) bool {
			return strings.SplitN(fresh[i], "/", 2)[0] < strings.SplitN(fresh[j], "/", 2)[0]
		})
		out = append(out, fresh...)
		prev = cur
	}
	return collapseRepeatedBlocks(out)
}

// collapseRepeatedBlocks collapses any block that immediately repeats itself
// ([A B A B A B] becomes [A B], [A A] becomes [A]), for block lengths 1..n/2,
// until stable. Retry counts are not meaningful; milestone order is.
func collapseRepeatedBlocks(in []string) []string {
	out := in
	for changed := true; changed; {
		changed = false
	scan:
		for n := 1; n <= len(out)/2; n++ {
			for i := 0; i+2*n <= len(out); i++ {
				if slices.Equal(out[i:i+n], out[i+n:i+2*n]) {
					out = append(append([]string{}, out[:i+n]...), out[i+2*n:]...)
					changed = true
					break scan
				}
			}
		}
	}
	return out
}

// compareGolden returns one line per mismatch; empty means equal. Segments must
// match by name, and within each the events, results, TimesAdvanced, final
// timeline entry and milestone order must be equal. The timeline entries in
// between may differ, since retry counts vary between runs.
func compareGolden(want, got golden) []string {
	var diffs []string
	if len(want.Segments) != len(got.Segments) {
		return []string{fmt.Sprintf("segments: want %d, got %d", len(want.Segments), len(got.Segments))}
	}
	for i := range want.Segments {
		w, g := want.Segments[i], got.Segments[i]
		if w.Name != g.Name {
			diffs = append(diffs, fmt.Sprintf("segment %d: name want %q, got %q", i, w.Name, g.Name))
			continue
		}
		if !apiequality.Semantic.DeepEqual(w.Events, g.Events) {
			diffs = append(diffs, fmt.Sprintf("segment %q: events differ: want %+v, got %+v", w.Name, w.Events, g.Events))
		}
		if !apiequality.Semantic.DeepEqual(w.Results, g.Results) {
			diffs = append(diffs, fmt.Sprintf("segment %q: results differ: want %+v, got %+v", w.Name, w.Results, g.Results))
		}
		if !apiequality.Semantic.DeepEqual(w.TimesAdvanced, g.TimesAdvanced) {
			diffs = append(diffs, fmt.Sprintf("segment %q: TimesAdvanced differ: want %v, got %v", w.Name, w.TimesAdvanced, g.TimesAdvanced))
		}
		diffs = append(diffs, compareLast(w, g)...)
		if wm, gm := milestones(w.Timeline), milestones(g.Timeline); !apiequality.Semantic.DeepEqual(wm, gm) {
			diffs = append(diffs, fmt.Sprintf("segment %q: milestones differ: want %v, got %v", w.Name, wm, gm))
		}
	}
	return diffs
}

func compareLast(w, g segment) []string {
	if len(w.Timeline) == 0 || len(g.Timeline) == 0 {
		if len(w.Timeline) != len(g.Timeline) {
			return []string{fmt.Sprintf("segment %q: timeline: want %d entries, got %d", w.Name, len(w.Timeline), len(g.Timeline))}
		}
		return nil
	}
	wl, gl := w.Timeline[len(w.Timeline)-1], g.Timeline[len(g.Timeline)-1]
	if !apiequality.Semantic.DeepEqual(wl, gl) {
		return []string{fmt.Sprintf("segment %q: final timeline entry differs: want %+v, got %+v", w.Name, wl, gl)}
	}
	return nil
}

func readGolden(path string) (golden, error) {
	var g golden
	b, err := os.ReadFile(path)
	if err != nil {
		return g, fmt.Errorf("reading golden %s: %w", path, err)
	}
	if err := yaml.Unmarshal(b, &g); err != nil {
		return g, fmt.Errorf("parsing golden %s: %w", path, err)
	}
	return g, nil
}

// updateGolden writes got to path unless the golden there already equals it under
// compareGolden. It returns whether it wrote, and the compareGolden lines against
// the existing golden (none when the golden was missing).
func updateGolden(path string, got golden) (bool, []string, error) {
	existing, err := readGolden(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return true, nil, writeGolden(path, got)
	case err != nil:
		return false, nil, err
	}
	diffs := compareGolden(existing, got)
	if len(diffs) == 0 {
		return false, nil, nil
	}
	return true, diffs, writeGolden(path, got)
}

func writeGolden(path string, g golden) error {
	b, err := yaml.Marshal(g)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
