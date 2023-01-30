package runner

import (
	"context"
	"encoding/json"
	"github.com/Masterminds/sprig/v3"
	"io/ioutil"
	"os"
	"path/filepath"
	"sigs.k8s.io/controller-runtime"
	"text/template"
)

func (r *TerraformRunnerServer) GenerateTemplate(ctx context.Context, req *GenerateTemplateRequest) (*GenerateTemplateReply, error) {
	log := controllerruntime.LoggerFrom(ctx).WithName(loggerName)
	log.Info("generating the template founds")

	workDir := req.WorkingDir

	// find main.tf.tpl file
	mainTfTplPath := filepath.Join(workDir, "main.tf.tpl")
	if _, err := os.Stat(mainTfTplPath); os.IsNotExist(err) {
		log.Info("main.tf.tpl not found, skipping")
		return &GenerateTemplateReply{Message: "ok"}, nil
	}

	// marshal the vars
	vars := make(map[string]interface{})

	varFilePath := filepath.Join(req.WorkingDir, "generated.auto.tfvars.json")
	jsonBytes, err := ioutil.ReadFile(varFilePath)
	if err != nil {
		log.Error(err, "unable to read the file", "filePath", varFilePath)
		return nil, err
	}

	if err := json.Unmarshal(jsonBytes, &vars); err != nil {
		log.Error(err, "unable to unmarshal the data")
		return nil, err
	}

	// render the template
	// we use Helm-like syntax for the template
	tmpl, parseErr := template.New("main.tf.tpl").
		Delims("{{", "}}").
		Funcs(sprig.TxtFuncMap()).
		ParseFiles(mainTfTplPath)
	if parseErr != nil {
		log.Error(parseErr, "unable to parse the template", "filePath", mainTfTplPath)
		return nil, parseErr
	}

	mainTfPath := filepath.Join(workDir, "main.tf")
	f, fileErr := os.Create(mainTfPath)
	if fileErr != nil {
		log.Error(fileErr, "unable to create the file", "filePath", mainTfPath)
		return nil, fileErr
	}

	// make it Helm compatible
	vars["Values"] = vars["values"]

	if err := tmpl.Execute(f, vars); err != nil {
		log.Error(err, "unable to execute the template")
		return nil, err
	}

	if err := f.Close(); err != nil {
		return nil, err
	}

	return &GenerateTemplateReply{Message: "ok"}, nil
}
