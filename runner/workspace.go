package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/fluxcd/pkg/tar"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *TerraformRunnerServer) UploadAndExtract(ctx context.Context, req *UploadAndExtractRequest) (*UploadAndExtractReply, error) {
	ctrl.LoggerFrom(ctx).Info("preparing for Upload and Extraction")
	tmpDir, err := securejoin.SecureJoin(r.tempDir(), fmt.Sprintf("%s-%s", req.Namespace, req.Name))
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(req.TarGz)
	opts := tar.WithMaxUntarSize(tar.UnlimitedUntarSize)
	if err = tar.Untar(buf, tmpDir, opts); err != nil {
		return nil, fmt.Errorf("failed to untar artifact, error: %w", err)
	}

	dirPath, err := securejoin.SecureJoin(tmpDir, req.Path)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(dirPath); err != nil {
		return nil, fmt.Errorf("terraform path not found: %w", err)
	}

	return &UploadAndExtractReply{WorkingDir: dirPath, TmpDir: tmpDir}, nil
}

func (r *TerraformRunnerServer) CleanupDir(ctx context.Context, req *CleanupDirRequest) (*CleanupDirReply, error) {
	ctrl.LoggerFrom(ctx).Info("cleanup TmpDir", "tmpDir", req.TmpDir)
	if err := os.RemoveAll(req.TmpDir); err != nil {
		return nil, err
	}

	return &CleanupDirReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) WriteBackendConfig(ctx context.Context, req *WriteBackendConfigRequest) (*WriteBackendConfigReply, error) {
	log := ctrl.LoggerFrom(ctx)
	const backendConfigPath = "backend_override.tf"
	log.Info("write backend config", "path", req.DirPath, "config", backendConfigPath)
	filePath, err := securejoin.SecureJoin(req.DirPath, backendConfigPath)
	if err != nil {
		return nil, err
	}

	log.Info("write config to file", "filePath", filePath)
	if err := os.WriteFile(filePath, req.BackendConfig, 0644); err != nil {
		return nil, err
	}

	return &WriteBackendConfigReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) ProcessCliConfig(ctx context.Context, req *ProcessCliConfigRequest) (*ProcessCliConfigReply, error) {
	ctrl.LoggerFrom(ctx).Info("processing configuration", "namespace", req.Namespace, "name", req.Name)
	cliConfigKey := types.NamespacedName{Namespace: req.Namespace, Name: req.Name}
	var cliConfig v1.Secret
	if err := r.Get(ctx, cliConfigKey, &cliConfig); err != nil {
		return nil, err
	}

	if len(cliConfig.Data) != 1 {
		return nil, fmt.Errorf("expect the secret to contain 1 data")
	}

	var tfrcFilepath string
	for tfrcFilename, v := range cliConfig.Data {
		if !strings.HasSuffix(tfrcFilename, ".tfrc") {
			return nil, fmt.Errorf("expect the secret key to end with .tfrc")
		}

		var err error
		tfrcFilepath, err = securejoin.SecureJoin(req.DirPath, tfrcFilename)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(tfrcFilepath, v, 0644); err != nil {
			return nil, err
		}
	}
	return &ProcessCliConfigReply{FilePath: tfrcFilepath}, nil
}

func (r *TerraformRunnerServer) CreateFileMappings(ctx context.Context, req *CreateFileMappingsRequest) (*CreateFileMappingsReply, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("creating file mappings")

	for _, fileMapping := range req.FileMappings {
		var fileFullPath string
		var err error
		switch fileMapping.Location {
		case runnerFileMappingLocationHome:
			fileFullPath, err = securejoin.SecureJoin(r.homeDir(), fileMapping.Path)
		case runnerFileMappingLocationWorkspace:
			fileFullPath, err = securejoin.SecureJoin(req.WorkingDir, fileMapping.Path)
		default:
			err = fmt.Errorf("unknown file mapping location")
		}
		if err != nil {
			return nil, err
		}

		log.Info("prepare to write file", "fileFullPath", fileFullPath)
		dir := filepath.Dir(fileFullPath)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, runnerFileMappingDirectoryPermissions); err != nil {
				return nil, err
			}
		}

		if err := os.WriteFile(fileFullPath, fileMapping.Content, runnerFileMappingFilePermissions); err != nil {
			return nil, err
		}
	}

	return &CreateFileMappingsReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) GenerateTemplate(ctx context.Context, req *GenerateTemplateRequest) (*GenerateTemplateReply, error) {
	log := controllerruntime.LoggerFrom(ctx)
	log.Info("generating the template founds")

	workDir := req.WorkingDir

	// find main.tf.tpl file
	mainTfTplPath := filepath.Join(workDir, "main.tf.tpl")
	if _, err := os.Stat(mainTfTplPath); os.IsNotExist(err) {
		log.Info("main.tf.tpl not found, skipping")
		return &GenerateTemplateReply{Message: "ok"}, nil
	}

	// marshal the vars
	vars := make(map[string]any)

	varFilePath := filepath.Join(req.WorkingDir, "generated.auto.tfvars.json")
	jsonBytes, err := os.ReadFile(varFilePath)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(jsonBytes, &vars); err != nil {
		return nil, err
	}

	// render the template
	// we use Helm-like syntax for the template
	tmpl, parseErr := template.New("main.tf.tpl").
		Delims("{{", "}}").
		Funcs(sprig.TxtFuncMap()).
		ParseFiles(mainTfTplPath)
	if parseErr != nil {
		return nil, parseErr
	}

	mainTfPath := filepath.Join(workDir, "main.tf")
	f, fileErr := os.Create(mainTfPath)
	if fileErr != nil {
		return nil, fileErr
	}

	// make it Helm compatible
	vars["Values"] = vars["values"]

	if err := tmpl.Execute(f, vars); err != nil {
		return nil, err
	}

	if err := f.Close(); err != nil {
		return nil, err
	}

	return &GenerateTemplateReply{Message: "ok"}, nil
}
