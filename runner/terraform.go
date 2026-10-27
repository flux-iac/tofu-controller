package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
)

func (r *TerraformRunnerServer) Init(ctx context.Context, req *InitRequest) (*InitReply, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("initializing")

	s := sessionFrom(ctx)
	terraform := s.terraform

	log.Info("mapping the Spec.BackendConfigsFrom")
	backendConfigsOpts := []tfexec.InitOption{}
	for _, bf := range terraform.Spec.BackendConfigsFrom {
		objectKey := types.NamespacedName{
			Namespace: terraform.Namespace,
			Name:      bf.Name,
		}
		switch bf.Kind {
		case "Secret":
			var secret corev1.Secret
			err := r.Get(ctx, objectKey, &secret)
			if err != nil && !bf.Optional {
				return nil, err
			}
			// if VarsKeys is null, use all
			if bf.Keys == nil {
				for key, val := range secret.Data {
					backendConfigsOpts = append(backendConfigsOpts, tfexec.BackendConfig(key+"="+string(val)))
				}
			} else {
				for _, key := range bf.Keys {
					// A key missing from the Secret is skipped, so it
					// cannot blank the backend setting.
					if val, ok := secret.Data[key]; ok {
						backendConfigsOpts = append(backendConfigsOpts, tfexec.BackendConfig(key+"="+string(val)))
					}
				}
			}
		case "ConfigMap":
			var cm corev1.ConfigMap
			err := r.Get(ctx, objectKey, &cm)
			if err != nil && !bf.Optional {
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
	if err := s.tf.Init(ctx, initOpts...); err != nil {
		return nil, err
	}

	return &InitReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) SelectWorkspace(ctx context.Context, req *WorkspaceRequest) (*WorkspaceReply, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("workspace select")

	s := sessionFrom(ctx)
	if s.terraform.WorkspaceName() != infrav1.DefaultWorkspaceName {
		ws := s.terraform.Spec.Workspace
		// Select first, as the workspace usually exists. Only when that
		// fails is it created and selected again.
		if err := s.tf.WorkspaceSelect(ctx, ws); err != nil {
			if newErr := s.tf.WorkspaceNew(ctx, ws); newErr != nil {
				log.Error(newErr, "failed to create workspace", "workspace", ws)
				return nil, fmt.Errorf("failed to select workspace %s: %w", ws, errors.Join(err, newErr))
			}
			if err := s.tf.WorkspaceSelect(ctx, ws); err != nil {
				return nil, fmt.Errorf("failed to select workspace %s: %w", ws, err)
			}
		}
	}

	return &WorkspaceReply{Message: "ok"}, nil
}

// appendPlanSpecOptions maps the plan-only options from spec.plan onto tfexec
// plan options. It never affects the apply phase. A nil spec leaves the given
// options unchanged.
func appendPlanSpecOptions(opts []tfexec.PlanOption, plan *infrav1.PlanSpec) []tfexec.PlanOption {
	if plan == nil {
		return opts
	}

	if plan.Lock != nil {
		opts = append(opts, tfexec.Lock(*plan.Lock))
	}

	return opts
}

// tfVarsFileOptions turns the tfvars file paths, relative to root, into
// -var-file plan options. Paths that climb out of root with .. are resolved
// inside root. It fails when a resolved path cannot be read or is a
// directory, which is bad input.
func tfVarsFileOptions(root string, files []string) ([]tfexec.PlanOption, error) {
	var planOpt []tfexec.PlanOption
	for _, path := range files {
		secureTfVarsFile, err := securejoin.SecureJoin(root, path)
		if err != nil {
			return nil, err
		}

		info, err := os.Stat(secureTfVarsFile)
		if os.IsNotExist(err) {
			return nil, &InvalidInputError{Err: fmt.Errorf("error running plan: tfvars file's path does not exist: %s", secureTfVarsFile)}
		}
		if err != nil {
			return nil, &InvalidInputError{Err: fmt.Errorf("error running plan: tfvars file's path cannot be read: %w", err)}
		}

		if info.IsDir() {
			return nil, &InvalidInputError{Err: fmt.Errorf("error running Plan: tfvars file's path is a directory: %s",
				secureTfVarsFile)}
		}

		planOpt = append(planOpt, tfexec.VarFile(secureTfVarsFile))
	}

	return planOpt, nil
}

func (r *TerraformRunnerServer) Plan(ctx context.Context, req *PlanRequest) (*PlanReply, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("creating a plan")
	s := sessionFrom(ctx)

	var planOpt []tfexec.PlanOption
	if req.Out != "" {
		planOpt = append(planOpt, tfexec.Out(req.Out))
	} else {
		// if backend is disabled completely, there will be no plan output file (req.Out = "")
		log.Info("backend seems to be disabled completely, so there will be no plan output file")
	}

	if !req.Refresh {
		planOpt = append(planOpt, tfexec.Refresh(req.Refresh))
	}

	if req.Destroy {
		planOpt = append(planOpt, tfexec.Destroy(req.Destroy))
	}

	if req.LockTimeout != "" {
		planOpt = append(planOpt, tfexec.LockTimeout(req.LockTimeout))
	}

	for _, target := range req.Targets {
		planOpt = append(planOpt, tfexec.Target(target))
	}

	varFileOpts, err := tfVarsFileOptions(req.SourceRefRootDir, s.terraform.Spec.TfVarsFiles)
	if err != nil {
		return nil, err
	}
	planOpt = append(planOpt, varFileOpts...)

	planOpt = appendPlanSpecOptions(planOpt, s.terraform.Spec.Plan)

	drifted, err := s.tf.Plan(ctx, planOpt...)
	if err != nil {
		return nil, err
	}

	planCreated := false
	if req.Out != "" {
		planCreated = true

		plan, err := s.tf.ShowPlanFile(ctx, req.Out)
		if err != nil {
			return nil, err
		}

		// This is the case when the plan is empty.
		if plan.PlannedValues.Outputs == nil &&
			plan.PlannedValues.RootModule.Resources == nil &&
			plan.ResourceChanges == nil &&
			plan.PriorState == nil &&
			plan.OutputChanges == nil {
			planCreated = false
		}

	}

	return &PlanReply{Message: "ok", Drifted: drifted, PlanCreated: planCreated}, nil
}

func (r *TerraformRunnerServer) Apply(ctx context.Context, req *ApplyRequest) (*ApplyReply, error) {
	ctrl.LoggerFrom(ctx).Info("running apply")
	s := sessionFrom(ctx)

	var applyOpt []tfexec.ApplyOption
	if req.DirOrPlan != "" {
		applyOpt = append(applyOpt, tfexec.DirOrPlan(req.DirOrPlan))
	}
	if req.RefreshBeforeApply {
		applyOpt = append(applyOpt, tfexec.Refresh(true))
	}
	for _, target := range req.Targets {
		applyOpt = append(applyOpt, tfexec.Target(target))
	}
	if req.Parallelism > 0 {
		applyOpt = append(applyOpt, tfexec.Parallelism(int(req.Parallelism)))
	}

	// The readable plan is only a log aid, so failing to print it never
	// stops the apply.
	if err := printHumanReadablePlanIfEnabled(ctx, req.DirOrPlan, s.tf.ShowPlanFileRaw); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "failed to print the human readable plan")
	}

	if err := s.tf.Apply(ctx, applyOpt...); err != nil {
		return nil, err
	}

	return &ApplyReply{Message: "ok"}, nil
}

func printHumanReadablePlanIfEnabled(ctx context.Context, planName string, tfShowPlanFileRaw func(ctx context.Context, planPath string, opts ...tfexec.ShowOption) (string, error)) error {
	if os.Getenv("LOG_HUMAN_READABLE_PLAN") == "1" {
		if planName == "" {
			planName = TFPlanName
		}

		rawOutput, err := tfShowPlanFileRaw(ctx, planName)
		if err != nil {
			return err
		}

		fmt.Println(rawOutput)
	}

	return nil
}

func (r *TerraformRunnerServer) Destroy(ctx context.Context, req *DestroyRequest) (*DestroyReply, error) {
	ctrl.LoggerFrom(ctx).Info("running destroy")
	s := sessionFrom(ctx)

	var destroyOpt []tfexec.DestroyOption
	for _, target := range req.Targets {
		destroyOpt = append(destroyOpt, tfexec.Target(target))
	}

	if err := s.tf.Destroy(ctx, destroyOpt...); err != nil {
		return nil, err
	}

	return &DestroyReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) ShowPlanFileRaw(ctx context.Context, req *ShowPlanFileRawRequest) (*ShowPlanFileRawReply, error) {
	controllerruntime.LoggerFrom(ctx).Info("show the raw plan file")

	rawOutput, err := sessionFrom(ctx).tf.ShowPlanFileRaw(ctx, req.Filename)
	if err != nil {
		return nil, err
	}

	return &ShowPlanFileRawReply{RawOutput: rawOutput}, nil
}

func (r *TerraformRunnerServer) ShowPlanFile(ctx context.Context, req *ShowPlanFileRequest) (*ShowPlanFileReply, error) {
	controllerruntime.LoggerFrom(ctx).Info("show the plan file")

	plan, err := sessionFrom(ctx).tf.ShowPlanFile(ctx, req.Filename)
	if err != nil {
		return nil, err
	}

	jsonBytes, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}

	return &ShowPlanFileReply{JsonOutput: jsonBytes}, nil
}

