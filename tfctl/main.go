package tfctl

import (
	"fmt"
	"io"
	"net/http"

	"github.com/fluxcd/pkg/ssa"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"

	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	repo = "weaveworks/tf-controller"
)

// CLI is the main struct for the tfctl command line tool
type CLI struct {
	restConfig *rest.Config
	client     client.Client
	namespace  string
	terraform  string
	build      string
	release    string
}

// New returns a new CLI instance
func New(build, release string) *CLI {
	return &CLI{build: build, release: release}
}

// Init initializes the CLI instance for a given kubeconfig, namespace and terraform binary
func (c *CLI) Init(k8sConfig *rest.Config, config *viper.Viper) error {
	scheme := runtime.NewScheme()
	cobra.CheckErr(corev1.AddToScheme(scheme))
	cobra.CheckErr(appsv1.AddToScheme(scheme))
	cobra.CheckErr(infrav1.AddToScheme(scheme))

	client, err := client.NewWithWatch(k8sConfig, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return err
	}

	c.client = client
	c.namespace = config.GetString("namespace")
	c.terraform = config.GetString("terraform")

	return nil
}

func stringp(s string) *string {
	return &s
}

func download(version, resource string) ([]byte, error) {
	tpl := "https://github.com/%s/releases/download/%s/tf-controller.%s.yaml"

	url := fmt.Sprintf(tpl, repo, version, resource)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download manifest file: %w", err)
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download manifest file with status code: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, err
	}

	err = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close response body: %w", err)
	}

	return data, nil
}

func newManager(kubeClient client.Client) (*ssa.ResourceManager, error) {
	kubePoller := polling.NewStatusPoller(kubeClient, kubeClient.RESTMapper(), polling.Options{})

	return ssa.NewResourceManager(kubeClient, kubePoller, ssa.Owner{
		Field: "tf-controller",
		Group: "contrib.fluxcd.io",
	}), nil
}
