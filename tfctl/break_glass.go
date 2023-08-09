package tfctl

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *CLI) BreakTheGlass(out io.Writer, resource string) error {
	tfObject := types.NamespacedName{
		Name:      resource,
		Namespace: c.namespace,
	}

	if err := requestBreakingTheGlass(context.TODO(), c.client, tfObject); err != nil {
		return err
	}
	fmt.Fprintf(out, " Break the glass requested for %s/%s\n", c.namespace, resource)
	if err := requestReconciliation(context.TODO(), c.client, tfObject); err != nil {
		return err
	}

	defer func() {
		err := removeBreakingTheGlass(context.TODO(), c.client, tfObject)
		if err != nil {
			fmt.Fprintf(out, " Failed to remove break the glass annotation for %s/%s\n", c.namespace, resource)
		}
	}()

	terraform := &infrav1.Terraform{}
	err := wait.PollImmediate(5*time.Second, 5*time.Minute, func() (bool, error) {
		err := c.client.Get(context.TODO(), tfObject, terraform)
		if err != nil {
			return false, nil
		}

		for _, condition := range terraform.Status.Conditions {
			if condition.Type == "Ready" {
				fmt.Println("Waiting: ", condition.Message)
				if condition.Status == "Unknown" && condition.Reason == "Progressing" && condition.Message == "Breaking the glass ..." {
					fmt.Println("The glass is breaking!")
					return true, nil
				} else if condition.Status == "Unknown" && condition.Reason == "Progressing" && condition.Message == "Breaking the glass is not allowed" {
					return true, fmt.Errorf("breaking the glass is not allowed")
				}
			}
		}

		return false, nil
	})

	if err != nil {
		return err
	}

	shell(context.TODO(), c.restConfig, tfObject)

	return nil
}

func requestBreakingTheGlass(ctx context.Context, kubeClient client.Client, namespacedName types.NamespacedName) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		terraform := &infrav1.Terraform{}
		if err := kubeClient.Get(ctx, namespacedName, terraform); err != nil {
			return err
		}
		patch := client.MergeFrom(terraform.DeepCopy())
		if ann := terraform.GetAnnotations(); ann == nil {
			terraform.SetAnnotations(map[string]string{
				infrav1.BreakTheGlassAnnotation: time.Now().Format(time.RFC3339Nano),
			})
		} else {
			ann[infrav1.BreakTheGlassAnnotation] = time.Now().Format(time.RFC3339Nano)
			terraform.SetAnnotations(ann)
		}
		return kubeClient.Patch(ctx, terraform, patch)
	})
}

func removeBreakingTheGlass(ctx context.Context, kubeClient client.Client, namespacedName types.NamespacedName) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		terraform := &infrav1.Terraform{}
		if err := kubeClient.Get(ctx, namespacedName, terraform); err != nil {
			return err
		}
		patch := client.MergeFrom(terraform.DeepCopy())
		if ann := terraform.GetAnnotations(); ann == nil {
			// ignore
		} else {
			delete(ann, infrav1.BreakTheGlassAnnotation)
			terraform.SetAnnotations(ann)
		}
		return kubeClient.Patch(ctx, terraform, patch)
	})
}

func shell(ctx context.Context, restConfig *rest.Config, tfObject types.NamespacedName) error {
	// kubectl exec --stdin --tty -n flux-system helloworld-tf-tf-runner -- /bin/sh
	podName := tfObject.Name + "-tf-runner"
	namespace := tfObject.Namespace

	cmd := exec.Command("kubectl", "exec",
		"--stdin", "--tty", "-n", namespace, podName,
		"--", "/bin/sh", "-c", "cd /tmp/"+tfObject.Namespace+"-"+tfObject.Name+" && /bin/sh && rm /tmp/.break-glass")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}
