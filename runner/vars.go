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

	"github.com/flux-iac/tofu-controller/api/typeinfo"
	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/utils"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/json"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ctyValueToGoValue(ctyValue cty.Value) (any, error) {
	var goValue any

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
		list := make([]any, 0, ctyValue.LengthInt())
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

	// cty sets hold no duplicates, so every member is kept.
	if ctyValue.Type().IsSetType() {
		result := make([]any, 0, ctyValue.LengthInt())
		for it := ctyValue.ElementIterator(); it.Next(); {
			_, v := it.Element()
			goVal, err := ctyValueToGoValue(v)
			if err != nil {
				return nil, err
			}
			result = append(result, goVal)
		}
		goValue = result
		return goValue, nil
	}

	if ctyValue.Type().IsMapType() {
		m := make(map[string]any)
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
		t := make([]any, 0, ctyValue.LengthInt())
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
		o := make(map[string]any)
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

func convertSecretDataToInputs(secret *v1.Secret) (map[string]any, error) {
	var keys []string
	for k := range secret.Data {
		if strings.HasSuffix(k, typeinfo.Suffix) {
			continue
		}
		keys = append(keys, k)
	}

	data := map[string]any{}
	for _, key := range keys {
		typeInfo, exist := secret.Data[key+typeinfo.Suffix]
		if exist {
			raw := secret.Data[key]
			ct, err := json.UnmarshalType(typeInfo)
			if err != nil {
				return nil, err
			}

			// The decode error can quote part of the value, so it is not
			// passed on.
			cv, err := json.Unmarshal(raw, ct)
			if err != nil {
				return nil, &InvalidInputError{Err: fmt.Errorf("secret %s key %q: value does not match its declared type", secret.Name, key)}
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

func getSecretForReadInputs(ctx context.Context, r client.Client, objectKey client.ObjectKey) (*v1.Secret, error) {
	secret := &v1.Secret{}
	err := r.Get(ctx, objectKey, secret)
	if err != nil {
		return secret, err
	}
	return secret, nil
}

func readInputsForGenerateVarsForTF(ctx context.Context, c client.Client, terraform *infrav1.Terraform) (map[string]any, error) {
	inputs := map[string]any{}
	if len(terraform.Spec.ReadInputsFromSecrets) > 0 {
		for _, readSpec := range terraform.Spec.ReadInputsFromSecrets {
			objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: readSpec.Name}
			secret, err := getSecretForReadInputs(ctx, c, objectKey)
			if err != nil {
				return nil, err
			}
			data, err := convertSecretDataToInputs(secret)
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
	log := controllerruntime.LoggerFrom(ctx)
	log.Info("setting up the input variables")

	// use from the cached object
	terraform := *sessionFrom(ctx).terraform

	vars := map[string]*apiextensionsv1.JSON{}

	//inputs := map[string]interface{}{}
	inputs, err := readInputsForGenerateVarsForTF(ctx, r.Client, &terraform)
	if err != nil {
		return nil, err
	}

	log.Info("mapping the Spec.Values")
	if terraform.Spec.Values != nil {
		tmpl, err := template.New("values").
			Delims("${{", "}}").
			Parse(string(terraform.Spec.Values.Raw))
		if err != nil {
			return nil, err
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, inputs); err != nil {
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
		switch vf.Kind {
		case "Secret":
			var s v1.Secret
			err := r.Get(ctx, objectKey, &s)
			if err != nil && !vf.Optional {
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
				for _, pattern := range vf.VarsKeys {
					oldKey, newKey, err := parseRenamePattern(pattern)
					if err != nil {
						return nil, err
					}

					vars[newKey], err = utils.JSONEncodeBytes(s.Data[oldKey])
					if err != nil {
						return nil, fmt.Errorf("failed to encode key %q with error: %w", pattern, err)
					}
				}
			}
		case "ConfigMap":
			var cm v1.ConfigMap
			err := r.Get(ctx, objectKey, &cm)
			if err != nil && !vf.Optional {
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
				for _, pattern := range vf.VarsKeys {
					oldKey, newKey, err := parseRenamePattern(pattern)
					if err != nil {
						return nil, err
					}

					if val, ok := cm.Data[oldKey]; ok {
						vars[newKey], err = utils.JSONEncodeBytes([]byte(val))
						if err != nil {
							return nil, fmt.Errorf("failed to encode key %s with error: %w", pattern, err)
						}
					}
					if val, ok := cm.BinaryData[oldKey]; ok {
						vars[newKey], err = utils.JSONEncodeBytes(val)
						if err != nil {
							return nil, err
						}
					}
				}
			}
		}
	}

	jsonBytes, err := json2.Marshal(vars)
	if err != nil {
		return nil, err
	}

	varFilePath := filepath.Join(req.WorkingDir, "generated.auto.tfvars.json")
	if err := os.WriteFile(varFilePath, jsonBytes, 0644); err != nil {
		return nil, fmt.Errorf("error generating var file: %s", err)
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
