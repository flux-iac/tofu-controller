package controllers

import (
	"context"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	ctrl "sigs.k8s.io/controller-runtime"
)

// syncInventory fills status.inventory from the runner when it is enabled and
// missing, and clears it when inventory is disabled.
func (r *TerraformReconciler) syncInventory(ctx context.Context, s *runnerSession, obj *infrav1.Terraform) {
	log := ctrl.LoggerFrom(ctx)

	if !obj.Spec.EnableInventory {
		log.Info("inventory is disabled")
		obj.Status.Inventory = nil
		return
	}
	if obj.Status.Inventory != nil {
		return
	}

	var inventoryEntries []infrav1.ResourceRef

	getInventoryRequest := &runner.GetInventoryRequest{TfInstance: s.tfInstance}
	getInventoryReply, err := s.client.GetInventory(ctx, getInventoryRequest)
	if err != nil {
		log.Error(err, "error getting inventory")
		return
	}

	for _, iv := range getInventoryReply.Inventories {
		inventoryEntries = append(inventoryEntries, infrav1.ResourceRef{
			Name:       iv.GetName(),
			Type:       iv.GetType(),
			Identifier: iv.GetIdentifier(),
		})
	}
	log.Info("got inventory", "entries", len(inventoryEntries))

	if len(inventoryEntries) > 0 {
		obj.Status.Inventory = &infrav1.ResourceInventory{
			Entries: inventoryEntries,
		}
	}
}
