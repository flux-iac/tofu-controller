package runner

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/spf13/afero"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"github.com/weaveworks/tf-controller/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *TerraformRunnerServer) LoadTFPlan(ctx context.Context, req *LoadTFPlanRequest) (*LoadTFPlanReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("loading plan from secret")

	if req.TfInstance != r.InstanceID {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	fs := afero.NewOsFs()
	return loadTFPlan(ctx, log, req, r.terraform, r.tf.WorkingDir(), r.Client, fs)
}

// loadTFPlan loads the plan from the secret and returns the plan as a reply.
func loadTFPlan(
	ctx context.Context,
	log logr.Logger,
	req *LoadTFPlanRequest,
	terraform *infrav1.Terraform,
	workingDir string,
	client client.Client,
	fs afero.Fs,
) (*LoadTFPlanReply, error) {
	tfplanSecretKey := types.NamespacedName{Namespace: req.Namespace, Name: "tfplan-" + terraform.WorkspaceName() + "-" + req.Name}
	tfplanSecret := corev1.Secret{}
	err := client.Get(ctx, tfplanSecretKey, &tfplanSecret)
	if err != nil {
		err = fmt.Errorf("error getting plan secret: %s", err)
		log.Error(err, "unable to get secret")
		return nil, err
	}

	if terraform.Spec.Force == true {
		// skip the annotation check
		log.Info("force mode, skipping the plan's annotation check")
	} else {
		// this must be the short plan format: see api/planid/plan_id.go
		pendingPlanId := req.PendingPlan
		if tfplanSecret.Annotations[SavedPlanSecretAnnotation] != pendingPlanId {
			err = fmt.Errorf("error pending plan and plan's name in the secret are not matched: %s != %s",
				pendingPlanId,
				tfplanSecret.Annotations[SavedPlanSecretAnnotation])
			log.Error(err, "plan name mismatch")
			return nil, err
		}
	}

	if req.BackendCompletelyDisable {
		// do nothing
	} else {
		tfplan := tfplanSecret.Data[TFPlanName]
		tfplan, err = utils.GzipDecode(tfplan)
		if err != nil {
			log.Error(err, "unable to decode the plan")
			return nil, err
		}
		err = afero.WriteFile(fs, filepath.Join(workingDir, TFPlanName), tfplan, 0644)
		if err != nil {
			err = fmt.Errorf("error saving plan file to disk: %s", err)
			log.Error(err, "unable to write the plan to disk")
			return nil, err
		}
	}

	return &LoadTFPlanReply{Message: "ok"}, nil
}
