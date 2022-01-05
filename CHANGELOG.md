# Changelog

All notable changes to this project are documented in this file.

## 0.3.0

**Release date:** 2022-01-05

This pre-release ships with the implementation of the following features. 

New Features:
  * The ability to apply Terraform in the `auto` approval mode, as specified by [tc000010](controllers/tc000010_no_outputs_test.go)
  * Support backend configuration, as specified by [tc000020](controllers/tc000020_with_backend_no_outputs_test.go).
  * The ability to `plan` Terraform, as specified by [tc000030](controllers/tc000030_plan_only_no_outputs_test.go), and tc000050.
  * Support outputs and selection of those outputs as secrets, as specified by tc000040 and tc000041.
  * Support variables, also from `Secrets` and `ConfigMaps`, as specified by tc000060, tc000070, tc000080, tc000090.
  * The ability to reconcile when Source changes, as specified by tc000100, and tc000110.
  * Resource deletion implementation, as specified by tc000120.
  * Support the `destroy` mode, as specified by tc000130.
  * Support drift detection, as specified by tc000140 and tc000150.
