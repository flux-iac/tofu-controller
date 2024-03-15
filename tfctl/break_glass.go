package tfctl

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericclioptions"
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

	shell(context.TODO(), c.kubeconfigArgs, tfObject)

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

func shell(ctx context.Context, kubeconfigArgs *genericclioptions.ConfigFlags, tfObject types.NamespacedName) error {
	podName := tfObject.Name + "-tf-runner"
	namespace := tfObject.Namespace

	// set basic arguments for exec
	cmdArgs := []string{"exec", "--stdin", "--tty"}

	// add Namespace
	cmdArgs = append(cmdArgs, "--namespace", namespace)

	// add Username if set
	if kubeconfigArgs.Username != nil && *kubeconfigArgs.Username != "" {
		cmdArgs = append(cmdArgs, "--as", *kubeconfigArgs.Username)
	}

	// add ImpersonateGroup(s) if set
	if kubeconfigArgs.ImpersonateGroup != nil && len(*kubeconfigArgs.ImpersonateGroup) != 0 {
		for _, group := range *kubeconfigArgs.ImpersonateGroup {
			cmdArgs = append(cmdArgs, "--as-group", group)
		}
	}

	// add ImpersonateUID if set
	if kubeconfigArgs.ImpersonateUID != nil && *kubeconfigArgs.ImpersonateUID != "" {
		cmdArgs = append(cmdArgs, "--as-uid", *kubeconfigArgs.ImpersonateUID)
	}

	// add CacheDir if set
	if kubeconfigArgs.CacheDir != nil && *kubeconfigArgs.CacheDir != "" {
		cmdArgs = append(cmdArgs, "--cache-dir", *kubeconfigArgs.CacheDir)
	}

	// add CAFile if set
	if kubeconfigArgs.CAFile != nil && *kubeconfigArgs.CAFile != "" {
		cmdArgs = append(cmdArgs, "--certificate-authority", *kubeconfigArgs.CAFile)
	}

	// add CertFile if set
	if kubeconfigArgs.CertFile != nil && *kubeconfigArgs.CertFile != "" {
		cmdArgs = append(cmdArgs, "--client-certificate", *kubeconfigArgs.CertFile)
	}

	// add KeyFile if set
	if kubeconfigArgs.KeyFile != nil && *kubeconfigArgs.KeyFile != "" {
		cmdArgs = append(cmdArgs, "--client-key", *kubeconfigArgs.KeyFile)
	}

	// add ClusterName if set
	if kubeconfigArgs.ClusterName != nil && *kubeconfigArgs.ClusterName != "" {
		cmdArgs = append(cmdArgs, "--cluster", *kubeconfigArgs.ClusterName)
	}

	// add Context if set
	if kubeconfigArgs.Context != nil && *kubeconfigArgs.Context != "" {
		cmdArgs = append(cmdArgs, "--context", *kubeconfigArgs.Context)
	}

	// add DisableCompression if set
	if kubeconfigArgs.DisableCompression != nil && *kubeconfigArgs.DisableCompression {
		cmdArgs = append(cmdArgs, "--disable-compression")
	}

	// add Insecure if set
	if kubeconfigArgs.Insecure != nil && *kubeconfigArgs.Insecure {
		cmdArgs = append(cmdArgs, "--insecure-skip-tls-verify")
	}

	// add Kubeconfig file if set
	if kubeconfigArgs.KubeConfig != nil && *kubeconfigArgs.KubeConfig != "" {
		cmdArgs = append(cmdArgs, "--kubeconfig", *kubeconfigArgs.KubeConfig)
	}

	// add Timeout if set
	if kubeconfigArgs.Timeout != nil && *kubeconfigArgs.Timeout != "" {
		cmdArgs = append(cmdArgs, "--request-timeout", *kubeconfigArgs.Timeout)
	}

	// add APIServer if set
	if kubeconfigArgs.APIServer != nil && *kubeconfigArgs.APIServer != "" {
		cmdArgs = append(cmdArgs, "--server", *kubeconfigArgs.APIServer)
	}

	// add TLSServerName if set
	if kubeconfigArgs.TLSServerName != nil && *kubeconfigArgs.TLSServerName != "" {
		cmdArgs = append(cmdArgs, "--tls-server-name", *kubeconfigArgs.TLSServerName)
	}

	// add BearerToken if set
	if kubeconfigArgs.BearerToken != nil && *kubeconfigArgs.BearerToken != "" {
		cmdArgs = append(cmdArgs, "--token", *kubeconfigArgs.BearerToken)
	}

	// add AuthInfoName if set
	if kubeconfigArgs.AuthInfoName != nil && *kubeconfigArgs.AuthInfoName != "" {
		cmdArgs = append(cmdArgs, "--user", *kubeconfigArgs.AuthInfoName)
	}

	// add podName of the runner
	cmdArgs = append(cmdArgs, podName)

	// add command to run for break-glass
	cmdArgs = append(cmdArgs, "--", "/bin/sh", "-c", "cd /tmp/"+tfObject.Namespace+"-"+tfObject.Name+" && /bin/sh && rm /tmp/.break-glass")

	cmd := exec.Command("kubectl", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}
