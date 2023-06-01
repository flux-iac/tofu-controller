package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/fluxcd/pkg/untar"
	"github.com/go-logr/logr"
	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"github.com/weaveworks/tf-controller/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

type LocalPrintfer struct {
	logger logr.Logger
}

func (l LocalPrintfer) Printf(format string, v ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, v...))
}

type TerraformRunnerServer struct {
	UnimplementedRunnerServer
	tf *tfexec.Terraform
	client.Client
	Scheme     *runtime.Scheme
	Done       chan os.Signal
	terraform  *infrav1.Terraform
	InstanceID string
}

const loggerName = "runner.terraform"

func (r *TerraformRunnerServer) LookPath(ctx context.Context, req *LookPathRequest) (*LookPathReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("looking for path", "file", req.File)
	execPath, err := exec.LookPath(req.File)
	if err != nil {
		log.Error(err, "unable to look for path", "file", req.File)
		return nil, err
	}
	return &LookPathReply{ExecPath: execPath}, nil
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
	if _, err = untar.Untar(buf, tmpDir); err != nil {
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
	var cliConfig corev1.Secret
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
	r.tf = tf

	var terraform infrav1.Terraform
	if err := terraform.FromBytes(req.Terraform, r.Scheme); err != nil {
		log.Error(err, "there was a problem getting the terraform resource")
		return nil, err
	}
	// cache the Terraform resource when initializing
	r.terraform = &terraform

	disableTestLogging := os.Getenv("DISABLE_TF_LOGS") == "1"
	if !disableTestLogging {
		r.tf.SetStdout(os.Stdout)
		r.tf.SetStderr(os.Stderr)
		if os.Getenv("ENABLE_SENSITIVE_TF_LOGS") == "1" {
			r.tf.SetLogger(&LocalPrintfer{logger: log})
		}
	}

	return &NewTerraformReply{Id: r.InstanceID}, nil
}

func (r *TerraformRunnerServer) SetEnv(ctx context.Context, req *SetEnvRequest) (*SetEnvReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("setting envvars")

	if req.TfInstance != r.InstanceID {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	log.Info("getting envvars from os environments")
	envs := utils.EnvMap(os.Environ())
	for k, v := range req.Envs {
		envs[k] = v
	}
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

func (r *TerraformRunnerServer) Init(ctx context.Context, req *InitRequest) (*InitReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("initializing")
	if req.TfInstance != r.InstanceID {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	terraform := r.terraform

	log.Info("mapping the Spec.BackendConfigsFrom")
	backendConfigsOpts := []tfexec.InitOption{}
	for _, bf := range terraform.Spec.BackendConfigsFrom {
		objectKey := types.NamespacedName{
			Namespace: terraform.Namespace,
			Name:      bf.Name,
		}
		if bf.Kind == "Secret" {
			var s corev1.Secret
			err := r.Get(ctx, objectKey, &s)
			if err != nil && bf.Optional == false {
				log.Error(err, "unable to get object key", "objectKey", objectKey, "secret", s.ObjectMeta.Name)
				return nil, err
			}
			// if VarsKeys is null, use all
			if bf.Keys == nil {
				for key, val := range s.Data {
					backendConfigsOpts = append(backendConfigsOpts, tfexec.BackendConfig(key+"="+string(val)))
				}
			} else {
				for _, key := range bf.Keys {
					backendConfigsOpts = append(backendConfigsOpts, tfexec.BackendConfig(key+"="+string(s.Data[key])))
				}
			}
		} else if bf.Kind == "ConfigMap" {
			var cm corev1.ConfigMap
			err := r.Get(ctx, objectKey, &cm)
			if err != nil && bf.Optional == false {
				log.Error(err, "unable to get object key", "objectKey", objectKey, "configmap", cm.ObjectMeta.Name)
				return nil, err
			}

			// if Keys is null, use all
			if bf.Keys == nil {
				for key, val := range cm.Data {
					backendConfigsOpts = append(backendConfigsOpts, tfexec.BackendConfig(key+"="+val))
				}
				for key, val := range cm.BinaryData {
					backendConfigsOpts = append(backendConfigsOpts, tfexec.BackendConfig(key+"="+string(val)))
				}
			} else {
				for _, key := range bf.Keys {
					if val, ok := cm.Data[key]; ok {
						backendConfigsOpts = append(backendConfigsOpts, tfexec.BackendConfig(key+"="+val))
					}
					if val, ok := cm.BinaryData[key]; ok {
						backendConfigsOpts = append(backendConfigsOpts, tfexec.BackendConfig(key+"="+string(val)))
					}
				}
			}
		}
	}

	initOpts := []tfexec.InitOption{tfexec.Upgrade(req.Upgrade), tfexec.ForceCopy(req.ForceCopy)}
	initOpts = append(initOpts, backendConfigsOpts...)
	if err := r.tf.Init(ctx, initOpts...); err != nil {
		st := status.New(codes.Internal, err.Error())
		var stateErr *tfexec.ErrStateLocked

		if errors.As(err, &stateErr) {
			st, err = st.WithDetails(&InitReply{Message: "not ok", StateLockIdentifier: stateErr.ID})

			if err != nil {
				return nil, err
			}
		}

		log.Error(err, "unable to initialize")
		return nil, st.Err()
	}

	return &InitReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) SelectWorkspace(ctx context.Context, req *WorkspaceRequest) (*WorkspaceReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("workspace select")
	if req.TfInstance != r.InstanceID {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
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

func (r *TerraformRunnerServer) LoadTFPlan(ctx context.Context, req *LoadTFPlanRequest) (*LoadTFPlanReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("loading plan from secret")
	if req.TfInstance != r.InstanceID {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	tfplanSecretKey := types.NamespacedName{Namespace: req.Namespace, Name: "tfplan-" + r.terraform.WorkspaceName() + "-" + req.Name}
	tfplanSecret := corev1.Secret{}
	err := r.Get(ctx, tfplanSecretKey, &tfplanSecret)
	if err != nil {
		err = fmt.Errorf("error getting plan secret: %s", err)
		log.Error(err, "unable to get secret")
		return nil, err
	}

	if tfplanSecret.Annotations[SavedPlanSecretAnnotation] != req.PendingPlan {
		err = fmt.Errorf("error pending plan and plan's name in the secret are not matched: %s != %s",
			req.PendingPlan,
			tfplanSecret.Annotations[SavedPlanSecretAnnotation])
		log.Error(err, "plan name mismatch")
		return nil, err
	}

	if req.BackendCompletelyDisable {
		// do nothing
	} else {
		tfplan := tfplanSecret.Data[TFPlanName]
		tfplan, err = utils.GzipDecode(tfplan)
		if err != nil {
			log.Error(err, "unable to decode the plan")
			return nil, err
		}
		err = ioutil.WriteFile(filepath.Join(r.tf.WorkingDir(), TFPlanName), tfplan, 0644)
		if err != nil {
			err = fmt.Errorf("error saving plan file to disk: %s", err)
			log.Error(err, "unable to write the plan to disk")
			return nil, err
		}
	}

	return &LoadTFPlanReply{Message: "ok"}, nil
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

	if req.TfInstance != r.InstanceID {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	var destroyOpt []tfexec.DestroyOption
	for _, target := range req.Targets {
		destroyOpt = append(destroyOpt, tfexec.Target(target))
	}

	if err := r.tf.Destroy(ctx, destroyOpt...); err != nil {
		st := status.New(codes.Internal, err.Error())
		var stateErr *tfexec.ErrStateLocked

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

	if req.TfInstance != r.InstanceID {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	var applyOpt []tfexec.ApplyOption
	if req.DirOrPlan != "" {
		applyOpt = []tfexec.ApplyOption{tfexec.DirOrPlan(req.DirOrPlan)}
	}

	if req.RefreshBeforeApply {
		applyOpt = []tfexec.ApplyOption{tfexec.Refresh(true)}
	}

	for _, target := range req.Targets {
		applyOpt = append(applyOpt, tfexec.Target(target))
	}

	if req.Parallelism > 0 {
		applyOpt = append(applyOpt, tfexec.Parallelism(int(req.Parallelism)))
	}

	if err := r.tf.Apply(ctx, applyOpt...); err != nil {
		st := status.New(codes.Internal, err.Error())
		var stateErr *tfexec.ErrStateLocked

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
	if req.TfInstance != r.InstanceID {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "get inventory: no terraform")
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
	// nil dereference bug here
	planObjectKey := types.NamespacedName{Namespace: req.Namespace, Name: "tfplan-" + req.Workspace + "-" + req.Name}
	var planSecret corev1.Secret
	if err := r.Client.Get(ctx, planObjectKey, &planSecret); err == nil {
		if err := r.Client.Delete(ctx, &planSecret); err != nil {
			// transient failure
			log.Error(err, "unable to delete the plan secret")
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else if apierrors.IsNotFound(err) {
		log.Error(err, "plan secret not found")
		return nil, status.Error(codes.NotFound, err.Error())
	} else {
		// transient failure
		log.Error(err, "unable to get the plan secret")
		return nil, status.Error(codes.Internal, err.Error())
	}

	if req.HasSpecifiedOutputSecret {
		outputsObjectKey := types.NamespacedName{Namespace: req.Namespace, Name: req.OutputSecretName}
		var outputsSecret corev1.Secret
		if err := r.Client.Get(ctx, outputsObjectKey, &outputsSecret); err == nil {
			if err := r.Client.Delete(ctx, &outputsSecret); err != nil {
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
