package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/fluxcd/pkg/tar"
	"github.com/go-logr/logr"
	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/flux-iac/tofu-controller/api/plan"
	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/utils"
)

const (
	TFPlanName                            = "tfplan"
	SavedPlanSecretAnnotation             = "savedPlan"
	runnerFileMappingLocationHome         = "home"
	runnerFileMappingLocationWorkspace    = "workspace"
	runnerFileMappingDirectoryPermissions = 0700
	runnerFileMappingFilePermissions      = 0600
	HomePath                              = "/home/runner"
)

// TerraformSessionNotInitializedError indicates that the current Terraform instance ID is empty.
type TerraformSessionNotInitializedError struct {
	RequestedInstanceID string
}

func (e *TerraformSessionNotInitializedError) Error() string {
	return fmt.Sprintf("terraform session error: instance id is empty, expected '%s'", e.RequestedInstanceID)
}

// TerraformSessionMismatchError indicates that the requested Terraform instance ID does not match the current instance ID.
type TerraformSessionMismatchError struct {
	RequestedInstanceID string
	CurrentInstanceID   string
}

func (e *TerraformSessionMismatchError) Error() string {
	return fmt.Sprintf("terraform session mismatch: requested instance id '%s' does not match the current instance '%s'", e.RequestedInstanceID, e.CurrentInstanceID)
}

type LocalPrintfer struct {
	logger logr.Logger
}

func (l LocalPrintfer) Printf(format string, v ...any) {
	l.logger.Info(fmt.Sprintf(format, v...))
}

type TerraformRunnerServer struct {
	UnimplementedRunnerServer
	tf *TerraformExecWrapper
	client.Client
	Scheme     *runtime.Scheme
	Done       chan os.Signal
	terraform  *infrav1.Terraform
	InstanceID string
}

const loggerName = "runner.terraform"

func (r *TerraformRunnerServer) ValidateInstanceID(requestedInstanceID string) error {
	if r.InstanceID == "" {
		return &TerraformSessionNotInitializedError{
			RequestedInstanceID: requestedInstanceID,
		}
	}

	if requestedInstanceID != r.InstanceID {
		return &TerraformSessionMismatchError{
			RequestedInstanceID: requestedInstanceID,
			CurrentInstanceID:   r.InstanceID,
		}
	}

	return nil
}

func (r *TerraformRunnerServer) LookPath(ctx context.Context, req *LookPathRequest) (*LookPathReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("looking for paths", "files", req.Files)

	for _, file := range req.Files {
		execPath, err := exec.LookPath(file)
		if err == nil {
			log.Info("found binary", "file", file, "execPath", execPath)
			return &LookPathReply{ExecPath: execPath}, nil
		}
	}

	return nil, errors.New("none of the specified binaries were found in the runners PATH")
}

func (r *TerraformRunnerServer) UploadAndExtract(ctx context.Context, req *UploadAndExtractRequest) (*UploadAndExtractReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("preparing for Upload and Extraction")
	tmpDir, err := securejoin.SecureJoin(os.TempDir(), fmt.Sprintf("%s-%s", req.Namespace, req.Name))
	if err != nil {
		log.Error(err, "unable to join securely", "tmpDir", os.TempDir(), "namespace", req.Namespace, "name", req.Name)
		return nil, err
	}

	buf := bytes.NewBuffer(req.TarGz)
	opts := tar.WithMaxUntarSize(tar.UnlimitedUntarSize)
	if err = tar.Untar(buf, tmpDir, opts); err != nil {
		log.Error(err, "unable to extract tar file", "namespace", req.Namespace, "name", req.Name)
		return nil, fmt.Errorf("failed to untar artifact, error: %w", err)
	}

	dirPath, err := securejoin.SecureJoin(tmpDir, req.Path)
	if err != nil {
		log.Error(err, "unable to join securely", "tmpDir", tmpDir, "path", req.Path)
		return nil, err
	}

	if _, err := os.Stat(dirPath); err != nil {
		log.Error(err, "path not found", "dirPath", dirPath)
		err = fmt.Errorf("terraform path not found: %w", err)
		return nil, err
	}

	return &UploadAndExtractReply{WorkingDir: dirPath, TmpDir: tmpDir}, nil
}

