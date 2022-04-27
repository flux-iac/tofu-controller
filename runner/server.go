package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/fluxcd/pkg/untar"
	"github.com/hashicorp/terraform-exec/tfexec"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"github.com/weaveworks/tf-controller/utils"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	TFPlanName                = "tfplan"
	SavedPlanSecretAnnotation = "savedPlan"
)

type TerraformRunnerServer struct {
	UnimplementedRunnerServer
	tf *tfexec.Terraform
	client.Client
	Scheme *runtime.Scheme
	Done   chan os.Signal
}

func (r *TerraformRunnerServer) LookPath(ctx context.Context, req *LookPathRequest) (*LookPathReply, error) {
	execPath, err := exec.LookPath(req.File)
	if err != nil {
		return nil, err
	}
	return &LookPathReply{ExecPath: execPath}, nil
}

func (r *TerraformRunnerServer) UploadAndExtract(ctx context.Context, req *UploadAndExtractRequest) (*UploadAndExtractReply, error) {
	tmpDir, err := securejoin.SecureJoin(os.TempDir(), fmt.Sprintf("%s-%s", req.Namespace, req.Name))
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(req.TarGz)
	if _, err = untar.Untar(buf, tmpDir); err != nil {
		return nil, fmt.Errorf("failed to untar artifact, error: %w", err)
	}

	dirPath, err := securejoin.SecureJoin(tmpDir, req.Path)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(dirPath); err != nil {
		err = fmt.Errorf("terraform path not found: %w", err)
		return nil, err
	}

	return &UploadAndExtractReply{WorkingDir: dirPath, TmpDir: tmpDir}, nil
}

func (r *TerraformRunnerServer) CleanupDir(ctx context.Context, req *CleanupDirRequest) (*CleanupDirReply, error) {
	err := os.RemoveAll(req.TmpDir)
	if err != nil {
		return nil, err
	}

	return &CleanupDirReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) WriteBackendConfig(ctx context.Context, req *WriteBackendConfigRequest) (*WriteBackendConfigReply, error) {
	const backendConfigPath = "generated_backend_config.tf"
	filePath, err := securejoin.SecureJoin(req.DirPath, backendConfigPath)
	if err != nil {
		return nil, err
	}
	err = os.WriteFile(filePath, req.BackendConfig, 0644)
	if err != nil {
		return nil, err
	}

	return &WriteBackendConfigReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) ProcessCliConfig(ctx context.Context, req *ProcessCliConfigRequest) (*ProcessCliConfigReply, error) {
	cliConfigKey := types.NamespacedName{Namespace: req.Namespace, Name: req.Name}
	var cliConfig corev1.Secret
	if err := r.Get(ctx, cliConfigKey, &cliConfig); err != nil {
		return nil, err
	}

	if len(cliConfig.Data) != 1 {
		err := fmt.Errorf("expect the secret to contain 1 data")
		return nil, err
	}

	var tfrcFilepath string
	for tfrcFilename, v := range cliConfig.Data {
		if strings.HasSuffix(tfrcFilename, ".tfrc") {
			var err error
			tfrcFilepath, err = securejoin.SecureJoin(req.DirPath, tfrcFilename)
			if err != nil {
				return nil, err
			}
			err = os.WriteFile(tfrcFilepath, v, 0644)
			if err != nil {
				return nil, err
			}
		} else {
			err := fmt.Errorf("expect the secret key to end with .tfrc")
			return nil, err
		}
	}
	return &ProcessCliConfigReply{FilePath: tfrcFilepath}, nil
}

func (r *TerraformRunnerServer) NewTerraform(ctx context.Context, req *NewTerraformRequest) (*NewTerraformReply, error) {
	tf, err := tfexec.NewTerraform(req.WorkingDir, req.ExecPath)
	if err != nil {
		return nil, err
	}

	// hold only 1 instance
	r.tf = tf
	return &NewTerraformReply{Id: "1"}, nil
}

func (r *TerraformRunnerServer) SetEnv(ctx context.Context, req *SetEnvRequest) (*SetEnvReply, error) {
	if req.TfInstance != "1" {
		return nil, fmt.Errorf("no TF instance found")
	}

	envs := utils.EnvMap(os.Environ())
	for k, v := range req.Envs {
		envs[k] = v
	}
	err := r.tf.SetEnv(envs)
	if err != nil {
		return nil, err
	}

	disableTestLogging := envs["DISABLE_TF_LOGS"] == "1"
	if !disableTestLogging {
		r.tf.SetStdout(os.Stdout)
		r.tf.SetStderr(os.Stderr)
		// TODO enable TF logger
		// r.tf.SetLogger(&LocalPrintfer{logger: log})
	}

	return &SetEnvReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) Init(ctx context.Context, req *InitRequest) (*InitReply, error) {
	if req.TfInstance != "1" {
		return nil, fmt.Errorf("no TF instance found")
	}

	initOpts := []tfexec.InitOption{tfexec.Upgrade(req.Upgrade), tfexec.ForceCopy(req.ForceCopy)}
	err := r.tf.Init(ctx, initOpts...)
	if err != nil {
		return nil, err
	}

	return &InitReply{Message: "ok"}, nil
}