func (r *TerraformRunnerServer) Output(ctx context.Context, req *OutputRequest) (*OutputReply, error) {
	ctrl.LoggerFrom(ctx).Info("creating outputs")

	outputs, err := sessionFrom(ctx).tf.Output(ctx)
	if err != nil {
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

func getInventoryFromTerraformModule(m *tfjson.StateModule) []*Inventory {
	var result []*Inventory
	for _, r := range m.Resources {
		var id string
		// TODO ARN is AWS-specific. Will need to support other cloud identifiers in the future.
		// A null or non-string value is skipped.
		if val, ok := r.AttributeValues["arn"].(string); ok {
			id = val
		} else if val, ok := r.AttributeValues["id"].(string); ok {
			id = val
		}
		result = append(result, &Inventory{
			Name:       r.Name,
			Type:       r.Type,
			Identifier: id,
		})
	}

	// recursively get all resources from submodules
	for _, childModule := range m.ChildModules {
		childInventory := getInventoryFromTerraformModule(childModule)
		result = append(result, childInventory...)
	}

	return result
}

func (r *TerraformRunnerServer) GetInventory(ctx context.Context, req *GetInventoryRequest) (*GetInventoryReply, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("get inventory")

	state, err := sessionFrom(ctx).tf.Show(ctx)
	if err != nil {
		return nil, err
	}

	// state contains no values after resource destruction for example
	if state.Values == nil {
		log.Info("get inventory: state values is nil")
		return &GetInventoryReply{Inventories: []*Inventory{}}, nil
	}

	if state.Values.RootModule == nil {
		log.Info("get inventory: root module is nil")
		return &GetInventoryReply{Inventories: []*Inventory{}}, nil
	}

	return &GetInventoryReply{Inventories: getInventoryFromTerraformModule(state.Values.RootModule)}, nil
}

func (r *TerraformRunnerServer) ForceUnlock(ctx context.Context, req *ForceUnlockRequest) (*ForceUnlockReply, error) {
	if err := sessionFrom(ctx).tf.ForceUnlock(ctx, req.LockIdentifier); err != nil {
		return nil, err
	}

	return &ForceUnlockReply{
		Message: fmt.Sprintf("Successfully unlocked state with lock identifier: %s", req.GetLockIdentifier()),
	}, nil
}
