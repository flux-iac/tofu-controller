package controllers

import (
	"context"
	"fmt"
	eventv1 "github.com/fluxcd/pkg/apis/event/v1beta1"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-exec/tfexec"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
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

		keysInSecret := []string{}
		for k, _ := range outputsSecret.Data {
			keysInSecret = append(keysInSecret, k)
		}
		sort.Strings(keysInSecret)

		keysInSpec := terraform.Spec.WriteOutputsToSecret.Outputs
		if len(keysInSpec) == 0 {
			keysInSpec = terraform.Status.AvailableOutputs
		}
		sort.Strings(keysInSpec)

		if strings.Join(keysInSecret, ",") != strings.Join(keysInSpec, ",") {
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

	// OutputMeta has
	// 1. type
	// 2. value
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

	var filteredOutputs map[string]tfexec.OutputMeta
	if len(wots.Outputs) == 0 {
		filteredOutputs = outputs
	} else {
		if result, err := filterOutputs(outputs, wots.Outputs); err != nil {
			return infrav1.TerraformNotReady(
				terraform,
				revision,
				infrav1.OutputsWritingFailedReason,
				err.Error(),
			), err
		} else {
			filteredOutputs = result
		}
	}

	for outputOrAlias, outputMeta := range filteredOutputs {
		ct, err := ctyjson.UnmarshalType(outputMeta.Type)
		if err != nil {
			return terraform, err
		}

		if ct == cty.String {
			cv, err := ctyjson.Unmarshal(outputMeta.Value, ct)
			if err != nil {
				return terraform, err
			}
			data[outputOrAlias] = []byte(cv.AsString())
		} else {
			data[outputOrAlias] = outputMeta.Value
			data[outputOrAlias+".type"] = outputMeta.Type
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
		r.event(ctx, terraform, revision, eventv1.EventSeverityInfo, msg, nil)
	}

	return infrav1.TerraformOutputsWritten(terraform, revision, "Outputs written"), nil
}

func filterOutputs(outputs map[string]tfexec.OutputMeta, outputsToWrite []string) (map[string]tfexec.OutputMeta, error) {
	if outputs == nil || outputsToWrite == nil {
		return nil, fmt.Errorf("input maps or outputsToWrite slice cannot be nil")
	}

	filteredOutputs := make(map[string]tfexec.OutputMeta)
	for _, outputMapping := range outputsToWrite {
		if len(outputMapping) == 0 {
			return nil, fmt.Errorf("output mapping cannot be empty")
		}

		// parse output mapping (output[:alias])
		parts := strings.SplitN(outputMapping, ":", 2)
		var (
			output string
			alias  string
		)
		if len(parts) == 1 {
			output = parts[0]
			alias = parts[0]
		} else if len(parts) == 2 {
			output = parts[0]
			alias = parts[1]
		} else {
			return nil, fmt.Errorf("invalid output mapping format: %s", outputMapping)
		}

		outputMeta, exist := outputs[output]
		if !exist {
			return nil, fmt.Errorf("output not found: %s", output)
		}

		filteredOutputs[alias] = outputMeta
	}

	return filteredOutputs, nil
}
