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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *TerraformReconciler) backendCompletelyDisable(terraform *infrav1.Terraform) bool {
	return resolveBackend(terraform, os.Getenv("DISABLE_TF_K8S_BACKEND") == "1").completelyDisabled()
}

// setup prepares the runner for this pass: it patches the
// "Initializing" checkpoint, uploads the artifact, writes the backend config,
// creates the terraform instance from the object as patched, and runs init and
// workspace selection. It sets s.tmpDir as soon as the artifact is extracted
// and s.tfInstance once NewTerraform succeeds; both are set even when a later
// RPC fails.
func (r *TerraformReconciler) setup(ctx context.Context, s *runnerSession, terraform *infrav1.Terraform, sourceObj sourcev1.Source) error {
	log := ctrl.LoggerFrom(ctx)

	// Bound the runner RPCs below (UploadAndExtract ... Init ... SelectWorkspace).
	// The runner gRPC client is created with waitForReady:true, so an RPC on a
	// channel that cannot connect (e.g. the runner pod was killed by a node reboot
	// mid-reconcile) blocks until its context is done. Without a deadline the call
	// blocks forever, and because controller-runtime does not start a new Reconcile
	// for a key while the previous one is still running, that Terraform object is
	// never reconciled again and a worker slot is leaked permanently. A generous,
	// configurable ceiling turns "never" into "requeue after RunnerRPCTimeout".
	ctx, cancel := context.WithTimeout(ctx, r.RunnerRPCTimeout)
	defer cancel()

	s.tfInstance = "0"
	s.tmpDir = ""

	infrav1.TerraformProgressing(terraform, "Initializing")
	if err := r.progress(ctx, s, terraform, "unable to update status before Terraform initialization"); err != nil {
		return err
	}

	// download artifact and extract files
	buf, err := r.downloadAsBytes(sourceObj.GetArtifact())
	if err != nil {
		return notReady(terraform, s.revision, infrav1.ArtifactFailedReason, err)
	}

	uploadAndExtractReply, err := s.client.UploadAndExtract(ctx, &runner.UploadAndExtractRequest{
		Namespace: terraform.Namespace,
		Name:      terraform.Name,
		TarGz:     buf.Bytes(),
		Path:      terraform.Spec.Path,
	})
	if err != nil {
		return notReady(terraform, s.revision, infrav1.ArtifactFailedReason, err)
	}
	workingDir := uploadAndExtractReply.WorkingDir
	s.tmpDir = uploadAndExtractReply.TmpDir

	DisableTFK8SBackend := os.Getenv("DISABLE_TF_K8S_BACKEND") == "1"
	// One resolution of the backend configuration drives both the HCL the
	// runner executes and the locator published in status, so the two cannot
	// describe different things.
	backend := resolveBackend(terraform, DisableTFK8SBackend)
	backendConfig, writeBackendConfig := backend.HCL()

	if r.backendCompletelyDisable(terraform) {
		log.Info("backendConfig is completely disabled. When Spec.Cloud is not nil, backendConfig is disabled by default too.")
	}

	if writeBackendConfig {
		configKind := "backend"
		if terraform.Spec.Cloud != nil {
			log.Info("Spec.Cloud is not nil. backendConfig is disabled by default.")
			configKind = "cloud"
		}
		writeBackendConfigReply, err := s.client.WriteBackendConfig(ctx,
			&runner.WriteBackendConfigRequest{
				DirPath:       workingDir,
				BackendConfig: []byte(backendConfig),
			})
		if err != nil {
			log.Error(err, "write config error", "kind", configKind)
			return err
		}
		log.Info("write config", "kind", configKind, "message", writeBackendConfigReply.Message)
	}

	setStateArtifactRef(terraform, backend.StateLocator(terraform))

	var tfrcFilepath string
	if terraform.Spec.CliConfigSecretRef != nil {
		cliConfigSecretRef := *(terraform.Spec.CliConfigSecretRef.DeepCopy())
		if cliConfigSecretRef.Namespace == "" {
			cliConfigSecretRef.Namespace = terraform.Namespace
		}

		if r.NoCrossNamespaceRefs && cliConfigSecretRef.Namespace != terraform.GetNamespace() {
			msg := fmt.Sprintf("cannot access secret %s/%s, cross-namespace references have been disabled", cliConfigSecretRef.Namespace, cliConfigSecretRef.Name)
			return notReady(terraform, s.revision, infrav1.AccessDeniedReason, acl.AccessDeniedError(msg))
		}

		processCliConfigReply, err := s.client.ProcessCliConfig(ctx, &runner.ProcessCliConfigRequest{
			DirPath:   workingDir,
			Namespace: cliConfigSecretRef.Namespace,
			Name:      cliConfigSecretRef.Name,
		})
		if err != nil {
			err = fmt.Errorf("cannot process cli config: %s", err.Error())
			return notReady(terraform, s.revision, infrav1.TFExecNewFailedReason, err)
		}
		tfrcFilepath = processCliConfigReply.FilePath
	}

	// The priority is to use the Terraform binary first, and then fall back to OpenTofu
	// The Terraform binary is not included in the latest runner images, so standard behaviour is to use OpenTofu
	lookPathReply, err := s.client.LookPath(ctx,
		&runner.LookPathRequest{
			Files: []string{"terraform", "tofu"},
		},
	)
	if err != nil {
		err = fmt.Errorf("cannot find any of the required binaries (terraform, tofu): %s", err)

		return notReady(terraform, s.revision, infrav1.TFExecNewFailedReason, err)
	}

	execPath := lookPathReply.ExecPath

	log.Info("new terraform", "workingDir", workingDir)

	terraformBytes, err := terraform.ToBytes(r.Scheme)
	if err != nil {
		// transient error?
		return err
	}

	newTerraformReply, err := s.client.NewTerraform(ctx,
		&runner.NewTerraformRequest{
			WorkingDir: workingDir,
			ExecPath:   execPath,
			InstanceID: s.instanceID,
			Terraform:  terraformBytes,
		})
	if err != nil {
		err = fmt.Errorf("error running NewTerraform: %s", err)
		return notReady(terraform, s.revision, infrav1.TFExecNewFailedReason, err)
	}

	s.tfInstance = newTerraformReply.Id
	envs := map[string]string{}

	for _, env := range terraform.Spec.RunnerPodTemplate.Spec.Env {
		if env.ValueFrom != nil {
			var err error

			if env.ValueFrom.SecretKeyRef != nil {
				secret := corev1.Secret{}
				err = r.Get(ctx, types.NamespacedName{
					Namespace: terraform.GetObjectMeta().GetNamespace(),
					Name:      env.ValueFrom.SecretKeyRef.Name,
				}, &secret)
				envs[env.Name] = string(secret.Data[env.ValueFrom.SecretKeyRef.Key])
			} else if env.ValueFrom.ConfigMapKeyRef != nil {
				cm := corev1.ConfigMap{}
				err = r.Get(ctx, types.NamespacedName{
					Namespace: terraform.GetObjectMeta().GetNamespace(),
					Name:      env.ValueFrom.ConfigMapKeyRef.Name,
				}, &cm)
				envs[env.Name] = string(cm.Data[env.ValueFrom.ConfigMapKeyRef.Key])
			}

			if err != nil {
				err = fmt.Errorf("error getting valuesFrom document for Terraform: %s", err)
				return notReady(terraform, s.revision, infrav1.TFExecInitFailedReason, err)
			}
		} else {
			envs[env.Name] = env.Value
		}
	}

	if tfrcFilepath != "" {
		envs["TF_CLI_CONFIG_FILE"] = tfrcFilepath
	}

	// SetEnv returns a nil for the first return values if there is an error, so
	// let's ignore that as it's not used elsewhere.
	if _, err := s.client.SetEnv(ctx,
		&runner.SetEnvRequest{
			TfInstance: s.tfInstance,
			Envs:       envs,
		}); err != nil {
		err = fmt.Errorf("error setting env for Terraform: %s", err)
		return notReady(terraform, s.revision, infrav1.TFExecInitFailedReason, err)
	}

	if len(terraform.Spec.FileMappings) > 0 {
		log.Info("generate runner mapping files")
		runnerFileMappingList, err := r.createRunnerFileMapping(ctx, terraform)
		if err != nil {
			err = fmt.Errorf("error creating runner file mappings: %w", err)
			return notReady(terraform, s.revision, infrav1.TFExecInitFailedReason, err)
		}

		log.Info("create mapping files")
		if _, err := s.client.CreateFileMappings(ctx, &runner.CreateFileMappingsRequest{
			WorkingDir:   workingDir,
			FileMappings: runnerFileMappingList,
		}); err != nil {
			err = fmt.Errorf("error creating file mappings for Terraform: %w", err)
			return notReady(terraform, s.revision, infrav1.TFExecInitFailedReason, err)
		}
	}

	generateVarsForTFReply, err := s.client.GenerateVarsForTF(ctx, &runner.GenerateVarsForTFRequest{
		TfInstance: s.tfInstance,
		WorkingDir: workingDir,
	})
	if err != nil {
		// transient error?
		return notReady(terraform, s.revision, infrav1.VarsGenerationFailedReason, err)
	}
	log.Info("generate vars from tf", "message", generateVarsForTFReply.Message)

	log.Info("generated var files from spec")

	generateTemplateReply, err := s.client.GenerateTemplate(ctx, &runner.GenerateTemplateRequest{
		TfInstance: s.tfInstance,
		WorkingDir: workingDir,
	})
	if err != nil {
		return notReady(terraform, s.revision, infrav1.TemplateGenerationFailedReason, err)
	}
	log.Info("generate template", "message", generateTemplateReply.Message)

	log.Info("generated template")

	// TODO we currently use a fork version of TFExec to workaround the forceCopy bug
	// https://github.com/hashicorp/terraform-exec/issues/262

	initRequest := &runner.InitRequest{
		TfInstance: s.tfInstance,
		Upgrade:    terraform.Spec.UpgradeOnInit,
		ForceCopy:  true,
		// Terraform:  terraformBytes,
	}
	if r.backendCompletelyDisable(terraform) {
		initRequest.ForceCopy = false
	}

	initReply, err := s.client.Init(ctx, initRequest)
	if err != nil {
		if lock, ok := stateLockFrom(err); ok {
			infrav1.TerraformStateLocked(terraform, lock.Id, fmt.Sprintf("Terraform Locked with Lock Identifier: %s", lock.Id))
		}

		err = fmt.Errorf("error running Init: %s", err)

		return notReady(terraform, s.revision, infrav1.TFExecInitFailedReason, err)
	}
	log.Info("init reply", "message", initReply.Message)

	log.Info("tfexec initialized terraform")

	workspaceRequest := &runner.WorkspaceRequest{
		TfInstance: s.tfInstance,
		// Terraform:  terraformBytes,
	}
	workspaceReply, err := s.client.SelectWorkspace(ctx, workspaceRequest)
	if err != nil {
		return notReady(terraform, s.revision, infrav1.WorkspaceSelectFailedReason, err)
	}

	log.Info("workspace select reply", "message", workspaceReply.Message)

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
		_, err := s.client.ForceUnlock(ctx, &runner.ForceUnlockRequest{
			TfInstance:     s.tfInstance,
			LockIdentifier: lockIdentifier,
		})

		// A lock that is no longer held (its holder released it meanwhile)
		// needs no unlocking, so it counts as unlocked. A lock now held by
		// another holder is recorded like any held lock: it becomes the
		// pending lock, and forceUnlock decides about it on the next pass.
		if err != nil && !isNotLocked(err) {
			if lock, ok := stateLockFrom(err); ok {
				infrav1.TerraformStateLocked(terraform, lock.Id, fmt.Sprintf("Terraform Locked with Lock Identifier: %s", lock.Id))

				err = fmt.Errorf("error running ForceUnlock: %s", err)

				return notReady(terraform, s.revision, infrav1.TFExecLockHeldReason, err)
			}

			return err
		}

		infrav1.TerraformForceUnlock(terraform, fmt.Sprintf("Terraform Force Unlock with Lock Identifier: %s", lockIdentifier))

		// Patch now, because users can observe this intermediate state:
		// StateLocked False/ForceUnlock only exists until the next plan
		// releases the lock.
		if err := r.patchStatus(ctx, s.patcher, terraform, "unable to update status before Terraform force unlock"); err != nil {
			return err
		}
	}

	return nil
}

// backendConfigFor returns the backend (or cloud) HCL that setup
// writes into the working directory, and write=false when nothing
// is written: the backend is completely disabled and spec.cloud is not set.
// disableK8SBackend carries DISABLE_TF_K8S_BACKEND=1, read by the caller.
func backendConfigFor(terraform *infrav1.Terraform, disableK8SBackend bool) (hcl string, write bool) {
	return resolveBackend(terraform, disableK8SBackend).HCL()
}

func getLabelsAsHCL(labels map[string]string, indent int) string {
	var result string
	for k, v := range labels {
		// print space for indentation
		for range indent {
			result += " "
		}
		result = result + fmt.Sprintf("%q = %q\n", k, v)
	}

	return strings.TrimSpace(result)
}
