package tfctl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// ApprovePlan approves the pending plan for a given terraform resource
func (c *CLI) ApprovePlan(out io.Writer, resource string, yamlFile string) error {
	key := types.NamespacedName{
		Name:      resource,
		Namespace: c.namespace,
	}
	terraform := infrav1.Terraform{}
	if err := c.client.Get(context.TODO(), key, &terraform); err != nil {
		return fmt.Errorf("resource %s not found", resource)
	}

	if terraform.Status.Plan.Pending == "" {
		fmt.Fprintln(out, "no plan pending")
		return nil
	}

	//plan := terraform.Status.Plan.Pending

	err := approvePlan(terraform, yamlFile)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, " Plan approval set. Please commit and push to continue.")

	return nil
}

func approvePlan(terraform infrav1.Terraform, filename string) error {
	fileBytes, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	rdr := &kio.ByteReader{
		Reader:            bytes.NewBuffer(fileBytes),
		PreserveSeqIndent: true,
	}
	nodes, err := rdr.Read()
	if err != nil {
		return err
	}
	for _, node := range nodes {
		// check name and namespace of the node
		if node.GetKind() == "Terraform" &&
			node.GetApiVersion() == "infra.contrib.fluxcd.io/v1alpha2" &&
			node.GetName() == terraform.Name &&
			node.GetNamespace() == terraform.Namespace {
			// set the plan approval
			err = node.PipeE(yaml.LookupCreate(yaml.ScalarNode, "spec", "approvePlan"))
			if err != nil {
				return err
			}
			err = node.PipeE(
				yaml.Lookup("spec"),
				yaml.FieldSetter{Name: "approvePlan", StringValue: terraform.Status.Plan.Pending},
			)
			if err != nil {
				return err
			}
		}
	}

	b := &bytes.Buffer{}
	wtr := &kio.ByteWriter{Writer: b}
	err = wtr.Write(nodes)
	if err != nil {
		return err
	}

	return os.WriteFile(filename, b.Bytes(), 0644)
}
