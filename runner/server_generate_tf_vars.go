package runner

import (
	"bytes"
	"context"
	json2 "encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/go-logr/logr"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"github.com/weaveworks/tf-controller/utils"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/json"
	"k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ctyValueToGoValue(ctyValue cty.Value) (interface{}, error) {
	var goValue interface{}

	if ctyValue.IsNull() {
		return nil, nil
	}

	if ctyValue.Type() == cty.String {
		goValue = ctyValue.AsString()
		return goValue, nil
	}

	if ctyValue.Type() == cty.Number {
		goValue, _ = ctyValue.AsBigFloat().Float64()
		return goValue, nil
	}

	if ctyValue.Type() == cty.Bool {
		goValue = ctyValue.True()
		return goValue, nil
	}

	if ctyValue.Type().IsListType() {
		list := make([]interface{}, 0, ctyValue.LengthInt())
		for it := ctyValue.ElementIterator(); it.Next(); {
			_, v := it.Element()
			goVal, err := ctyValueToGoValue(v)
			if err != nil {
				return nil, err
			}
			list = append(list, goVal)
		}
		goValue = list
		return goValue, nil
	}

	if ctyValue.Type().IsSetType() {
		result := make([]interface{}, 0, ctyValue.LengthInt())
		set := make(map[interface{}]struct{})
		for it := ctyValue.ElementIterator(); it.Next(); {
			_, v := it.Element()
			goVal, err := ctyValueToGoValue(v)
			if err != nil {
				return nil, err
			}

			if _, exist := set[goVal]; !exist {
				set[goVal] = struct{}{}
				result = append(result, goVal)
			}
		}
		goValue = result
		return goValue, nil
	}

	if ctyValue.Type().IsMapType() {
		m := make(map[string]interface{})
		for it := ctyValue.ElementIterator(); it.Next(); {
			k, v := it.Element()
			goKey, err := ctyValueToGoValue(k)
			if err != nil {
				return nil, err
			}
			key, ok := goKey.(string)
			if !ok {
				return nil, fmt.Errorf("map key must be string, got %T", goKey)
			}
			goVal, err := ctyValueToGoValue(v)
			if err != nil {
				return nil, err
			}
			m[key] = goVal
		}
		goValue = m
		return goValue, nil
	}

	if ctyValue.Type().IsTupleType() {
		t := make([]interface{}, 0, ctyValue.LengthInt())
		for it := ctyValue.ElementIterator(); it.Next(); {
			_, v := it.Element()
			goVal, err := ctyValueToGoValue(v)
			if err != nil {
				return nil, err
			}
			t = append(t, goVal)
		}
		goValue = t
		return goValue, nil
	}

	if ctyValue.Type().IsObjectType() {
		o := make(map[string]interface{})
		for it := ctyValue.ElementIterator(); it.Next(); {
			k, v := it.Element()
			goKey, err := ctyValueToGoValue(k)
			if err != nil {
				return nil, err
			}
			key, ok := goKey.(string)
			if !ok {
				return nil, fmt.Errorf("object key must be string, got %T", goKey)
			}
			goVal, err := ctyValueToGoValue(v)
			if err != nil {
				return nil, err
			}
			o[key] = goVal
		}
		goValue = o
		return goValue, nil
	}

	return nil, fmt.Errorf("unsupported type: %s", ctyValue.Type().FriendlyName())
}

func convertSecretDataToInputs(log logr.Logger, secret *v1.Secret) (map[string]interface{}, error) {
	var keys []string
	for k := range secret.Data {
		if strings.HasSuffix(k, ".type") {
			continue
		}
		keys = append(keys, k)
	}

	data := map[string]interface{}{}
	for _, key := range keys {
		typeInfo, exist := secret.Data[key+".type"]
		if exist {
			raw := secret.Data[key]
			ct, err := json.UnmarshalType(typeInfo)
			if err != nil {
				log.Error(err, "unable to unmarshal type", "type", string(typeInfo), "key", key)
				return nil, err
			}

			cv, err := json.Unmarshal(raw, ct)
			if err != nil {
				log.Error(err, "unable to unmarshal value", "raw", string(raw), "key", key)
				return nil, err
			}

			result, err := ctyValueToGoValue(cv)
			if err != nil {
				return nil, err
			}
			data[key] = result
		} else { // this is string
			data[key] = string(secret.Data[key])
		}
	}

	return data, nil
}

func getSecretForReadInputs(ctx context.Context, log logr.Logger, r client.Client, objectKey client.ObjectKey) (*v1.Secret, error) {
	secret := &v1.Secret{}
	err := r.Get(ctx, objectKey, secret)
	if err != nil {
		log.Error(err, "unable to get secret", "secret", objectKey)
		return secret, err
	}
	return secret, nil
}

func readInputsForGenerateVarsForTF(ctx context.Context, log logr.Logger, c client.Client, terraform *infrav1.Terraform) (map[string]interface{}, error) {
	inputs := map[string]interface{}{}
	if len(terraform.Spec.ReadInputsFromSecrets) > 0 {
		for _, readSpec := range terraform.Spec.ReadInputsFromSecrets {
			objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: readSpec.Name}
			secret, err := getSecretForReadInputs(ctx, log, c, objectKey)
			if err != nil {
				return nil, err
			}
			data, err := convertSecretDataToInputs(log, secret)
			if err != nil {
				return nil, err
			}
			inputs[readSpec.As] = data
		}
	}
	return inputs, nil
}

