package runner_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flux-iac/tofu-controller/controllers"
	"github.com/flux-iac/tofu-controller/runner"
	"github.com/onsi/gomega"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/runtime/logger"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

const (
	// longer timeout duration is helpful to avoid flakiness when
	// asserting on k8s resources created via Terraform
	timeout               = time.Second * 30
	interval              = time.Millisecond * 500
	cleanupTimeoutSeconds = 60
)

var (
	cfg          *rest.Config
	k8sClient    client.Client
	testEnv      *envtest.Environment
	reconciler   *controllers.TerraformReconciler
	runnerServer *runner.TerraformRunnerServer
	runnerClient runner.RunnerClient
)

var (
	ctx       context.Context
	cancel    context.CancelFunc
	logBuffer bytes.Buffer
)

func TestMain(m *testing.M) {
	var err error

	if os.Getenv("WITH_STDOUT_LOGGER") != "" {
		ctrl.SetLogger(logger.NewLogger(logger.Options{}))
	} else {
		ctrl.SetLogger(zap.New(zap.WriteTo(&logBuffer)))
	}

	ctx, cancel = context.WithCancel(context.TODO())
	// "bootstrapping test environment"
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:       []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing:   true,
		ControlPlaneStopTimeout: 60 * time.Second,
	}

	cfg, err := testEnv.Start()
	if err != nil {
		panic(err.Error())
	}
	if cfg == nil {
		panic("cfg cannot be nil")
	}

	scheme := runtime.NewScheme()

	err = clientgoscheme.AddToScheme(scheme)
	if err != nil {
		panic(err.Error())
	}

	err = sourcev1.AddToScheme(scheme)
	if err != nil {
		panic(err.Error())
	}

	err = sourcev1b2.AddToScheme(scheme)
	if err != nil {
		panic(err.Error())
	}

	err = infrav1.AddToScheme(scheme)
	if err != nil {
		panic(err.Error())
	}

	//+kubebuilder:scaffold:scheme
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		panic(err.Error())
	}
	if k8sClient == nil {
		panic("k8sClient cannot be nil")
	}

	// "preparing flux-system namespace"
	fluxSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "flux-system",
		},
		Spec: corev1.NamespaceSpec{},
	}
	err = k8sClient.Create(ctx, fluxSystemNS)
	if err != nil {
		panic(err.Error())
	}

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Logger: ctrl.LoggerFrom(ctx).WithName("controller"),
	})
	if err != nil {
		panic(err.Error())
	}

	stopRunnerServer := make(chan os.Signal)
	runnerServer = &runner.TerraformRunnerServer{
		Client:     k8sManager.GetClient(),
		Scheme:     k8sManager.GetScheme(),
		Done:       stopRunnerServer,
		InstanceID: "test",
	}

	grpcAddr := "localhost:30000"
	go func() {
		err := startGRPCServer(runnerServer, grpcAddr)
		if err != nil {
			fmt.Println(err.Error())
		}
	}()

	runnerClient, err = getGrpcClient(ctx, grpcAddr)
	if err != nil {
		panic(err.Error())
	}

	go func() {
		if err := k8sManager.Start(ctx); err != nil {
			fmt.Println(err.Error())
		}
	}()

	code := m.Run()
	cancel()
	close(stopRunnerServer)

	// "tearing down the test environment"
	err = testEnv.Stop()
	if err != nil {
		fmt.Println(err.Error())
	}

	if os.Getenv("DISABLE_K8S_LOGS") != "1" {
		if code > 0 {
			fmt.Println(logBuffer.String())
		}
	}

	os.Exit(code)
}

func findKubeConfig(e *envtest.Environment) (string, error) {
	files, err := ioutil.ReadDir(e.ControlPlane.APIServer.CertDir)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".kubecfg") {
			return filepath.Join(e.ControlPlane.APIServer.CertDir, file.Name()), nil
		}
	}

	return "", fmt.Errorf("file not found")
}

func startGRPCServer(server *runner.TerraformRunnerServer, addr string) error {
	grpcServer := grpc.NewServer()

	// local runner, use the same client as the manager
	runner.RegisterRunnerServer(grpcServer, server)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil
	}
	if err := grpcServer.Serve(listener); err != nil {
		return err
	}

	return nil
}

func getGrpcClient(ctx context.Context, addr string) (runner.RunnerClient, error) {
	const retryPolicy = `{
"methodConfig": [{
  "name": [{"service": "runner.Runner"}],
  "waitForReady": true,
  "retryPolicy": {
    "MaxAttempts": 4,
    "InitialBackoff": ".01s",
    "MaxBackoff": ".01s",
    "BackoffMultiplier": 1.0,
    "RetryableStatusCodes": [ "UNAVAILABLE" ]
  }
}]}`

	grpcConn, err := grpc.DialContext(ctx, addr, grpc.WithBlock(), grpc.WithDefaultServiceConfig(retryPolicy), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %v", addr, err)
	}

	return runner.NewRunnerClient(grpcConn), nil
}

func waitResourceToBeDelete(g gomega.Gomega, resource client.Object) {
	ctx := context.Background()
	key := types.NamespacedName{Namespace: resource.GetNamespace(), Name: resource.GetName()}

	err := k8sClient.Get(ctx, key, resource)
	if apierrors.IsNotFound(err) {
		return
	}

	g.Expect(k8sClient.Delete(ctx, resource)).Should(gomega.Succeed())
	g.Eventually(func() error {
		return k8sClient.Get(ctx, key, resource)
	}, cleanupTimeoutSeconds, interval).ShouldNot(gomega.Succeed())
}
