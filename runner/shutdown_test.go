package runner_test

import (
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"

	. "github.com/onsi/gomega"
)

// An idle runner stops serving promptly once it is told to stop, and takes no
// more calls.
func TestShutdown_IdleThenPlan(t *testing.T) {
	g := NewWithT(t)
	h, _, id := terraformSession(t, nil)

	h.Stop()

	g.Eventually(h.ServeDone, 2*time.Second).Should(BeClosed())
	g.Expect(h.ServeErr()).NotTo(HaveOccurred())

	_, err := h.Client.Plan(t.Context(), &runner.PlanRequest{TfInstance: id})
	g.Expect(status.Code(err)).To(Equal(codes.Unavailable))
}

// Stopping the runner interrupts a running apply, and the caller is told the
// runner is shutting down.
func TestShutdown_CancelsInFlightApply(t *testing.T) {
	g := NewWithT(t)
	h, stub, id := terraformSession(t, nil)
	stub.Sleep("apply", 30)

	errCh := make(chan error, 1)
	go func() {
		_, err := h.Client.Apply(t.Context(), &runner.ApplyRequest{TfInstance: id})
		errCh <- err
	}()

	g.Eventually(func() bool { return stub.Sleeping("apply") }, 10*time.Second, 20*time.Millisecond).
		Should(BeTrue(), "apply is sleeping with its interrupt handler installed")
	h.Stop()

	var err error
	g.Eventually(errCh, 10*time.Second).Should(Receive(&err))
	g.Expect(status.Code(err)).To(Equal(codes.Canceled))
	g.Expect(status.Convert(err).Message()).To(ContainSubstring("runner shutting down"))
	g.Eventually(h.ServeDone, 10*time.Second).Should(BeClosed())
}

// Stopping the runner interrupts every running command, not only one. A
// session runs one command at a time, so the second plan runs in a newer
// session while the first is still running.
func TestShutdown_ConcurrentCalls(t *testing.T) {
	g := NewWithT(t)
	h, stub, first := terraformSession(t, nil)
	stub.Sleep("plan", 30)

	errCh := make(chan error, 2)
	plan := func(id, out string) {
		_, err := h.Client.Plan(t.Context(), &runner.PlanRequest{TfInstance: id, Out: filepath.Join(h.Dir, out), Refresh: true})
		errCh <- err
	}

	// Each plan must be sleeping with its interrupt handler installed before
	// the next step, so the stop interrupts both rather than one of them
	// mid-start.
	go plan(first, "first.tfplan")
	g.Eventually(func() bool { return stub.Sleeping("plan") }, 10*time.Second, 20*time.Millisecond).
		Should(BeTrue(), "the first plan is sleeping")
	stub.ClearSleeping("plan")

	tf := &infrav1.Terraform{ObjectMeta: metav1.ObjectMeta{Name: "stub", Namespace: "default"}}
	second := h.newSession(t, tf, stub.Path(), t.TempDir())
	go plan(second, "second.tfplan")
	g.Eventually(func() bool { return stub.Sleeping("plan") }, 10*time.Second, 20*time.Millisecond).
		Should(BeTrue(), "the second plan is sleeping")
	g.Expect(planCalls(stub)).To(Equal(2), "both plans are running")
	h.Stop()

	for range 2 {
		var err error
		g.Eventually(errCh, 10*time.Second).Should(Receive(&err))
		g.Expect(status.Code(err)).To(Equal(codes.Canceled))
		g.Expect(status.Convert(err).Message()).To(ContainSubstring("runner shutting down"))
	}
	g.Eventually(h.ServeDone, 10*time.Second).Should(BeClosed())
}

// A stop that arrives before the server has started serving, such as a
// SIGTERM during startup, is still a clean stop.
func TestShutdown_StopBeforeServe(t *testing.T) {
	g := NewWithT(t)
	shutdown := make(chan struct{})
	stop := make(chan struct{})
	close(stop)

	err := runner.Serve(bufconn.Listen(1<<20), grpc.NewServer(), shutdown, stop)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(shutdown).To(BeClosed())
}

// failingListener is a listener whose Accept always fails.
type failingListener struct{ net.Listener }

func (failingListener) Accept() (net.Conn, error) { return nil, errors.New("accept failed") }

// When serving fails, Serve reports why and still shuts the server down.
func TestShutdown_ServeFails(t *testing.T) {
	g := NewWithT(t)
	lis := bufconn.Listen(1 << 20)
	t.Cleanup(func() { _ = lis.Close() })
	shutdown := make(chan struct{})

	err := runner.Serve(failingListener{lis}, grpc.NewServer(), shutdown, make(chan struct{}))

	g.Expect(err).To(MatchError(ContainSubstring("accept failed")))
	g.Expect(shutdown).To(BeClosed())
}

// A call that arrives after shutdown has begun fails at once without running
// terraform.
func TestShutdown_CallAfterShutdown(t *testing.T) {
	g := NewWithT(t)
	shutdown := make(chan struct{})
	h := newHarnessWith(t, []harnessOption{withShutdown(shutdown)})
	stub := newStubTerraform(t)
	tf := &infrav1.Terraform{ObjectMeta: metav1.ObjectMeta{Name: "stub", Namespace: "default"}}
	id := h.newSession(t, tf, stub.Path(), t.TempDir())
	close(shutdown)

	_, err := h.Client.Plan(t.Context(), &runner.PlanRequest{TfInstance: id, Out: filepath.Join(h.Dir, "tfplan"), Refresh: true})

	g.Expect(status.Code(err)).To(Equal(codes.Canceled))
	g.Expect(status.Convert(err).Message()).To(ContainSubstring("runner shutting down"))
	g.Expect(planCalls(stub)).To(BeZero(), "terraform plan never ran")
}

// planCalls counts the plan invocations the stub has seen.
func planCalls(stub *stubTerraform) int {
	n := 0
	for _, argv := range stub.Calls() {
		if len(argv) > 0 && argv[0] == "plan" {
			n++
		}
	}
	return n
}
