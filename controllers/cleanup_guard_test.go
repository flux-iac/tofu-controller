package controllers

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// nonTestControllerSources returns the contents of every non-test .go file in this package.
func nonTestControllerSources(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[f] = string(b)
	}
	return out
}

func TestNoTraceBreadcrumbs(t *testing.T) {
	for f, src := range nonTestControllerSources(t) {
		if strings.Contains(src, "traceLog.Info(") {
			t.Errorf("%s still contains traceLog.Info( breadcrumbs", f)
		}
	}
}

func TestRequeueTrueOnlyAtFinalizerAdd(t *testing.T) {
	n := 0
	for _, src := range nonTestControllerSources(t) {
		n += strings.Count(src, "Requeue: true")
	}
	if n != 1 {
		t.Errorf("want exactly 1 `Requeue: true` (finalizer add), got %d", n)
	}
}

func TestNoSprintfInLogCalls(t *testing.T) {
	for f, src := range nonTestControllerSources(t) {
		for _, problem := range sprintfLogCalls(src) {
			t.Errorf("%s: %s; use key/values", f, problem)
		}
	}
}

var (
	// sprintfInLogCall matches a logger call that formats its message inline.
	sprintfInLogCall = regexp.MustCompile(`\.(Info|Error)\([^\n]*fmt\.Sprintf\(|\.Info\(\s*fmt\.Sprintf\(`)
	// sprintfAssign matches `v := fmt.Sprintf(` or `v = fmt.Sprintf(`.
	sprintfAssign = regexp.MustCompile(`(\w+)\s*:?=\s*fmt\.Sprintf\(`)
)

// sprintfLogCalls returns the logger calls in src whose message is built with
// fmt.Sprintf, either inline or through a variable assigned from fmt.Sprintf
// earlier in the same function.
func sprintfLogCalls(src string) []string {
	var problems []string
	if sprintfInLogCall.MatchString(src) {
		problems = append(problems, "a log call wraps fmt.Sprintf")
	}
	for _, fn := range strings.Split(src, "\nfunc ") {
		for _, m := range sprintfAssign.FindAllStringSubmatch(fn, -1) {
			v := regexp.QuoteMeta(m[1])
			usedAsMessage := regexp.MustCompile(`\.Info\(\s*` + v + `\s*[,)]|\.Error\([^,\n]+,\s*` + v + `\s*[,)]`)
			if usedAsMessage.MatchString(fn) {
				problems = append(problems, "a log message is the fmt.Sprintf result "+m[1])
			}
		}
	}
	return problems
}

func TestSprintfLogCallsCatchesVariableMessages(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{"inline Info", "func a() {\n\tlog.Info(fmt.Sprintf(\"x %s\", y))\n}", true},
		{"inline on a logger expression", "func a() {\n\tctrl.LoggerFrom(ctx).Info(fmt.Sprintf(\"x %s\", y))\n}", true},
		{"variable Info", "func a() {\n\tmsg := fmt.Sprintf(\"x %s\", y)\n\tlog.Info(msg)\n}", true},
		{"variable Error", "func a() {\n\tmsg := fmt.Sprintf(\"x %s\", y)\n\tlog.Error(err, msg)\n}", true},
		{"variable used elsewhere", "func a() {\n\tmsg := fmt.Sprintf(\"x %s\", y)\n\tsetCondition(msg)\n\tlog.Info(\"done\")\n}", false},
		{"variable in another function", "func a() {\n\tmsg := fmt.Sprintf(\"x %s\", y)\n\tuse(msg)\n}\nfunc b() {\n\tlog.Info(msg)\n}", false},
		{"key/values", "func a() {\n\tlog.Info(\"x\", \"y\", y)\n}", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(sprintfLogCalls(tt.src)) > 0; got != tt.want {
				t.Errorf("sprintfLogCalls(%q) found=%v, want %v", tt.src, got, tt.want)
			}
		})
	}
}
