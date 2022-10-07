package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/fluxcd/pkg/runtime/events"
	"github.com/hashicorp/terraform-exec/tfexec"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"github.com/weaveworks/tf-controller/runner"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func convertOutputs(outputs map[string]*runner.OutputMeta) map[string]tfexec.OutputMeta {
	result := map[string]tfexec.OutputMeta{}
	for k, v := range outputs {
		result[k] = tfexec.OutputMeta{
			Sensitive: v.Sensitive,
			Type:      v.Type,
			Value:     v.Value,
		}
	}
	return result
}

func (r *TerraformReconciler) outputsMayBeDrifted(ctx context.Context, terraform infrav1.Terraform) (bool, error) {
	if terraform.Spec.WriteOutputsToSecret != nil {
		outputsSecretKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Spec.WriteOutputsToSecret.Name}
		var outputsSecret corev1.Secret
		err := r.Client.Get(ctx, outputsSecretKey, &outputsSecret)
		if err != nil && apierrors.IsNotFound(err) {
			return true, nil
		}

		return false, err
	}

	return false, nil
}

func (r *TerraformReconciler) shouldWriteOutputs(terraform infrav1.Terraform, outputs map[string]tfexec.OutputMeta) bool {
	if terraform.Spec.WriteOutputsToSecret != nil && len(outputs) > 0 {
		return true
	}

	return false
}

func (r *TerraformReconciler) processOutputs(ctx context.Context, runnerClient runner.RunnerClient, terraform infrav1.Terraform, tfInstance string, revision string) (infrav1.Terraform, error) {

	log := ctrl.LoggerFrom(ctx)
	objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Name}

	outputs := map[string]tfexec.OutputMeta{}
	var err error
	terraform, err = r.obtainOutputs(ctx, terraform, tfInstance, runnerClient, revision, &outputs)
	if err != nil {
		return terraform, err
	}

	if r.shouldWriteOutputs(terraform, outputs) {
		terraform, err = r.writeOutput(ctx, terraform, runnerClient, outputs, revision)
		if err != nil {
			return terraform, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after writing outputs")
			return terraform, err
		}

	}

	return terraform, nil
}

func (r *TerraformReconciler) obtainOutputs(ctx context.Context, terraform infrav1.Terraform, tfInstance string, runnerClient runner.RunnerClient, revision string, outputs *map[string]tfexec.OutputMeta) (infrav1.Terraform, error) {
	outputReply, err := runnerClient.Output(ctx, &runner.OutputRequest{
		TfInstance: tfInstance,
	})
	if err != nil {
		// TODO should not be this Error
		// warning-like status is enough
		err = fmt.Errorf("error running Output: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecOutputFailedReason,
			err.Error(),
		), err
	}
	*outputs = convertOutputs(outputReply.Outputs)

	var availableOutputs []string
	for k := range *outputs {
		availableOutputs = append(availableOutputs, k)
	}
	if len(availableOutputs) > 0 {
		sort.Strings(availableOutputs)
		terraform = infrav1.TerraformOutputsAvailable(terraform, availableOutputs, "Outputs available")
	}

	return terraform, nil
}

func (r *TerraformReconciler) writeOutput(ctx context.Context, terraform infrav1.Terraform, runnerClient runner.RunnerClient, outputs map[string]tfexec.OutputMeta, revision string) (infrav1.Terraform, error) {
	log := ctrl.LoggerFrom(ctx)

	wots := terraform.Spec.WriteOutputsToSecret
	data := map[string][]byte{}

	// if not specified .spec.writeOutputsToSecret.outputs,
	// then it means export all outputs
	if len(wots.Outputs) == 0 {
		for output, v := range outputs {
			ct, err := ctyjson.UnmarshalType(v.Type)
			if err != nil {
				return terraform, err
			}
			// if it's a string, we can embed it directly into Secret's data
			switch ct {
			case cty.String:
				cv, err := ctyjson.Unmarshal(v.Value, ct)
				if err != nil {
					return terraform, err
				}
				data[output] = []byte(cv.AsString())
			// there's no need to unmarshal and convert to []byte
			// we'll just pass the []byte directly from OutputMeta Value
			case cty.Number, cty.Bool:
				data[output] = v.Value
			default:
				outputBytes, err := json.Marshal(v.Value)
				if err != nil {
					return terraform, err
				}
				data[output] = outputBytes
			}
		}
	} else {
		// filter only defined output
		// output maybe contain mapping output:mapped_name
		for _, outputMapping := range wots.Outputs {
			parts := strings.SplitN(outputMapping, ":", 2)
			var output string
			var mappedTo string
			if len(parts) == 1 {
				output = parts[0]
				mappedTo = parts[0]
				// no mapping
			} else if len(parts) == 2 {
				output = parts[0]
				mappedTo = parts[1]
			} else {
				log.Error(fmt.Errorf("invalid mapping format"), outputMapping)
				continue
			}

			v, exist := outputs[output]
			if !exist {
				log.Error(fmt.Errorf("output not found"), output)
				continue
			}

			ct, err := ctyjson.UnmarshalType(v.Type)
			if err != nil {
				return terraform, err
			}
			switch ct {
			case cty.String:
				cv, err := ctyjson.Unmarshal(v.Value, ct)
				if err != nil {
					return terraform, err
				}
				data[mappedTo] = []byte(cv.AsString())
			// there's no need to unmarshal and convert to []byte
			// we'll just pass the []byte directly from OutputMeta Value
			case cty.Number, cty.Bool:
				data[mappedTo] = v.Value
			default:
				outputBytes, err := json.Marshal(v.Value)
				if err != nil {
					return terraform, err
				}
				data[mappedTo] = outputBytes
			}
		}
	}

	if len(data) == 0 || terraform.Spec.Destroy == true {
		return infrav1.TerraformOutputsWritten(terraform, revision, "No Outputs written"), nil
	}

	writeOutputsReply, err := runnerClient.WriteOutputs(ctx, &runner.WriteOutputsRequest{
		Namespace:  terraform.Namespace,
		Name:       terraform.Name,
		SecretName: terraform.Spec.WriteOutputsToSecret.Name,
		Uuid:       string(terraform.UID),
		Data:       data,
	})
	if err != nil {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.OutputsWritingFailedReason,
			err.Error(),
		), err
	}
	log.Info(fmt.Sprintf("write outputs: %s, changed: %v", writeOutputsReply.Message, writeOutputsReply.Changed))

	if writeOutputsReply.Changed {
		keysWritten := []string{}
		for k, _ := range data {
			keysWritten = append(keysWritten, k)
		}
		msg := fmt.Sprintf("Outputs written.\n%d output(s): %s", len(keysWritten), strings.Join(keysWritten, ", "))
		r.event(ctx, terraform, revision, events.EventSeverityInfo, msg, nil)
	}

	return infrav1.TerraformOutputsWritten(terraform, revision, "Outputs written"), nil
}
