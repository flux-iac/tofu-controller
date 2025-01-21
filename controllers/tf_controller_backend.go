package controllers

import (
	"context"
	"fmt"
	"os"
	"strings"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	"github.com/fluxcd/pkg/runtime/acl"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *TerraformReconciler) backendCompletelyDisable(terraform infrav1.Terraform) bool {
	if terraform.Spec.Cloud != nil {
		return true
	}

	return terraform.Spec.BackendConfig != nil && terraform.Spec.BackendConfig.Disable == true
}

func (r *TerraformReconciler) setupTerraform(ctx context.Context, runnerClient runner.RunnerClient, terraform infrav1.Terraform, sourceObj sourcev1.Source, revision string, objectKey types.NamespacedName, reconciliationLoopID string) (infrav1.Terraform, string, string, error) {
	log := ctrl.LoggerFrom(ctx)

	tfInstance := "0"
	tmpDir := ""

	terraform = infrav1.TerraformProgressing(terraform, "Initializing")
	if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
		log.Error(err, "unable to update status before Terraform initialization")
		return terraform, tfInstance, tmpDir, err
	}

	// download artifact and extract files
	buf, err := r.downloadAsBytes(sourceObj.GetArtifact())
	if err != nil {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.ArtifactFailedReason,
			err.Error(),
		), tfInstance, tmpDir, err
	}

	// we fix timeout of UploadAndExtract to be 30s
	// ctx30s, cancelCtx30s := context.WithTimeout(ctx, 30*time.Second)
	// defer cancelCtx30s()
	uploadAndExtractReply, err := runnerClient.UploadAndExtract(ctx, &runner.UploadAndExtractRequest{
		Namespace: terraform.Namespace,
		Name:      terraform.Name,
		TarGz:     buf.Bytes(),
		Path:      terraform.Spec.Path,
	})
	if err != nil {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.ArtifactFailedReason,
			err.Error(),
		), tfInstance, tmpDir, err
	}
	workingDir := uploadAndExtractReply.WorkingDir
	tmpDir = uploadAndExtractReply.TmpDir

	var backendConfig string
	DisableTFK8SBackend := os.Getenv("DISABLE_TF_K8S_BACKEND") == "1"

	if terraform.Spec.BackendConfig != nil && terraform.Spec.BackendConfig.CustomConfiguration != "" {
		backendConfig = fmt.Sprintf(`
terraform {
  %v
}
`,
			terraform.Spec.BackendConfig.CustomConfiguration)
	} else if terraform.Spec.BackendConfig != nil {
		backendConfig = fmt.Sprintf(`
terraform {
  backend "kubernetes" {
    secret_suffix     = "%s"
    in_cluster_config = %v
    config_path       = "%s"
    namespace         = "%s"
    labels            = {
      %s
    }
  }
}
`,
			terraform.Spec.BackendConfig.SecretSuffix,
			terraform.Spec.BackendConfig.InClusterConfig,
			terraform.Spec.BackendConfig.ConfigPath,
			terraform.Namespace,
			getLabelsAsHCL(terraform.Labels, 6))
	} else if DisableTFK8SBackend && terraform.Spec.BackendConfig == nil {
		backendConfig = `
terraform {
  backend "local" { }
}`
	} else if terraform.Spec.BackendConfig == nil {
		// TODO must be tested in cluster only
		backendConfig = fmt.Sprintf(`
terraform {
  backend "kubernetes" {
    secret_suffix     = "%s"
    in_cluster_config = true
    namespace         = "%s"
    labels            = {
      %s
    }
  }
}
`,
			terraform.Name,
			terraform.Namespace,
			getLabelsAsHCL(terraform.Labels, 6))
	}

	if r.backendCompletelyDisable(terraform) {
		log.Info("backendConfig is completely disabled. When Spec.Cloud is not nil, backendConfig is disabled by default too.")
		if terraform.Spec.Cloud != nil {
			log.Info("Spec.Cloud is not nil. backendConfig is disabled by default.")
			writeBackendConfigReply, err := runnerClient.WriteBackendConfig(ctx,
				&runner.WriteBackendConfigRequest{
					DirPath:       workingDir,
					BackendConfig: []byte(terraform.Spec.Cloud.ToHCL()),
				})
			if err != nil {
				log.Error(err, "write cloud config error")
				return terraform, tfInstance, tmpDir, err
			}
			log.Info(fmt.Sprintf("write cloud config: %s", writeBackendConfigReply.Message))
		}
	} else {
		writeBackendConfigReply, err := runnerClient.WriteBackendConfig(ctx,
			&runner.WriteBackendConfigRequest{
				DirPath:       workingDir,
				BackendConfig: []byte(backendConfig),
			})
		if err != nil {
			log.Error(err, "write backend config error")
			return terraform, tfInstance, tmpDir, err
		}
		log.Info(fmt.Sprintf("write backend config: %s", writeBackendConfigReply.Message))
	}

	var tfrcFilepath string
	if terraform.Spec.CliConfigSecretRef != nil {
		cliConfigSecretRef := *(terraform.Spec.CliConfigSecretRef.DeepCopy())
		if cliConfigSecretRef.Namespace == "" {
			cliConfigSecretRef.Namespace = terraform.Namespace
		}

		if r.NoCrossNamespaceRefs && cliConfigSecretRef.Namespace != terraform.GetNamespace() {
			msg := fmt.Sprintf("cannot access secret %s/%s, cross-namespace references have been disabled", cliConfigSecretRef.Namespace, cliConfigSecretRef.Name)
			terraform = infrav1.TerraformNotReady(terraform, revision, infrav1.AccessDeniedReason, msg)
			return terraform, tfInstance, tmpDir, acl.AccessDeniedError(msg)
		}

		processCliConfigReply, err := runnerClient.ProcessCliConfig(ctx, &runner.ProcessCliConfigRequest{
			DirPath:   workingDir,
			Namespace: cliConfigSecretRef.Namespace,
			Name:      cliConfigSecretRef.Name,
		})
		if err != nil {
			err = fmt.Errorf("cannot process cli config: %s", err.Error())
			return infrav1.TerraformNotReady(
				terraform,
				revision,
				infrav1.TFExecNewFailedReason,
				err.Error(),
			), tfInstance, tmpDir, err
		}
		tfrcFilepath = processCliConfigReply.FilePath
	}

	lookPathReply, err := runnerClient.LookPath(ctx,
		&runner.LookPathRequest{
			File: "terraform",
		})
	if err != nil {
		err = fmt.Errorf("cannot find Terraform binary: %s in %s", err, os.Getenv("PATH"))
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecNewFailedReason,
			err.Error(),
		), tfInstance, tmpDir, err
	}
	execPath := lookPathReply.ExecPath

	log.Info("new terraform", "workingDir", workingDir)

	terraformBytes, err := terraform.ToBytes(r.Scheme)
	if err != nil {
		// transient error?
		return terraform, tfInstance, tmpDir, err
	}

	newTerraformReply, err := runnerClient.NewTerraform(ctx,
		&runner.NewTerraformRequest{
			WorkingDir: workingDir,
			ExecPath:   execPath,
			InstanceID: reconciliationLoopID,
			Terraform:  terraformBytes,
		})
	if err != nil {
		err = fmt.Errorf("error running NewTerraform: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecNewFailedReason,
			err.Error(),
		), tfInstance, tmpDir, err
	}

	tfInstance = newTerraformReply.Id
	envs := map[string]string{}

	for _, env := range terraform.Spec.RunnerPodTemplate.Spec.Env {
		if env.ValueFrom != nil {
			var err error

			if env.ValueFrom.SecretKeyRef != nil {
				secret := corev1.Secret{}
				err = r.Client.Get(ctx, types.NamespacedName{
					Namespace: terraform.GetObjectMeta().GetNamespace(),
					Name:      env.ValueFrom.SecretKeyRef.Name,
				}, &secret)
				envs[env.Name] = string(secret.Data[env.ValueFrom.SecretKeyRef.Key])
			} else if env.ValueFrom.ConfigMapKeyRef != nil {
				cm := corev1.ConfigMap{}
				err = r.Client.Get(ctx, types.NamespacedName{
					Namespace: terraform.GetObjectMeta().GetNamespace(),
					Name:      env.ValueFrom.ConfigMapKeyRef.Name,
				}, &cm)
				envs[env.Name] = string(cm.Data[env.ValueFrom.ConfigMapKeyRef.Key])
			}

			if err != nil {
				err = fmt.Errorf("error getting valuesFrom document for Terraform: %s", err)
				return infrav1.TerraformNotReady(
					terraform,
					revision,
					infrav1.TFExecInitFailedReason,
					err.Error(),
				), tfInstance, tmpDir, err
			}
		} else {
			envs[env.Name] = env.Value
		}
	}

	disableTestLogging := os.Getenv("DISABLE_TF_LOGS") == "1"
	if !disableTestLogging {
		envs["DISABLE_TF_LOGS"] = "1"
	}

	if tfrcFilepath != "" {
		envs["TF_CLI_CONFIG_FILE"] = tfrcFilepath
	}

	// SetEnv returns a nil for the first return values if there is an error, so
	// let's ignore that as it's not used elsewhere.
	if _, err := runnerClient.SetEnv(ctx,
		&runner.SetEnvRequest{
			TfInstance: tfInstance,
			Envs:       envs,
		}); err != nil {
		err = fmt.Errorf("error setting env for Terraform: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecInitFailedReason,
			err.Error(),
		), tfInstance, tmpDir, err
	}

	if len(terraform.Spec.FileMappings) > 0 {
		log.Info("generate runner mapping files")
		runnerFileMappingList, err := r.createRunnerFileMapping(ctx, terraform)
		if err != nil {
			err = fmt.Errorf("error creating runner file mappings: %w", err)
			return infrav1.TerraformNotReady(
				terraform,
				revision,
				infrav1.TFExecInitFailedReason,
				err.Error(),
			), tfInstance, tmpDir, err
		}

		log.Info("create mapping files")
		if _, err := runnerClient.CreateFileMappings(ctx, &runner.CreateFileMappingsRequest{
			WorkingDir:   workingDir,
			FileMappings: runnerFileMappingList,
		}); err != nil {
			err = fmt.Errorf("error creating file mappings for Terraform: %w", err)
			return infrav1.TerraformNotReady(
				terraform,
				revision,
				infrav1.TFExecInitFailedReason,
				err.Error(),
			), tfInstance, tmpDir, err
		}
	}

	generateVarsForTFReply, err := runnerClient.GenerateVarsForTF(ctx, &runner.GenerateVarsForTFRequest{
		WorkingDir: workingDir,
	})
	if err != nil {
		// transient error?
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.VarsGenerationFailedReason,
			err.Error(),
		), tfInstance, tmpDir, err
	}
	log.Info(fmt.Sprintf("generate vars from tf: %s", generateVarsForTFReply.Message))

	log.Info("generated var files from spec")

	generateTemplateReply, err := runnerClient.GenerateTemplate(ctx, &runner.GenerateTemplateRequest{
		WorkingDir: workingDir,
	})
	if err != nil {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TemplateGenerationFailedReason,
			err.Error(),
		), tfInstance, tmpDir, err
	}
	log.Info(fmt.Sprintf("generate template: %s", generateTemplateReply.Message))

	log.Info("generated template")

	// TODO we currently use a fork version of TFExec to workaround the forceCopy bug
	// https://github.com/hashicorp/terraform-exec/issues/262

	initRequest := &runner.InitRequest{
		TfInstance: tfInstance,
		Upgrade:    terraform.Spec.UpgradeOnInit,
		ForceCopy:  true,
		// Terraform:  terraformBytes,
	}
	if r.backendCompletelyDisable(terraform) {
		initRequest.ForceCopy = false
	}

	initReply, err := runnerClient.Init(ctx, initRequest)
	if err != nil {
		if st, ok := status.FromError(err); ok {
			for _, detail := range st.Details() {
				if reply, ok := detail.(*runner.InitReply); ok {
					terraform = infrav1.TerraformStateLocked(terraform, reply.StateLockIdentifier, fmt.Sprintf("Terraform Locked with Lock Identifier: %s", reply.StateLockIdentifier))
				}
			}
		}

		err = fmt.Errorf("error running Init: %s", err)

		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecInitFailedReason,
			err.Error(),
		), tfInstance, tmpDir, err
	}
	log.Info(fmt.Sprintf("init reply: %s", initReply.Message))

	log.Info("tfexec initialized terraform")

	workspaceRequest := &runner.WorkspaceRequest{
		TfInstance: tfInstance,
		// Terraform:  terraformBytes,
	}
	workspaceReply, err := runnerClient.SelectWorkspace(ctx, workspaceRequest)
	if err != nil {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.WorkspaceSelectFailedReason,
			err.Error(),
		), tfInstance, tmpDir, err
	}

	log.Info(fmt.Sprintf("workspace select reply: %s", workspaceReply.Message))

	// This variable is going to be used to force unlock the state if it is locked
	lockIdentifier := ""

	// If we have a lock id we want to force unlock the state
	if terraform.Spec.TFState != nil {
		if terraform.Spec.TFState.ForceUnlock == infrav1.ForceUnlockEnumYes && terraform.Spec.TFState.LockIdentifier == terraform.Status.Lock.Pending {
			lockIdentifier = terraform.Status.Lock.Pending
		} else if terraform.Spec.TFState.ForceUnlock == infrav1.ForceUnlockEnumAuto {
			lockIdentifier = terraform.Status.Lock.Pending
		}
	}

	// If we have a lock id need to force unlock it
	if lockIdentifier != "" {
		_, err := runnerClient.ForceUnlock(context.Background(), &runner.ForceUnlockRequest{
			LockIdentifier: lockIdentifier,
		})

		if err != nil {
			return terraform, tfInstance, tmpDir, err
		}

		terraform = infrav1.TerraformForceUnlock(terraform, fmt.Sprintf("Terraform Force Unlock with Lock Identifier: %s", lockIdentifier))

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status before Terraform force unlock")
			return terraform, tfInstance, tmpDir, err
		}
	}

	return terraform, tfInstance, tmpDir, nil
}

func getLabelsAsHCL(labels map[string]string, indent int) string {
	var result string
	for k, v := range labels {
		// print space for indentation
		for i := 0; i < indent; i++ {
			result += " "
		}
		result = result + fmt.Sprintf("%q = %q\n", k, v)
	}

	return strings.TrimSpace(result)
}
