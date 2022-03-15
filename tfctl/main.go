package tfctl

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"github.com/weaveworks/tf-controller/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/hashicorp/terraform-exec/tfexec"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CLI struct {
	client    client.Client
	namespace string
	terraform string
}

func newScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = infrav1.AddToScheme(scheme)
	return scheme
}

func New() *CLI {
	return &CLI{}
}

func (c *CLI) Init(kubeconfig, namespace, tfPath string) error {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}

	scheme := newScheme()

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

// ShowPlan displays the plan for the given Terraform resource
func (c *CLI) ShowPlan(out io.Writer, resource string) error {
	planSecret := &corev1.Secret{}

	key := types.NamespacedName{
		Name:      fmt.Sprintf("tfplan-default-%s", resource),
		Namespace: c.namespace,
	}

	if err := c.client.Get(context.TODO(), key, planSecret); err != nil {
		return fmt.Errorf("plan for resources %s not found", resource)
	}

	data, err := utils.GzipDecode(planSecret.Data["tfplan"])
	if err != nil {
		return err
	}

	tmpDir, err := ioutil.TempDir(os.TempDir(), "tfctl")
	if err != nil {
		return err
	}

	planFile, err := ioutil.TempFile(tmpDir, "tfctl-plan-")
	if err != nil {
		return err
	}
	defer func() {
		os.Remove(planFile.Name())
		os.Remove(tmpDir)
	}()

	if err := os.WriteFile(planFile.Name(), data, 0644); err != nil {
		return err
	}

	planFile.Close()

	tf, err := tfexec.NewTerraform(tmpDir, c.terraform)
	if err != nil {
		return err
	}

	result, err := tf.ShowPlanFileRaw(context.TODO(), planFile.Name())
	if err != nil {
		return err
	}

	fmt.Fprintf(out, result)

	return nil
}