func (r *TerraformRunnerServer) CleanupDir(ctx context.Context, req *CleanupDirRequest) (*CleanupDirReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("cleanup TmpDir", "tmpDir", req.TmpDir)
	err := os.RemoveAll(req.TmpDir)
	if err != nil {
		log.Error(err, "error cleaning up TmpDir", "tmpDir", req.TmpDir)
		return nil, err
	}

	return &CleanupDirReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) WriteBackendConfig(ctx context.Context, req *WriteBackendConfigRequest) (*WriteBackendConfigReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	const backendConfigPath = "backend_override.tf"
	log.Info("write backend config", "path", req.DirPath, "config", backendConfigPath)
	filePath, err := securejoin.SecureJoin(req.DirPath, backendConfigPath)
	if err != nil {
		log.Error(err, "unable to join securely", "dirPath", req.DirPath, "config", backendConfigPath)
		return nil, err
	}

	log.Info("write config to file", "filePath", filePath)
	err = os.WriteFile(filePath, req.BackendConfig, 0644)
	if err != nil {
		log.Error(err, "unable to write file", "filePath", filePath)
		return nil, err
	}

	return &WriteBackendConfigReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) ProcessCliConfig(ctx context.Context, req *ProcessCliConfigRequest) (*ProcessCliConfigReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("processing configuration", "namespace", req.Namespace, "name", req.Name)
	cliConfigKey := types.NamespacedName{Namespace: req.Namespace, Name: req.Name}
	var cliConfig v1.Secret
	if err := r.Get(ctx, cliConfigKey, &cliConfig); err != nil {
		log.Error(err, "unable to get secret", "namespace", req.Namespace, "name", req.Name)
		return nil, err
	}

	if len(cliConfig.Data) != 1 {
		err := fmt.Errorf("expect the secret to contain 1 data")
		log.Error(err, "secret missing data", "namespace", req.Namespace, "name", req.Name)
		return nil, err
	}

	var tfrcFilepath string
	for tfrcFilename, v := range cliConfig.Data {
		if strings.HasSuffix(tfrcFilename, ".tfrc") {
			var err error
			tfrcFilepath, err = securejoin.SecureJoin(req.DirPath, tfrcFilename)
			if err != nil {
				log.Error(err, "secure join error", "dirPath", req.DirPath, "tfrcFilename", tfrcFilename)
				return nil, err
			}
			err = os.WriteFile(tfrcFilepath, v, 0644)
			if err != nil {
				log.Error(err, "write file error", "tfrcFilename", tfrcFilename, "fileMode", "0644")
				return nil, err
			}
		} else {
			err := fmt.Errorf("expect the secret key to end with .tfrc")
			log.Error(err, "file extension error", "tfrcFilename", tfrcFilename)
			return nil, err
		}
	}
	return &ProcessCliConfigReply{FilePath: tfrcFilepath}, nil
}

// initLogger sets up the logger for the terraform runner
func (r *TerraformRunnerServer) initLogger(log logr.Logger) {
	disableTestLogging := os.Getenv("DISABLE_TF_LOGS") == "1"
	if !disableTestLogging {
		r.tf.SetStdout(os.Stdout)
		r.tf.SetStderr(os.Stderr)
		if os.Getenv("ENABLE_SENSITIVE_TF_LOGS") == "1" {
			r.tf.SetLogger(&LocalPrintfer{logger: log})
		}
	}
}

func (r *TerraformRunnerServer) NewTerraform(ctx context.Context, req *NewTerraformRequest) (*NewTerraformReply, error) {
	r.InstanceID = req.GetInstanceID()
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("creating new terraform", "workingDir", req.WorkingDir, "execPath", req.ExecPath)
	tf, err := tfexec.NewTerraform(req.WorkingDir, req.ExecPath)
	if err != nil {
		log.Error(err, "unable to create new terraform", "workingDir", req.WorkingDir, "execPath", req.ExecPath)
		return nil, err
	}

	// hold only 1 instance
	r.tf = NewTerraformExecWrapper(tf)

	var terraform infrav1.Terraform
	if err := terraform.FromBytes(req.Terraform, r.Scheme); err != nil {
		log.Error(err, "there was a problem getting the terraform resource")
		return nil, err
	}
	// cache the Terraform resource when initializing
	r.terraform = &terraform

	// init default logger
	r.initLogger(log)

	return &NewTerraformReply{Id: r.InstanceID}, nil
}

