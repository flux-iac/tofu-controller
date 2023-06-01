package runner

import (
	"context"
	"fmt"
	"reflect"

	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *TerraformRunnerServer) Output(ctx context.Context, req *OutputRequest) (*OutputReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("creating outputs")
	if req.TfInstance != r.InstanceID {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	outputs, err := r.tf.Output(ctx)
	if err != nil {
		log.Error(err, "unable to get outputs")
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
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("write outputs to secret")

	objectKey := types.NamespacedName{Namespace: req.Namespace, Name: req.SecretName}
	var outputSecret corev1.Secret

	drift := true
	create := true
	if err := r.Client.Get(ctx, objectKey, &outputSecret); err == nil {
		// if everything is there, we don't write anything
		if reflect.DeepEqual(outputSecret.Data, req.Data) {
			drift = false
		} else {
			// found, but need update
			create = false
		}
	} else if apierrors.IsNotFound(err) == false {
		log.Error(err, "unable to get output secret")
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

			err := r.Client.Create(ctx, &outputSecret)
			if err != nil {
				log.Error(err, "unable to create secret")
				return nil, err
			}
		} else {
			outputSecret.Data = req.Data
			err := r.Client.Update(ctx, &outputSecret)
			if err != nil {
				log.Error(err, "unable to update secret")
				return nil, err
			}
		}

		return &WriteOutputsReply{Message: "ok", Changed: true}, nil
	}

	return &WriteOutputsReply{Message: "ok", Changed: false}, nil
}

func (r *TerraformRunnerServer) GetOutputs(ctx context.Context, req *GetOutputsRequest) (*GetOutputsReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("get outputs")
	outputKey := types.NamespacedName{Namespace: req.Namespace, Name: req.SecretName}
	outputSecret := corev1.Secret{}
	err := r.Client.Get(ctx, outputKey, &outputSecret)
	if err != nil {
		err = fmt.Errorf("error getting terraform output for health checks: %s", err)
		log.Error(err, "unable to check terraform health")
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
