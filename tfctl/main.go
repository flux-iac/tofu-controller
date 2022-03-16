package tfctl

import (
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CLI is the main struct for the tfctl command line tool
type CLI struct {
	client    client.Reader
	namespace string
	terraform string
}

// New returns a new CLI instance
func New() *CLI {
	return &CLI{}
}

// Init initializes the CLI instance for a given kubeconfig, namespace and terraform binary
func (c *CLI) Init(kubeconfig, namespace, tfPath string) error {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = infrav1.AddToScheme(scheme)

	client, err := client.NewWithWatch(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return err
	}

	c.client = client
	c.namespace = namespace
	c.terraform = tfPath

	return nil
}