func (r *TerraformRunnerServer) SetEnv(ctx context.Context, req *SetEnvRequest) (*SetEnvReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("setting envvars")

	if err := r.ValidateInstanceID(req.TfInstance); err != nil {
		log.Error(err, "terraform session mismatch when setting environment variables")

		return nil, err
	}

	log.Info("getting envvars from os environments")
	envs := utils.EnvMap(os.Environ())
	maps.Copy(envs, req.Envs)
	err := r.tf.SetEnv(envs)
	if err != nil {
		log.Error(err, "unable to set envvars", "envvars", envs)
		return nil, err
	}

	return &SetEnvReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) CreateFileMappings(ctx context.Context, req *CreateFileMappingsRequest) (*CreateFileMappingsReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("creating file mappings")

	for _, fileMapping := range req.FileMappings {
		var fileFullPath string
		var err error
		switch fileMapping.Location {
		case runnerFileMappingLocationHome:
			fileFullPath, err = securejoin.SecureJoin(HomePath, fileMapping.Path)
			if err != nil {
				log.Error(err, "insecure file path", "path", fileMapping.Path)
				return nil, err
			}
		case runnerFileMappingLocationWorkspace:
			fileFullPath, err = securejoin.SecureJoin(req.WorkingDir, fileMapping.Path)
			if err != nil {
				log.Error(err, "insecure file path", "path", fileMapping.Path)
				return nil, err
			}
		default:
			err := fmt.Errorf("unknown file mapping location")
			log.Error(err, "unknown file mapping location", "location", fileMapping.Location)
			return nil, err
		}

		log.Info("prepare to write file", "fileFullPath", fileFullPath)
		dir := filepath.Dir(fileFullPath)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			// create dir
			err := os.MkdirAll(dir, runnerFileMappingDirectoryPermissions)
			if err != nil {
				log.Error(err, "unable to create dir", "dir", dir)
				return nil, err
			}
		}

		if err := os.WriteFile(fileFullPath, fileMapping.Content, runnerFileMappingFilePermissions); err != nil {
			log.Error(err, "Unable to create file from file mapping", "file", fileFullPath)
			return nil, err
		}
	}

	return &CreateFileMappingsReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) SelectWorkspace(ctx context.Context, req *WorkspaceRequest) (*WorkspaceReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("workspace select")

	if err := r.ValidateInstanceID(req.TfInstance); err != nil {
		log.Error(err, "terraform session mismatch when selecting workspace")

		return nil, err
	}

	terraform := r.terraform

	if terraform.WorkspaceName() != infrav1.DefaultWorkspaceName {
		wsOpts := []tfexec.WorkspaceNewCmdOption{}
		ws := terraform.Spec.Workspace
		if err := r.tf.WorkspaceNew(ctx, ws, wsOpts...); err != nil {
			log.Info(fmt.Sprintf("workspace new:, %s", err.Error()))
		}
		if err := r.tf.WorkspaceSelect(ctx, ws); err != nil {
			err := fmt.Errorf("failed to select workspace %s", ws)
			log.Error(err, "workspace select error")
			return nil, err
		}
	}

	return &WorkspaceReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) Destroy(ctx context.Context, req *DestroyRequest) (*DestroyReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("running destroy")

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-r.Done:
			cancel()
		case <-ctx.Done():
		}
	}()

	if err := r.ValidateInstanceID(req.TfInstance); err != nil {
		log.Error(err, "terraform session mismatch when running destroy")

		return nil, err
	}

	var destroyOpt []tfexec.DestroyOption
	for _, target := range req.Targets {
		destroyOpt = append(destroyOpt, tfexec.Target(target))
	}

	if err := r.tf.Destroy(ctx, destroyOpt...); err != nil {
		st := status.New(codes.Internal, err.Error())
		var stateErr *StateLockError

		if errors.As(err, &stateErr) {
			st, err = st.WithDetails(&DestroyReply{Message: "not ok", StateLockIdentifier: stateErr.ID})

			if err != nil {
				return nil, err
			}
		}

		log.Error(err, "unable to destroy")
		return nil, st.Err()
	}

	return &DestroyReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) Apply(ctx context.Context, req *ApplyRequest) (*ApplyReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("running apply")

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-r.Done:
			cancel()
		case <-ctx.Done():
		}
	}()

	if err := r.ValidateInstanceID(req.TfInstance); err != nil {
		log.Error(err, "terraform session mismatch when running apply")

		return nil, err
	}

	var applyOpt []tfexec.ApplyOption
	if req.DirOrPlan != "" {
		applyOpt = append(applyOpt, tfexec.DirOrPlan(req.DirOrPlan))
	}
	if req.RefreshBeforeApply {
		applyOpt = append(applyOpt, tfexec.Refresh(true))
	}
	for _, target := range req.Targets {
		applyOpt = append(applyOpt, tfexec.Target(target))
	}
	if req.Parallelism > 0 {
		applyOpt = append(applyOpt, tfexec.Parallelism(int(req.Parallelism)))
	}

	if err := printHumanReadablePlanIfEnabled(ctx, req.DirOrPlan, r.tfShowPlanFileRaw); err != nil {
		log.Error(err, "unable to print plan")
		return nil, err
	}

	if err := r.tf.Apply(ctx, applyOpt...); err != nil {
		st := status.New(codes.Internal, err.Error())
		var stateErr *StateLockError

		if errors.As(err, &stateErr) {
			st, err = st.WithDetails(&ApplyReply{Message: "not ok", StateLockIdentifier: stateErr.ID})

			if err != nil {
				return nil, err
			}
		}

		log.Error(err, "unable to apply plan")
		return nil, st.Err()
	}

	return &ApplyReply{Message: "ok"}, nil
}

