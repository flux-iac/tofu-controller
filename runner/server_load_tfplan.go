package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kClient "sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/utils"
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
	client client.Client,
	fs afero.Fs,
) (*LoadTFPlanReply, error) {
	secrets := &v1.SecretList{}

	planIDLabel := fmt.Sprintf("tfplan-%s-%s", terraform.WorkspaceName(), req.Name)

	// List relevant secrets
	if err := client.List(ctx, secrets, kClient.InNamespace(req.Namespace), kClient.MatchingLabels{
		"infra.contrib.fluxcd.io/plan-id": planIDLabel,
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

	// To store the individual plan chunks by index
	chunkMap := make(map[int][]byte)

	for _, secret := range secrets.Items {
		if !terraform.Spec.Force {
			// Check that the plan IDs match what we are expecting
			if secret.Annotations[SavedPlanSecretAnnotation] != pendingPlanId {
				err := fmt.Errorf("error pending plan and plan's name in the secret are not matched: %s != %s",
					pendingPlanId,
					secret.Annotations[SavedPlanSecretAnnotation])
				log.Error(err, "plan name mismatch")
				return nil, err
			}
		}

		// Check our plan data is still there in the secret
		// TODO: also validate checksum?
		planStr, ok := secret.Data[TFPlanName]
		if !ok {
			err := fmt.Errorf("secret %s missing key %s", secret.Name, TFPlanName)
			log.Error(err, "missing plan data")
			return nil, err
		}

		// Grab the chunk index from the secret annotation
		chunkIndex := 0
		if idxStr, ok := secret.Annotations["infra.contrib.fluxcd.io/plan-chunk"]; ok && idxStr != "" {
			var err error
			chunkIndex, err = strconv.Atoi(idxStr)
			if err != nil {
				log.Error(err, "invalid chunk index annotation found on secret", "secret", secret.Name)
				return nil, err
			}
		}

		chunkMap[chunkIndex] = planStr
	}

	if !req.BackendCompletelyDisable {
		var planBytes []byte

		// we know the number of chunks we "should" have, so work
		// up til there checking we have each chunk
		for i := 0; i < len(chunkMap); i++ {
			chunk, ok := chunkMap[i]
			if !ok {
				err := fmt.Errorf("missing chunk %d for plan %s", i, planIDLabel)
				log.Error(err, "incomplete plan")
				return nil, err
			}
			planBytes = append(planBytes, chunk...)
		}

		tfplan, err := utils.GzipDecode(planBytes)
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
