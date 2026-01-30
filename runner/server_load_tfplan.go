package runner

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/flux-iac/tofu-controller/api/plan"
	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/spf13/afero"
	v1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *TerraformRunnerServer) LoadTFPlan(ctx context.Context, req *LoadTFPlanRequest) (*LoadTFPlanReply, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("loading plan from secret")

	if err := r.ValidateInstanceID(req.TfInstance); err != nil {
		log.Error(err, "terraform session mismatch when loading the plan")

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
	kubeClient client.Client,
	fs afero.Fs,
) (*LoadTFPlanReply, error) {
	if !req.BackendCompletelyDisable {
		secrets := &v1.SecretList{}

		// List relevant secrets
		if err := kubeClient.List(ctx, secrets, client.InNamespace(req.Namespace), client.MatchingLabels{
			plan.TFPlanNameLabel:      plan.SafeLabelValue(req.Name),
			plan.TFPlanWorkspaceLabel: terraform.WorkspaceName(),
		}); err != nil {
			log.Error(err, "unable to list existing plan secrets")
			return nil, err
		}

		// Check that we actually have some secrets to read
		if len(secrets.Items) == 0 {
			err := fmt.Errorf("no plan secrets found for plan %s", req.PendingPlan)
			log.Error(err, "no plan secret found")
			return nil, err
		}

		pendingPlanId := req.PendingPlan

		for _, s := range secrets.Items {
			if terraform.Spec.Force {
				continue
			}

			if s.Annotations[plan.TFPlanSavedAnnotation] != pendingPlanId {
				return nil, fmt.Errorf("pending plan %s does not match secret %s (%s)", pendingPlanId, s.Name, s.Annotations[plan.TFPlanSavedAnnotation])
			}
		}

		tfPlan, err := plan.NewFromSecrets(req.Name, req.Namespace, string(terraform.GetUID()), secrets.Items)
		if err != nil {
			log.Error(err, "unable to reconstruct plan from secrets")
			return nil, err
		}

		err = afero.WriteFile(fs, filepath.Join(workingDir, TFPlanName), tfPlan.Bytes(), 0644)
		if err != nil {
			err = fmt.Errorf("error saving plan file to disk: %s", err)
			log.Error(err, "unable to write the plan to disk")
			return nil, err
		}
	}

	return &LoadTFPlanReply{Message: "ok"}, nil
}