// GenerateVarsForTF renders the Terraform variables as a json file for the given inputs
// variables supplied in the varsFrom field will override those specified in the spec
func (r *TerraformRunnerServer) GenerateVarsForTF(ctx context.Context, req *GenerateVarsForTFRequest) (*GenerateVarsForTFReply, error) {
	var terraform infrav1.Terraform
	err := terraform.FromBytes(req.Terraform, r.Scheme)
	if err != nil {
		return nil, err
	}

	vars := map[string]*apiextensionsv1.JSON{}
	if len(terraform.Spec.Vars) > 0 {
		for _, v := range terraform.Spec.Vars {
			vars[v.Name] = v.Value
		}
	}

	// varsFrom overwrite vars
	for _, vf := range terraform.Spec.VarsFrom {
		objectKey := types.NamespacedName{
			Namespace: terraform.Namespace,
			Name:      vf.Name,
		}
		if vf.Kind == "Secret" {
			var s corev1.Secret
			err := r.Get(ctx, objectKey, &s)
			if err != nil && vf.Optional == false {
				return nil, err
			}
			// if VarsKeys is null, use all
			if vf.VarsKeys == nil {
				for key, val := range s.Data {
					vars[key], err = utils.JSONEncodeBytes(val)
					if err != nil {
						return nil, fmt.Errorf("failed to encode key %s with error: %w", key, err)
					}
				}
			} else {
				for _, key := range vf.VarsKeys {
					vars[key], err = utils.JSONEncodeBytes(s.Data[key])
					if err != nil {
						return nil, fmt.Errorf("failed to encode key %s with error: %w", key, err)
					}
				}
			}
		} else if vf.Kind == "ConfigMap" {
			var cm corev1.ConfigMap
			err := r.Get(ctx, objectKey, &cm)
			if err != nil && vf.Optional == false {
				return nil, err
			}

			// if VarsKeys is null, use all
			if vf.VarsKeys == nil {
				for key, val := range cm.Data {
					vars[key], err = utils.JSONEncodeBytes([]byte(val))
					if err != nil {
						return nil, fmt.Errorf("failed to encode key %s with error: %w", key, err)
					}
				}
				for key, val := range cm.BinaryData {
					vars[key], err = utils.JSONEncodeBytes(val)
					if err != nil {
						return nil, fmt.Errorf("failed to encode key %s with error: %w", key, err)
					}
				}
			} else {
				for _, key := range vf.VarsKeys {
					if val, ok := cm.Data[key]; ok {
						vars[key], err = utils.JSONEncodeBytes([]byte(val))
						if err != nil {
							return nil, fmt.Errorf("failed to encode key %s with error: %w", key, err)
						}
					}
					if val, ok := cm.BinaryData[key]; ok {
						vars[key], err = utils.JSONEncodeBytes(val)
						if err != nil {
							return nil, err
						}
					}
				}
			}
		}
	}

	jsonBytes, err := json.Marshal(vars)
	if err != nil {
		return nil, err
	}

	varFilePath := filepath.Join(req.WorkingDir, "generated.auto.tfvars.json")
	if err := ioutil.WriteFile(varFilePath, jsonBytes, 0644); err != nil {
		err = fmt.Errorf("error generating var file: %s", err)
		return nil, err
	}

	return &GenerateVarsForTFReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) Plan(ctx context.Context, req *PlanRequest) (*PlanReply, error) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-r.Done:
			cancel()
		case <-ctx.Done():
		}
	}()

	if req.TfInstance != "1" {
		return nil, fmt.Errorf("no TF instance found")
	}

	var planOpt []tfexec.PlanOption
	if req.Out != "" {
		planOpt = append(planOpt, tfexec.Out(req.Out))
	}

	if req.Refresh == false {
		planOpt = append(planOpt, tfexec.Refresh(req.Refresh))
	}

	if req.Destroy {
		planOpt = append(planOpt, tfexec.Destroy(req.Destroy))
	}

	drifted, err := r.tf.Plan(ctx, planOpt...)
	if err != nil {
		return nil, err
	}
	return &PlanReply{
		Drifted: drifted,
		Message: "ok",
	}, nil
}

