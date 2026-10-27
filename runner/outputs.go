package runner

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
)

func (r *TerraformRunnerServer) WriteOutputs(ctx context.Context, req *WriteOutputsRequest) (*WriteOutputsReply, error) {
	ctrl.LoggerFrom(ctx).Info("write outputs to secret")

	objectKey := types.NamespacedName{Namespace: req.Namespace, Name: req.SecretName}
	var outputSecret corev1.Secret

	drift := true
	create := true
	if err := r.Get(ctx, objectKey, &outputSecret); err == nil {
		// if everything is there, we don't write anything
		if reflect.DeepEqual(outputSecret.Data, req.Data) {
			drift = false
		} else {
			// found, but need update
			create = false
		}
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}

	if drift {
		if create {
			vTrue := true
			outputSecret = corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        req.SecretName,
					Namespace:   req.Namespace,
					Labels:      req.Labels,
					Annotations: req.Annotations,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: infrav1.GroupVersion.Group + "/" + infrav1.GroupVersion.Version,
							Kind:       infrav1.TerraformKind,
							Name:       req.Name,
							UID:        types.UID(req.Uuid),
							Controller: &vTrue,
						},
					},
				},
				Type: corev1.SecretTypeOpaque,
				Data: req.Data,
			}

			if err := r.Create(ctx, &outputSecret); err != nil {
				return nil, err
			}
		} else {
			outputSecret.Data = req.Data
			if err := r.Update(ctx, &outputSecret); err != nil {
				return nil, err
			}
		}

		return &WriteOutputsReply{Message: "ok", Changed: true}, nil
	}

	return &WriteOutputsReply{Message: "ok", Changed: false}, nil
}

func (r *TerraformRunnerServer) GetOutputs(ctx context.Context, req *GetOutputsRequest) (*GetOutputsReply, error) {
	ctrl.LoggerFrom(ctx).Info("get outputs")
	outputKey := types.NamespacedName{Namespace: req.Namespace, Name: req.SecretName}
	outputSecret := corev1.Secret{}
	if err := r.Get(ctx, outputKey, &outputSecret); err != nil {
		return nil, fmt.Errorf("error getting terraform output for health checks: %s", err)
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
