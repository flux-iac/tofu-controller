package controllers

import (
	"context"
	"net"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"

	"github.com/flux-iac/tofu-controller/runner"
)

// planServer answers Plan after hold, or never if hold is zero. It signals on
// started each time a Plan call reaches it.
type planServer struct {
	runner.UnimplementedRunnerServer
	started chan struct{}
	hold    time.Duration
}

func (s *planServer) Plan(ctx context.Context, _ *runner.PlanRequest) (*runner.PlanReply, error) {
	select {
	case s.started <- struct{}{}:
	default:
	}

	if s.hold == 0 {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	select {
	case <-time.After(s.hold):
		return &runner.PlanReply{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// startPlanServer serves a planServer on addr, or on a free local port if addr
// is empty, and returns the address it listens on.
func startPlanServer(t *testing.T, addr string, hold time.Duration, opts ...grpc.ServerOption) (string, *grpc.Server, chan struct{}) {
	t.Helper()

	if addr == "" {
		addr = "127.0.0.1:0"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	started := make(chan struct{}, 1)
	server := grpc.NewServer(opts...)
	runner.RegisterRunnerServer(server, &planServer{started: started, hold: hold})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	return listener.Addr().String(), server, started
}

// freeLocalAddr returns a local address that nothing is listening on.
func freeLocalAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	return addr
}

// newWatchedRunnerClient builds a client the way getRunnerConnection does, minus
// mTLS: same service config (waitForReady plus UNAVAILABLE retries) and the
// same interceptor.
func newWatchedRunnerClient(t *testing.T, addr string, unreachableTimeout time.Duration) runner.RunnerClient {
	t.Helper()

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(runnerServiceConfig),
		grpc.WithChainUnaryInterceptor(runnerUnreachableInterceptor(unreachableTimeout)),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return runner.NewRunnerClient(conn)
}

// planInBackground calls Plan and delivers its error on the returned channel.
func planInBackground(ctx context.Context, client runner.RunnerClient) chan error {
	result := make(chan error, 1)
	go func() {
		_, err := client.Plan(ctx, &runner.PlanRequest{})
		result <- err
	}()
	return result
}

func waitForPlanToStart(t *testing.T, started chan struct{}) {
	t.Helper()

	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("Plan never reached the server")
	}
}

func TestRunnerCallHangsWhenRunnerDiesWithoutWatchdog(t *testing.T) {
	g := NewWithT(t)

	addr, server, started := startPlanServer(t, "", 0)
	client := newWatchedRunnerClient(t, addr, 0)

	// The reconcile context has no deadline. Cancel it at the end only so the
	// goroutine does not leak past the test.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := planInBackground(ctx, client)
	waitForPlanToStart(t, started)

	// Stopping the server mid-call is what the runner process dying does: the
	// in-flight call fails with UNAVAILABLE, gRPC retries it, and the retry
	// waits for a READY connection that never comes back.
	server.Stop()

	g.Consistently(result, 3*time.Second).ShouldNot(Receive())
}

func TestRunnerUnreachableInterceptorCancelsCallWhenRunnerDies(t *testing.T) {
	g := NewWithT(t)

	addr, server, started := startPlanServer(t, "", 0)
	client := newWatchedRunnerClient(t, addr, time.Second)

	result := planInBackground(context.Background(), client)
	waitForPlanToStart(t, started)
	server.Stop()

	var err error
	g.Eventually(result, 10*time.Second).Should(Receive(&err))
	g.Expect(status.Code(err)).To(Equal(codes.Unavailable))
	g.Expect(err.Error()).To(ContainSubstring(errRunnerUnreachable.Error()))
	g.Expect(err.Error()).To(ContainSubstring("/runner.Runner/Plan"))
}

func TestRunnerUnreachableInterceptorFailsFastAfterGivingUp(t *testing.T) {
	g := NewWithT(t)

	addr, server, started := startPlanServer(t, "", 0)
	client := newWatchedRunnerClient(t, addr, time.Second)

	result := planInBackground(context.Background(), client)
	waitForPlanToStart(t, started)
	server.Stop()
	g.Eventually(result, 10*time.Second).Should(Receive())

	// The reconcile follows a failed Plan with more calls on the same connection,
	// such as the deferred CleanupDir. They must not wait out the timeout again.
	begin := time.Now()
	_, err := client.CleanupDir(context.Background(), &runner.CleanupDirRequest{})
	g.Expect(status.Code(err)).To(Equal(codes.Unavailable))
	g.Expect(time.Since(begin)).To(BeNumerically("<", 500*time.Millisecond))
}

func TestRunnerUnreachableInterceptorKeepsLongHealthyCall(t *testing.T) {
	g := NewWithT(t)

	// The call takes several times the unreachable timeout, but the connection
	// stays READY throughout, so it must be allowed to finish.
	addr, _, _ := startPlanServer(t, "", 3*time.Second)
	client := newWatchedRunnerClient(t, addr, 500*time.Millisecond)

	_, err := client.Plan(context.Background(), &runner.PlanRequest{})
	g.Expect(err).NotTo(HaveOccurred())
}

func TestRunnerUnreachableInterceptorWaitsForRunnerToStart(t *testing.T) {
	g := NewWithT(t)

	// Nothing listens yet: the runner pod has an IP but its image is still being
	// pulled. That wait is bounded by RunnerRPCTimeout in setupTerraform, not by
	// the watchdog, so the call must keep waiting past the unreachable timeout.
	addr := freeLocalAddr(t)
	client := newWatchedRunnerClient(t, addr, 500*time.Millisecond)

	result := planInBackground(context.Background(), client)
	g.Consistently(result, 2*time.Second).ShouldNot(Receive())

	startPlanServer(t, addr, 100*time.Millisecond)

	var err error
	g.Eventually(result, 30*time.Second).Should(Receive(&err))
	g.Expect(err).NotTo(HaveOccurred())
}

func TestRunnerUnreachableInterceptorSurvivesGoaway(t *testing.T) {
	g := NewWithT(t)

	// MaxConnectionAge makes the server send GOAWAY while the call is still
	// running, as a proxy in front of the runner might. The call drains on the
	// old transport and the channel goes IDLE; the watchdog must not mistake that
	// for an unreachable runner.
	addr, _, _ := startPlanServer(t, "", 3*time.Second, grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionAge:      300 * time.Millisecond,
		MaxConnectionAgeGrace: time.Minute,
	}))
	client := newWatchedRunnerClient(t, addr, time.Second)

	_, err := client.Plan(context.Background(), &runner.PlanRequest{})
	g.Expect(err).NotTo(HaveOccurred())
}