func (r *TerraformRunnerServer) ShowPlanFileRaw(ctx context.Context, req *ShowPlanFileRawRequest) (*ShowPlanFileRawReply, error) {
	if req.TfInstance != "1" {
		return nil, fmt.Errorf("no TF instance found")
	}

	rawOutput, err := r.tf.ShowPlanFileRaw(ctx, req.Filename)
	if err != nil {
		return nil, err
	}

	return &ShowPlanFileRawReply{RawOutput: rawOutput}, nil
}

func (r *TerraformRunnerServer) SaveTFPlan(ctx context.Context, req *SaveTFPlanRequest) (*SaveTFPlanReply, error) {
	if req.TfInstance != "1" {
		return nil, fmt.Errorf("no TF instance found")
	}

	var tfplan []byte
	if req.BackendCompletelyDisable {
		tfplan = []byte("dummy plan")
	} else {
		var err error
		tfplan, err = ioutil.ReadFile(filepath.Join(r.tf.WorkingDir(), TFPlanName))
		if err != nil {
			err = fmt.Errorf("error running Plan: %s", err)
			return nil, err
		}
	}

	tfplanObjectKey := types.NamespacedName{Name: "tfplan-default-" + req.Name, Namespace: req.Namespace}
	var tfplanSecret corev1.Secret
	tfplanSecretExists := true
	if err := r.Client.Get(ctx, tfplanObjectKey, &tfplanSecret); err != nil {
		if errors.IsNotFound(err) {
			tfplanSecretExists = false
		} else {
			err = fmt.Errorf("error getting tfplanSecret: %s", err)
			return nil, err
		}
	}

	if tfplanSecretExists {
		if err := r.Client.Delete(ctx, &tfplanSecret); err != nil {
			err = fmt.Errorf("error deleting tfplanSecret: %s", err)
			return nil, err
		}
	}

	planRev := strings.Replace(req.Revision, "/", "-", 1)
	planName := "plan-" + planRev

	tfplan, err := utils.GzipEncode(tfplan)
	if err != nil {
		return nil, err
	}

	tfplanData := map[string][]byte{TFPlanName: tfplan}
	tfplanSecret = corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tfplan-default-" + req.Name,
			Namespace: req.Namespace,
			Annotations: map[string]string{
				"encoding":                "gzip",
				SavedPlanSecretAnnotation: planName,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: infrav1.GroupVersion.Group + "/" + infrav1.GroupVersion.Version,
					Kind:       infrav1.TerraformKind,
					Name:       req.Name,
					UID:        types.UID(req.Uuid),
				},
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: tfplanData,
	}

	if err := r.Client.Create(ctx, &tfplanSecret); err != nil {
		err = fmt.Errorf("error recording plan status: %s", err)
		return nil, err
	}

	return &SaveTFPlanReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) LoadTFPlan(ctx context.Context, req *LoadTFPlanRequest) (*LoadTFPlanReply, error) {
	if req.TfInstance != "1" {
		return nil, fmt.Errorf("no TF instance found")
	}

	tfplanSecretKey := types.NamespacedName{Namespace: req.Namespace, Name: "tfplan-default-" + req.Name}
	tfplanSecret := corev1.Secret{}
	err := r.Get(ctx, tfplanSecretKey, &tfplanSecret)
	if err != nil {
		err = fmt.Errorf("error getting plan secret: %s", err)
		return nil, err
	}

	if tfplanSecret.Annotations[SavedPlanSecretAnnotation] != req.PendingPlan {
		err = fmt.Errorf("error pending plan and plan's name in the secret are not matched: %s != %s",
			req.PendingPlan,
			tfplanSecret.Annotations[SavedPlanSecretAnnotation])
		return nil, err
	}

	if req.BackendCompletelyDisable {
		// do nothing
	} else {
		tfplan := tfplanSecret.Data[TFPlanName]
		tfplan, err = utils.GzipDecode(tfplan)
		if err != nil {
			return nil, err
		}
		err = ioutil.WriteFile(filepath.Join(r.tf.WorkingDir(), TFPlanName), tfplan, 0644)
		if err != nil {
			err = fmt.Errorf("error saving plan file to disk: %s", err)
			return nil, err
		}
	}

	return &LoadTFPlanReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) Destroy(ctx context.Context, req *DestroyRequest) (*DestroyReply, error) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-r.Done:
			cancel()
		case <-ctx.Done():
		}
	}()

	if req.TfInstance != "1" {
		return nil, fmt.Errorf("no TF instance found")
	}

	if err := r.tf.Destroy(ctx); err != nil {
		return nil, err
	}

	return &DestroyReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) Apply(ctx context.Context, req *ApplyRequest) (*ApplyReply, error) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-r.Done:
			cancel()
		case <-ctx.Done():
		}
	}()

	if req.TfInstance != "1" {
		return nil, fmt.Errorf("no TF instance found")
	}

	var applyOpt []tfexec.ApplyOption
	if req.DirOrPlan != "" {
		applyOpt = []tfexec.ApplyOption{tfexec.DirOrPlan(req.DirOrPlan)}
	}

	if req.RefreshBeforeApply {
		applyOpt = []tfexec.ApplyOption{tfexec.Refresh(true)}
	}

	if err := r.tf.Apply(ctx, applyOpt...); err != nil {
		return nil, err
	}

	return &ApplyReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) Output(ctx context.Context, req *OutputRequest) (*OutputReply, error) {
	if req.TfInstance != "1" {
		return nil, fmt.Errorf("no TF instance found")
	}

	outputs, err := r.tf.Output(ctx)
	if err != nil {
		return nil, err
	}

	outputReply := &OutputReply{Outputs: map[string]*OutputMeta{}}
	for k, v := range outputs {
		outputReply.Outputs[k] = &OutputMeta{
			Sensitive: v.Sensitive,
			Type:      v.Type,
			Value:     v.Value,
		}
	}
	return outputReply, nil
}

