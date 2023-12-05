# Troubleshooting with Interval and retryInterval

## Overview
This document describes the requeue behavior of the Reconcile method in the TerraformReconciler struct in the code base. 
Understanding these behaviors can be crucial for troubleshooting, as well as for future development and refinement of the system.

## Requeue Behaviors
The Reconcile method has several requeue behaviors based on different conditions and errors. 
We will group them into four categories based on their requeue behavior:

### 1. Immediate Requeue (Not using specified interval / retryInterval)

In these scenarios, the `Reconcile` method returns an error which leads to an immediate requeue orchestrated by the Controller Runtime.
The interval is based on the controller's configuration and not specified in the method itself:

 - When there's an error retrieving the Terraform object from the Kubernetes API.
 - After adding the finalizer, if there's an error in patching the Terraform object.
 - If there's a non-access-denied error in retrieving the source object.
 - When the ready condition is unknown or the status of the ready condition isn't unknown, and there's an error in patching the Terraform object.
 - In multiple situations where there's an error in patching the Terraform object's status.
 - If there's an error in creating or looking up the runner.
 - If there's an error while attempting to finalize the Terraform object.

### 2. Requeue After a Specific Interval (`spec.retryInterval`)
In these scenarios, the method specifically asks for a requeue after a certain interval specified by `spec.retryInterval` (default to 15s).
 
 - The Terraform object is being deleted but there are still dependent resources that haven't been deleted.
 - The source object specified by `spec.sourceRef` is not found.
 - The source object doesn't have an associated artifact.
 - The dependencies do not meet the ready condition.
 - There's an error during the main reconciliation process.
 - Drift is detected during the reconciliation process.
 
### 3. Requeue After a Specific Interval (`spec.interval`)

In this scenario, the method specifically asks for a requeue after a successful reconciliation:

The interval for the requeue is `spec.interval`.

### 4. No Requeue, wait for manual intervention

In these scenarios, the method returns without asking for a requeue, 
and the Controller Runtime will stop the reconciliation process until there is a manual intervention:

 - Access is denied when retrieving the source object.
 - The status of the plan is pending, and it's not set to force or auto-apply.
 