// GenerateVarsForTF renders the Terraform variables as a json file for the given inputs
// variables supplied in the varsFrom field will override those specified in the spec
func (r *TerraformRunnerServer) GenerateVarsForTF(ctx context.Context, req *GenerateVarsForTFRequest) (*GenerateVarsForTFReply, error) {
	log := controllerruntime.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("setting up the input variables")

	// use from the cached object
	terraform := *r.terraform

	vars := map[string]*apiextensionsv1.JSON{}

	//inputs := map[string]interface{}{}
	inputs, err := readInputsForGenerateVarsForTF(ctx, log, r.Client, &terraform)
	if err != nil {
		return nil, err
	}

	log.Info("mapping the Spec.Values")
	if terraform.Spec.Values != nil {
		tmpl, err := template.New("values").
			Delims("${{", "}}").
			Parse(string(terraform.Spec.Values.Raw))
		if err != nil {
			log.Error(err, "unable to parse values as template")
			return nil, err
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, inputs); err != nil {
			log.Error(err, "unable to execute values template")
			return nil, err
		}

		vars["values"] = &apiextensionsv1.JSON{Raw: buf.Bytes()}
	}

	log.Info("mapping the Spec.Vars")
	if len(terraform.Spec.Vars) > 0 {
		for _, v := range terraform.Spec.Vars {
			vars[v.Name] = v.Value
		}
	}

	log.Info("mapping the Spec.VarsFrom")
	// varsFrom overwrite vars
	for _, vf := range terraform.Spec.VarsFrom {
		objectKey := types.NamespacedName{
			Namespace: terraform.Namespace,
			Name:      vf.Name,
		}
		if vf.Kind == "Secret" {
			var s v1.Secret
			err := r.Get(ctx, objectKey, &s)
			if err != nil && vf.Optional == false {
				log.Error(err, "unable to get object key", "objectKey", objectKey, "secret", s.ObjectMeta.Name)
				return nil, err
			}
			// if VarsKeys is null, use all
			if vf.VarsKeys == nil {
				for key, val := range s.Data {
					vars[key], err = utils.JSONEncodeBytes(val)
					if err != nil {
						err := fmt.Errorf("failed to encode key %s with error: %w", key, err)
						log.Error(err, "encoding failure")
						return nil, err
					}
				}
			} else {
				for _, pattern := range vf.VarsKeys {
					oldKey, newKey, err := parseRenamePattern(pattern)
					if err != nil {
						log.Error(err, "unable to parse rename pattern")
						return nil, err
					}

					vars[newKey], err = utils.JSONEncodeBytes(s.Data[oldKey])
					if err != nil {
						err := fmt.Errorf("failed to encode key %q with error: %w", pattern, err)
						log.Error(err, "encoding failure")
						return nil, err
					}
				}
			}
		} else if vf.Kind == "ConfigMap" {
			var cm v1.ConfigMap
			err := r.Get(ctx, objectKey, &cm)
			if err != nil && vf.Optional == false {
				log.Error(err, "unable to get object key", "objectKey", objectKey, "configmap", cm.ObjectMeta.Name)
				return nil, err
			}

			// if VarsKeys is null, use all
			if vf.VarsKeys == nil {
				for key, val := range cm.Data {
					vars[key], err = utils.JSONEncodeBytes([]byte(val))
					if err != nil {
						err := fmt.Errorf("failed to encode key %s with error: %w", key, err)
						log.Error(err, "encoding failure")
						return nil, err
					}
				}
				for key, val := range cm.BinaryData {
					vars[key], err = utils.JSONEncodeBytes(val)
					if err != nil {
						err := fmt.Errorf("failed to encode key %s with error: %w", key, err)
						log.Error(err, "encoding failure")
						return nil, err
					}
				}
			} else {
				for _, pattern := range vf.VarsKeys {
					oldKey, newKey, err := parseRenamePattern(pattern)
					if err != nil {
						log.Error(err, "unable to parse rename pattern")
						return nil, err
					}

					if val, ok := cm.Data[oldKey]; ok {
						vars[newKey], err = utils.JSONEncodeBytes([]byte(val))
						if err != nil {
							err := fmt.Errorf("failed to encode key %s with error: %w", pattern, err)
							log.Error(err, "encoding failure")
							return nil, err
						}
					}
					if val, ok := cm.BinaryData[oldKey]; ok {
						vars[newKey], err = utils.JSONEncodeBytes(val)
						if err != nil {
							log.Error(err, "encoding failure")
							return nil, err
						}
					}
				}
			}
		}
	}

	jsonBytes, err := json2.Marshal(vars)
	if err != nil {
		log.Error(err, "unable to marshal the data")
		return nil, err
	}

	varFilePath := filepath.Join(req.WorkingDir, "generated.auto.tfvars.json")
	if err := os.WriteFile(varFilePath, jsonBytes, 0644); err != nil {
		err = fmt.Errorf("error generating var file: %s", err)
		log.Error(err, "unable to write the data to file", "filePath", varFilePath)
		return nil, err
	}

	return &GenerateVarsForTFReply{Message: "ok"}, nil
}

func parseRenamePattern(pattern string) (string, string, error) {
	oldKey := pattern
	newKey := pattern
	if strings.Contains(pattern, ":") {
		parts := strings.Split(pattern, ":")
		if len(parts) != 2 {
			err := fmt.Errorf("invalid rename pattern %q", pattern)
			return "", "", err
		}

		if parts[0] == "" {
			err := fmt.Errorf("invalid rename pattern old name: %q", pattern)
			return "", "", err
		}

		if parts[1] == "" {
			err := fmt.Errorf("invalid rename pattern new name: %q", pattern)
			return "", "", err
		}

		oldKey = parts[0]
		newKey = parts[1]
	}

	return oldKey, newKey, nil
}
