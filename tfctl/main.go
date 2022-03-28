package tfctl

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/fluxcd/pkg/ssa"
	"github.com/spf13/viper"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"

	"k8s.io/cli-runtime/pkg/genericclioptions"
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

type Config struct {
	*viper.Viper
}

// New returns a new CLI instance
func New(build, release string) *CLI {
	return &CLI{build: build, release: release}
}

// Init initializes the CLI instance for a given kubeconfig, namespace and terraform binary
func (c *CLI) Init(config *Config) error {
	var kubeconfigArgs = genericclioptions.NewConfigFlags(false)

	kubeconfigArgs.KubeConfig = stringp(config.GetString("kubeconfig"))

	if config.GetString("context") != "" {
		kubeconfigArgs.Context = stringp(config.GetString("context"))
	}

	if config.GetString("cluster") != "" {
		kubeconfigArgs.ClusterName = stringp(config.GetString("cluster"))
	}

	k8sConfig, err := kubeconfigArgs.ToRESTConfig()
	if err != nil {
		return err
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = infrav1.AddToScheme(scheme)

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
		return nil, err
	}
	defer resp.Body.Close()

	var data bytes.Buffer
	_, err = io.Copy(&data, resp.Body)
	if err != nil {
		return nil, err
	}

	return data.Bytes(), nil
}

func newManager(kubeClient client.Client) (*ssa.ResourceManager, error) {
	kubePoller := polling.NewStatusPoller(kubeClient, kubeClient.RESTMapper(), polling.Options{})

	return ssa.NewResourceManager(kubeClient, kubePoller, ssa.Owner{
		Field: "tf-controller",
		Group: "contrib.fluxcd.io",
	}), nil
}