func (r *TerraformRunnerServer) WriteOutputs(ctx context.Context, req *WriteOutputsRequest) (*WriteOutputsReply, error) {
	objectKey := types.NamespacedName{Namespace: req.Namespace, Name: req.SecretName}
	var outputSecret corev1.Secret

	if err := r.Client.Get(ctx, objectKey, &outputSecret); err == nil {
		if err := r.Client.Delete(ctx, &outputSecret); err != nil {
			return nil, err
		}
	} else if apierrors.IsNotFound(err) == false {
		return nil, err
	}

	block := true
	outputSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.SecretName,
			Namespace: req.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         infrav1.GroupVersion.Version + "/" + infrav1.GroupVersion.Version,
					Kind:               infrav1.TerraformKind,
					Name:               req.Name,
					UID:                types.UID(req.Uuid),
					BlockOwnerDeletion: &block,
				},
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: req.Data,
	}

	err := r.Client.Create(ctx, &outputSecret)
	if err != nil {
		return nil, err
	}

	return &WriteOutputsReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) GetOutputs(ctx context.Context, req *GetOutputsRequest) (*GetOutputsReply, error) {
	outputKey := types.NamespacedName{Namespace: req.Namespace, Name: req.SecretName}
	outputSecret := corev1.Secret{}
	err := r.Client.Get(ctx, outputKey, &outputSecret)
	if err != nil {
		err = fmt.Errorf("error getting terraform output for health checks: %s", err)
		return nil, err
	}

	outputs := map[string]string{}
	// parse map[string][]byte to map[string]string for go template parsing
	if len(outputSecret.Data) > 0 {
		for k, v := range outputSecret.Data {
			outputs[k] = string(v)
		}
	}

	return &GetOutputsReply{Outputs: outputs}, nil
}

func (r *TerraformRunnerServer) FinalizeSecrets(ctx context.Context, req *FinalizeSecretsRequest) (*FinalizeSecretsReply, error) {
	planObjectKey := types.NamespacedName{Namespace: req.Namespace, Name: "tfplan-default-" + req.Name}
	var planSecret corev1.Secret
	if err := r.Client.Get(ctx, planObjectKey, &planSecret); err == nil {
		if err := r.Client.Delete(ctx, &planSecret); err != nil {
			// transient failure
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else if apierrors.IsNotFound(err) {
		return nil, status.Error(codes.NotFound, err.Error())
	} else {
		// transient failure
		return nil, status.Error(codes.Internal, err.Error())
	}

	if req.HasSpecifiedOutputSecret {

		outputsObjectKey := types.NamespacedName{Namespace: req.Namespace, Name: req.OutputSecretName}
		var outputsSecret corev1.Secret
		if err := r.Client.Get(ctx, outputsObjectKey, &outputsSecret); err == nil {
			if err := r.Client.Delete(ctx, &outputsSecret); err != nil {
				// transient failure
				return nil, status.Error(codes.Internal, err.Error())
			}
		} else if apierrors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		} else {
			// transient failure
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &FinalizeSecretsReply{Message: "ok"}, nil
}
