package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/hashicorp/terraform-exec/tfexec"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *TerraformRunnerServer) tfInit(ctx context.Context, opts ...tfexec.InitOption) error {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)

	// This is the only place where we disable the logger
	r.tf.SetStdout(io.Discard)
	errBuf := &bytes.Buffer{}
	r.tf.SetStderr(errBuf)

	defer r.initLogger(log)

	err := r.tf.Init(ctx, opts...)

	var sl *StateLockError
	if err != nil && !errors.As(err, &sl) {
		fmt.Fprint(os.Stderr, errBuf.String())
		err = errors.New(errBuf.String())
	}

	return err
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
	if err := r.tfInit(ctx, initOpts...); err != nil {
		st := status.New(codes.Internal, err.Error())
		var stateErr *StateLockError

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
