package tfctl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// Create generates a terraform resource
func (c *CLI) Create(
	out io.Writer,
	name string,
	namespace string,
	path string,
	source string,
	interval string,
	export bool,
) error {
	sourceParams := strings.Split(source, "/")
	if len(sourceParams) != 2 ||
		!(sourceParams[0] == "GitRepository" || sourceParams[0] == "Bucket" || sourceParams[0] == "OCIRepository") {
		return fmt.Errorf("source must be of kind GitRepository or Bucket or OCIRepository")
	}

	intervalParsed, err := time.ParseDuration(interval)
	if err != nil {
		return err
	}

	gvk := infrav1.GroupVersion.WithKind(infrav1.TerraformKind)
	terraform := infrav1.Terraform{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gvk.GroupVersion().String(),
			Kind:       gvk.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: infrav1.TerraformSpec{
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      sourceParams[0],
				Name:      sourceParams[1],
				Namespace: namespace,
			},
			Path: path,
			Interval: metav1.Duration{
				Duration: intervalParsed,
			},
		},
	}

	if export {
		return printTerraform(&terraform)
	}

	if err := c.client.Create(context.TODO(), &terraform); err != nil {
		return err
	}

	fmt.Fprintf(out, " created Terraform resource %s/%s\n", namespace, name)

	return nil
}

func printTerraform(terraform interface{}) error {
	data, err := yaml.Marshal(terraform)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, resourceToString(data))
	return err
}

func resourceToString(data []byte) string {
	data = bytes.Replace(data, []byte("  creationTimestamp: null\n"), []byte(""), 1)
	data = bytes.Replace(data, []byte("status:\n"), []byte(""), 1)
	data = bytes.Replace(data, []byte("  plan: {}\n"), []byte(""), 1)
	return string(data)
}