func printHumanReadablePlanIfEnabled(ctx context.Context, planName string, tfShowPlanFileRaw func(ctx context.Context, planPath string, opts ...tfexec.ShowOption) (string, error)) error {
	if os.Getenv("LOG_HUMAN_READABLE_PLAN") == "1" {
		if planName == "" {
			planName = TFPlanName
		}

		rawOutput, err := tfShowPlanFileRaw(ctx, planName)
		if err != nil {
			return err
		}

		fmt.Println(rawOutput)
	}

	return nil
}

func getInventoryFromTerraformModule(m *tfjson.StateModule) []*Inventory {
	var result []*Inventory
	for _, r := range m.Resources {
		var id string
		// TODO ARN is AWS-specific. Will need to support other cloud identifiers in the future.
		if val, ok := r.AttributeValues["arn"]; ok {
			id = val.(string)
		} else if val, ok := r.AttributeValues["id"]; ok {
			id = val.(string)
		}
		result = append(result, &Inventory{
			Name:       r.Name,
			Type:       r.Type,
			Identifier: id,
		})
	}

	// recursively get all resources from submodules
	for _, childModule := range m.ChildModules {
		childInventory := getInventoryFromTerraformModule(childModule)
		result = append(result, childInventory...)
	}

	return result
}

func (r *TerraformRunnerServer) GetInventory(ctx context.Context, req *GetInventoryRequest) (*GetInventoryReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("get inventory")

	if err := r.ValidateInstanceID(req.TfInstance); err != nil {
		log.Error(err, "terraform session mismatch when getting inventory")

		return nil, err
	}

	state, err := r.tf.Show(ctx)
	if err != nil {
		log.Error(err, "get inventory: unable to get state via show command")
		return nil, err
	}

	// state contains no values after resource destruction for example
	if state.Values == nil {
		log.Info("get inventory: state values is nil")
		return &GetInventoryReply{Inventories: []*Inventory{}}, nil
	}

	if state.Values.RootModule == nil {
		log.Info("get inventory: root module is nil")
		return &GetInventoryReply{Inventories: []*Inventory{}}, nil
	}

	return &GetInventoryReply{Inventories: getInventoryFromTerraformModule(state.Values.RootModule)}, nil
}

func (r *TerraformRunnerServer) FinalizeSecrets(ctx context.Context, req *FinalizeSecretsRequest) (*FinalizeSecretsReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("finalize the output secrets")

	secrets := &v1.SecretList{}

	if err := r.List(ctx, secrets, client.InNamespace(req.Namespace), client.MatchingLabels{
		plan.TFPlanNameLabel:      plan.SafeLabelValue(req.Name),
		plan.TFPlanWorkspaceLabel: req.Workspace,
	}); err != nil {
		log.Error(err, "unable to list existing plan secrets")
		return nil, err
	}

	if len(secrets.Items) == 0 {
		var legacyPlanSecret v1.Secret

		if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: "tfplan-" + req.Workspace + "-" + req.Name}, &legacyPlanSecret); err == nil {
			secrets.Items = append(secrets.Items, legacyPlanSecret)
		}
	}

	for _, s := range secrets.Items {
		if err := r.Delete(ctx, &s); err != nil {
			log.Error(err, "unable to delete existing plan secret", "secretName", s.Name)
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if len(secrets.Items) == 0 {
		return nil, status.Error(codes.NotFound, "no existing plan secrets found to delete")
	}

	if req.HasSpecifiedOutputSecret {
		outputsObjectKey := types.NamespacedName{Namespace: req.Namespace, Name: req.OutputSecretName}
		var outputsSecret v1.Secret
		if err := r.Get(ctx, outputsObjectKey, &outputsSecret); err == nil {
			if err := r.Delete(ctx, &outputsSecret); err != nil {
				// transient failure
				log.Error(err, "unable to delete the output secret")
				return nil, status.Error(codes.Internal, err.Error())
			}
		} else if apierrors.IsNotFound(err) {
			log.Error(err, "output secret not found")
			return nil, status.Error(codes.NotFound, err.Error())
		} else {
			// transient failure
			log.Error(err, "unable to get the output secret")
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &FinalizeSecretsReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) ForceUnlock(ctx context.Context, req *ForceUnlockRequest) (*ForceUnlockReply, error) {
	reply := &ForceUnlockReply{
		Success: true,
		Message: fmt.Sprintf("Successfully unlocked state with lock identifier: %s", req.GetLockIdentifier()),
	}
	err := r.tf.ForceUnlock(ctx, req.LockIdentifier)

	if err != nil {
		reply.Success = false
		reply.Message = fmt.Sprintf("Error unlocking the state: %s", err)
	}

	return reply, err
}
