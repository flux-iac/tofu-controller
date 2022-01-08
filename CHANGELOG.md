# Changelog

All notable changes of this project are documented in this file.

## v0.4.1

**Release date:** 2022-01-09

This pre-release contains the following changes.

Improvements:
  * Improve `shouldDetectDrift` logic. 
  * Improve the behaviour of the auto mode t0 be started over from planning when the `apply` process fail because of entity exists externally, as specified by [tc000170](controllers/tc000170_if_apply_error_we_should_delete_the_plan_and_start_over_test.go).

## v0.4.0

**Release date:** 2022-01-08

This pre-release contains the following changes.

Improvements:
  * Introduce `.status.lastPlannedRevision` to handle a case of source changes by non TF-files, as specified by [tc000160](controllers/tc000160_auto_applied_should_tx_to_plan_when_unrelated_source_changed_test.go)
  * Improve `shouldDetectDrift` logic using `.status.lastPlannedRevision`.
  
## v0.3.1

**Release date:** 2022-01-07

This pre-release ships with the following changes.

Improvements:
  * Upgrade the Terraform binary tp v1.1.3.
  * Improve the spec documentation of the test files. 

## v0.3.0

**Release date:** 2022-01-05

This pre-release ships with the implementation of the following features. 

New Features:
  * The ability to apply Terraform in the `auto` approval mode, as specified by [tc000010](controllers/tc000010_no_outputs_test.go).
  * Support backend configuration, as specified by [tc000020](controllers/tc000020_with_backend_no_outputs_test.go).
  * The ability to `plan` Terraform, as specified by [tc000030](controllers/tc000030_plan_only_no_outputs_test.go), and [tc000050](controllers/tc000050_plan_and_manual_approve_no_outputs_test.go).
  * Support outputs and selection of those outputs as secrets, as specified by [tc000040](controllers/tc000040_controlled_outputs_test.go) and [tc000041](controllers/tc000041_all_outputs_test.go).
  * Support variables, also from `Secrets` and `ConfigMaps`, as specified by [tc000060](controllers/tc000060_vars_and_controlled_outputs_test.go), [tc000070](controllers/tc000070_varsfrom_secret_and_controlled_outputs_test.go), [tc000080](controllers/tc000080_varsfrom_configmap_and_controlled_outputs_test.go), [tc000090](controllers/tc000090_varsfrom_override_and_controlled_outputs_test.go).
  * The ability to reconcile when Source changes, as specified by [tc000100](controllers/tc000100_applied_should_tx_to_plan_when_source_changed_test.go), and [tc000110](controllers/tc000110_auto_applied_should_tx_to_plan_then_apply_when_source_changed_test.go).
  * Resource deletion implementation, as specified by [tc000120](controllers/tc000120_delete_test.go).
  * Support the `destroy` mode, as specified by [tc000130](controllers/tc000130_destroy_no_outputs_test.go).
  * Support drift detection, as specified by [tc000140](controllers/tc000140_auto_applied_should_tx_to_plan_then_apply_when_drift_detected_test.go) and [tc000150](controllers/tc000150_manual_apply_should_report_and_loop_when_drift_detected_test.go).
