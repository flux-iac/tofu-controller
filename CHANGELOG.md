# Changelog

All notable changes of this project are documented in this file.

# v0.16.0-rc.3

**Release date:** 2023-09-19

We've been implementing the new feature called the Branch Planner during v0.15.x as a separate component.
And it has been included in the installation of TF-Controller since v0.16.0-rc.2.
This version also includes many improvement for the Branch Planner.
Branch Planner allows us to interact with Pull Requests to plan and review the planning process with a separate branch,
while the GitOps automation is still working on the main branch. This feature is currently Technology Preview.

**BREAKING CHANGES**

This version also introduced the lockdown mode by default.
Lockdown is the mode that enhances security for your Terraform Controller setup
by preventing the Terraform objects from referencing cross-namespace Secrets and ConfigMaps.
To relax this restriction, you can enable `--allow-cross-namespace-refs` flag at the controller level.
This setting can also be done via a Helm Chart value too.

New Features and Bug Fixing:
  * Branch Planner: Delete Terraform objects before deleting its source @chanwit
  * Branch Planner: Exclude branch name from the planning object (use PR ID as suffix instead) @chanwit
  * correct the behaviour of e2e tests @chanwit
  * Screen cross-ns refs in `.spec.cliConfigSecretRef` @squaremo
  * docs: update least required permissions for a github api token @yitsushi
  * Add install docs for GKE Autopilot @chanwit
  * Fix reconcile stuck in a loop with manual approval @yitsushi
  * Expose `--allow-cross-namespace-refs` in the chart @squaremo
  * Adds Troubleshooting Section and tip to drift detection page @LappleApple
  * Update dependency github.com/cyphar/filepath-securejoin @chanwit
  * Add flag `--allow-cross-namespace-refs` to tf-controller and branch-planner @squaremo
  * Use Pod's Subdomain-based DNS resolution @syalioune
  * Add PriorityClassName, SecurityContext and ResourceRequirements to the Runner PodSpec @luizbafilho
  * Set default observedGeneration to -1 for Kustomization Controller compatibility @luizbafilho
  * Fix `tfctl install --export` not separating yaml objects properly @matheuscscp
  * Support Flux v2.1.0 @chanwit
  * Add SECURITY.md @yiannistri
  * Add OpenSSF Scorecard @yiannistri @LappleApple
  * Improve integration tests @yitsushi @luizbafilho @squaremo @yiannistri @chanwit
  * Refactor the runner base image @chanwit
  * Do not create branch planner resources if there are no terraform changes @yitsushi
  * Docs: Add Branch Planner guide @yiannistri @chanwit
  * Prevent dependsOn crossnamespace reference @squaremo @luizbafilho
  * Build: remove `nsswitch.conf` creation @hiddeco
  * Implement Resume and Suspend All @luizbafilho
  * Build(deps): bump the ci group with 5 updates @chanwit
  * Bump libcrypto3 and libssl3 @yitsushi

# v0.16.0-rc.2

New Features and Bug Fixing:

  * Fix NPE in the Branch Planner @chanwit
  * Capture StdErr from Terraform Init and send it back to the controller @chanwit
  * Implementing the Branch Planner system @yitsushi @luizbafilho @squaremo @yiannistri @chanwit

# v0.16.0-rc.1

New Features and Bug Fixing:
  * patch: static replica count for branch planner @yitsushi
  * feat: ability to set resource limits and security context for branch planner @yitsushi
  * fix: clear comment id after replan @yitsushi
  * Add RecordDuration metrics and using functions from fluxcd @luizbafilho
  * patch: use allowedNamespaces in Branch Planner @yitsushi
  * feat: post error as comment to a pull request @yitsushi
  * Fix source deletion when using branch planner @luizbafilho
  * Improve the Plan Only mode @yitsushi

# v0.15.1

**Release date:** 2023-06-06

This version is a bug fix release for v0.15.

Bug Fixing:
  * Fix type information suffix (@chanwit)
  * Update gRPC dependency for CVE-2023-32731 (@chanwit)

# v0.15.0

**Release date:** 2023-06-04

This version is the first stable release of Terraform Controller to support Flux v2 GA.

Bug Fixing:
  * Add OIDC go-client plugin to `tfctl` (@chanwit)
  * Update documents for v0.15.0 (@chanwit)

# v0.15.0-rc.6

**Release date:** 2023-06-01

This is the last release candidate for v0.15.0.  We'll release v0.15.0 in the next release.

Breaking changes:
   * Upgrade Flux to v2.0.0-rc.5 (@chanwit)

New Features and Bug Fixing:
   * Honor pod grace period seconds in case of the controller restarting (@chanwit)
   * Add planOnly mode (@yitsushi) 
   * Add finalizer to a dependency only if object is not being deleted (@chanwit)
   * Add --no-cross-namespace-refs to the tf-controller / the lockdown mode (@squaremo)
   * Fix force-unlock in the object deletion path (@mmeha)
   * Pending plan not equal plan id in the plan storage (@chanwit)
   * Add plan log sanitization (@chanwit)

# v0.15.0-rc.5

New Features and Bug Fixing:
   * Fix logging in tf-runner (@chanwit)
   * Fix broken metrics due to the Flux v2 upgrade (@chanwit)
   * Upgrade Alpine to v3.18 (@chanwit)
   * Fix logging in terraform output (@chanwit)

# v0.15.0-rc.4

New Features and Bug Fixing:
   * Upgrade Flux to v2.0.0-rc.4 (@chanwit)

# v0.15.0-rc.3

New Features and Bug Fixing:
   * Allow passing cluster domain (default is cluster.local) (@chanwit)
   * Add host aliases to the runner pod template (@chanwit)

# v0.15.0-rc.2

New Features and Bug Fixing:
   * Update Flux APIs and use OCI HelmRepository (@stefanprodan)
   * Fix the case of no resources to destroy (@chanwit)
   * Change the default retryInterval to 15s (@chanwit)
   * Fix regression when output plan is blank (@chanwit)
   * Implement garbage collection for old cert secrets (@chanwit)
   * Add labels and annotations to outputted secrets (@scott-david-walker)
   * Fix instance label (@luizbafilho)
   * Change output type suffix to __type (@luizbafilho)
   * Implement break-the-glass mode (@chanwit)
   * Support renaming keys in varsFrom (@chanwit)
   * Support multiple version of Terraform Runners (@chanwit)

# v0.15.0-rc.1

**Release date:** 2023-04-15

This release has a notable breaking change as we started supporting Flux v2.0.0 release candidates.
Please note that you need to upgrade your Flux to v2.0.0-rc.1 or later to use this release.
And this version is not compatible with Flux v2 0.41.x or earlier.

Breaking changes:
  * Upgrade Flux to v2.0.0-rc.1 (@chanwit)
  * Bump Terraform API to v1alpha2 and deprecated v1alpha1 (@chanwit)

# v0.14.0

**Release date:** 2023-02-25

This release contains a number of new features and bug fixes. 
The most notable feature is the first-class support for Terraform Cloud in TF-controller with the `spec.cloud` field.
This feature allows Weave GitOps Enterprise users to use GitOps Templates with Terraform Cloud as a backend for your Terraform resources.
We also upgraded Flux to v0.40.0 and Terraform to v1.3.9 in this release.

New Features and Bug Fixing:
  * Add Weave GitOps metadata to the AWS package (@chanwit)
  * Fix env vars in Helm chart by enforcing quotes (@odise)
  * Improve AWS package docs (@chanwit)
  * Add servicemonitor for Helm chart (@oliverbaehler)
  * Fix missing inventory entries (@chanwit)
  * Support configuring Kube API QPS and Burst (@tariq1890)
  * Fix typo and missing links in doc (@tariq1890)
  * Update docs for replicaCount (@tariq1890)
  * Add enterprise placeholder (@chanwit)
  * Fix wrong indentations for selector labels in Helm chart (@geNAZt)
  * Update Terraform binary to v1.3.7 (@akselleirv)
  * Add parallelism option for the Terraform apply stage (@siiimooon)
  * Update Flux to v0.38 (@chanwit)
  * Force replan if the controller cannot load the plan correctly from secret (@tomhuang12)
  * Fix error an error in the doc examples (@kingdonb)
  * Refactor message trimming (@chanwit)
  * Update dependency for CVE-2022-41721 (@chanwit)
  * Enhance outputs with type information (@chanwit)
  * Add cloud spec to first-class support Terraform Cloud (@chanwit)
  * Support multi-arch images (@rparmer)
  * Allow customizing controller log encoding (@tomhuang12)
  * Upgrade Terraform to v1.3.9 and Alpine to v3.16.4 (@chanwit)
  * Upgrade Flux to v0.40 (@chanwit)

# v0.13.1

**Release date:** 2022-11-06

New Features and Bug Fixing:
  * Update Source controller to v0.31.0 / Flux v0.36.0 (@chanwit)
  * Improve `tfctl` commands (@chanwit)

# v0.13.0

**Release date:** 2022-10-27

A notable feature in this version is the first-class YAML support for Terraform.
A Terraform object in v0.13.0+ allows you to better configure your Terraform resources via YAMLs, 
without introducing any extra CRDs to your cluster. 

Together with a new generator, Tofu-Jet, we'll now be able to ship pre-generated 
primitive Terraform modules for all major cloud providers. We shipped the alpha version of AWS package in this release.

A primitive Terraform module is a module that only contains a single primitive resource,
like `aws_iam_role`, or `aws_iam_policy`. With this concept, we would be able to use Terraform
without writing Terraform codes, and make it more GitOps-friendly at the same time.

New Features and Bug Fixing:
  * Implement webhooks for Terraform stages (@chanwit)
  * Add use case examples (@tomhuang12)
  * Add `.spec.workspace` field (@k0da)
  * Add the default value to workspace (@k0da)
  * Implement `spec.values` and map it to Terraform HCL (@chanwit)
  * Add docs for preflight checks (@chanwit)
  * Implement Helm-like template for Terraform files (@chanwit)
  * Add runner Dockerfile for Azure (@tomhuang12)
  * Upgrade Golang to v1.19 (@chanwit)
  * Bundle an alpha version AWS Package (@chanwit)
  * Fix e2e (@chanwit)
  * Implement init containers support on the runner pod (@Nalum)
  * Implement `spec.dependsOn` and watch for the output secret changes (@chanwit)
  * Implement templating for input references (@chanwit)
  * Fix the check of dependencies by taking the output secret into account (@chanwit)
  * Add tests for the `spec.dependsOn` feature (@chanwit)
  * Change templating delimiter to `${{ }}` (@chanwit)
  * Add labels to "tfstate" via the K8s backend so that we can group them by the labels (@chanwit)
  * Fix dependency in the finalizer (@chanwit)
  * Add an ability to Helm chart for creating service accounts in each namespace (@adamstrawson)
  * Parameterize AWS package in chart (@k0da)
  * Add trace logging (@Nalum)
  * Fix runner service account template not returning multiple docs (@skeletorXVI)
  * Implement "replan" to avoid double planning (@chanwit)
  * Add SHA and version information to the binaries (@chanwit)

# v0.12.0

**Release date:** 2022-09-07

This release contains a number of new features and bug fixes.

New Features and Bug Fixing:
  * Enable custom backends for Terraform (@fsequeira1)
  * Support `backendConfigsFrom` for specifying backend configuration from Secrets (@chanwit)
  * Add a parameter for specifying max gRPC message size, default to 4MB (@chanwit)
  * Implement force-unlock for tfstate management (@Nalum)
  * Fix the initialization status (@chanwit)
  * Recording events to support Flux notification controller (@chanwit)
  * Support specifying targets for plan and apply (@akselleirv)
  * Add node selector, affinity and tolerations for the runner pod (@Nalum)
  * Add volume and volumeMounts for the runner pod (@steve-fraser)
  * Add file mapping to map files from Secrets to home or workspace directory (@itamar-marom)
  * Fix Plan prompt being overridden by the progressing message (@chanwit)
  * Support storing human-readable plan output in a ConfigMap (@chanwit)

# v0.11.0

**Release date:** 2022-08-12

This release is another milestone of the project as it is the first release of TF-controller
that supports Flux's OCIRepository.

New Features and Bug Fixing:
  * Added support for Flux's OCIRepository (@chanwit)
  * Fixed EnvVars to pick up `valueFrom` to work with Secrets and ConfigMaps (@Nalum)
  * Fixed tfctl to show plan in the working directory (@github-vincent-miszczak)
  * Updated tfexec to v0.16.1 for the force-lock option (@chanwit)
  * Updated the Source controller to v0.26.1 (@chanwit)

# v0.10.1

**Release date:** 2022-08-05

This release is a huge improvement as we have successfully reconciled 1,500 Terraform modules concurrently.
This pre-release contains the following changes.

Bug Fixing:
  * Fix pod deletion process (@chanwit)
  * Make the gRPC dial process more reliable (@chanwit)
  * Add the runner pod creation timeout, default at 5m0s (@chanwit)
  * Fix another race condition secret (@chanwit)
  * Map runner's home to a volume to make it writeable (@chanwit)

# v0.10.0

**Release date:** 2022-08-02

This pre-release contains the following changes.

New Features and Bug Fixing:
  * Add support for Terraform Enterprise (@chanwit)
  * Implement resource inventory (@chanwit)
  * Improve security to make the images work with Weave GitOps Enterprise (@chanwit)
  * Re-implement certificate rotator (@chanwit)
  * Correct IRSA docs (@benreynolds-drizly)
  * Update Kubernetes libraries to v0.24.3 (@chanwit)
  * Update go-restful to fix CVE-2022-1996 (@chanwit)
  * Add pprof to the /debug/pprof endpoint (@chanwit)
  * Fix race condition to make sure that gRPC client and the runner use the same TLS (@chanwit)

# v0.9.5

**Release date:** 2022-05-30

This pre-release contains the following changes.

New Features:
  * Update Terraform binary to 1.1.9 (@chanwit)
  * Allow runner pod metadata customization (@tomhuang12)
  * Support runner pod environment variables specification (@Nalum)
  * Implement `.spec.refreshBeforeApply` to refresh the state before apply (@chanwit)
  * Use controller runtime logging library in runner (@chanwit)

Bug Fixing:
  * Fix nil reference for event recorder (@chanwit)
  * Fix insertion of sensitive information to runner pod logging (@chanwit)
  * Fix nil reference for in writeBackendConfig (@chanwit)

# v0.9.4

**Release date:** 2022-04-15

This pre-release contains the following changes.

New Features and Bug Fixing:
  * Fix Helm chart to support image pull secrets for `tf-runner` Service Accounts (@Nalum)
  * Upgrade Source Controller API to v0.22.4 (@tomhuang12)
  * Fix json bytes encoding (@phoban01)
  * Add Helm chart an option to specify AWS Security Group policy (@Nalum)
  * Move plan revision from labels to annotations (@Nalum)
  * Update images to include fix for CVE-2022-28391 (@chanwit)
  * Update Terraform binary to 1.1.8 (@chanwit)

# v0.9.3

**Release date:** 2022-03-28

This pre-release contains the following changes.

Bug Fixing:
  * Fix runner pod pointer variables so that getting pods works correctly (@chanwit)

# v0.9.2

**Release date:** 2022-03-25

This pre-release contains the following changes.

Bug Fixing:
  * Wait for runner pods to be completely terminated before reconcile a new one (@chanwit)

# v0.9.0

**Release date:** 2022-03-21

This pre-release contains the following changes.

New Features and Bug Fixing:
  * Network-based health checks (@tomhuang12)
  * Improved drift detection status (@phoban01)
  * Add FOSSA and CodeQL scan (@tomhuang12)
  * Support HCL variables (@phoban01)
  * Implemented local gRPC runner (@chanwit)
  * Update source-controller to v0.21.1 (@tomhuang12)
  * Add Trivy scan (@tomhuang12)
  * Change image repository to ghcr.io/weaveworks (@phoban01)
  * Move repository to Weaveworks (@chanwit)
  * Add E2E Tests (@tomhuang12)
  * Moved charts to `weaveworks/tf-controller` (@tomhuang12)
  * Add local http server for http health check (@tomhuang12)
  * Update Terraform version (@fsequeira1)
  * Update the Helm repository URL (@tomhuang12)
  * Implement CA certification generation and mTLS rotation (@phoban01)
  * Add documentation site (@tomhuang12)
  * Implemented runner pods (@chanwit)
  * Add multi-tenancy E2E test case (@tomhuang12)
  * Implemented output name mapping (@chanwit)
  * Improve test coverage (@tomhuang12)
  * Fix documents GitHub Actions (@tomhuang12)
  * Set the default Service Account to `tf-runner` (@tomhuang12)
  * Make certification rotation configurable (@tomhuang12)
  * Implement gRPC retry policy (@chanwit)
  * Implement TLS validation and graceful shutdown (@phoban01)
  * Fix certification validation (@chanwit)
  * Fix drifted outputs (@chanwit)
  * Change the behaviour of pod cleanup to be default (@chanwit)
  * Add concurrency to the Helm chart (@chanwit)
  * Fix CVE-2022-0778 (@chanwit)

# v0.8.0

**Release date:** 2022-01-24

This pre-release contains the following changes.

Breaking Changes:
  * Change `.spec.varsFrom` from object to be an array of object in `v1alpha1`. Users require to update their Terraform objects.

New Features:
  * Helm chart (@tomhuang12)
  * Allow many instances of `.spec.varsFrom` (@phoban01)
  * Add the Drift Detection only mode (@phoban01)
  * Upgrade Terraform to 1.1.4 (@chanwit)

Improvements:
  * Fix recording the status of destroy plans (@chanwit)

# v0.7.0

**Release date:** 2022-01-16

This pre-release contains the following changes.

New Features:
  * Add flag to allow disabling drift detection `.spec.disableDriftDetection` (@phoban01)
  * Add field to allow specifying TF client configuration `.spec.cliConfigurationSecretRef` and disable backend completely `.spec.backendConfig.disable` (@chanwit)
  
Improvements:
  * Add documentation on how to use TF-controller with EKS IRSA (@phoban01)
  * Support gzip encoding for `tfplan` (@tomhuang12)
  * Improve re-plan behaviour (@chanwit)

## v0.6.0

**Release date:** 2022-01-12

This pre-release contains the following changes.

Improvements:
  * Correct the manual approval behaviour, as amended in [tc000030](controllers/tc000030_plan_only_no_outputs_test.go).
  * Upgrade Go to 1.17.6
  * Upgrade Source controller to v0.20.1
  * Support Flux v2 0.25.x

## v0.5.2

**Release date:** 2022-01-11

This pre-release contains the following changes.

Improvements:
  * Improve UX for plan generation message. Plan message now includes an action with the short form of the plan id, as amended in [tc000050](controllers/tc000050_plan_and_manual_approve_no_outputs_test.go).

## v0.5.1

**Release date:** 2022-01-10

This pre-release contains the following changes.

Improvements:
  * Improve UX for plan generation message. Plan name is shown in the message, so that it can be used for an approval, as amended in [tc000050](controllers/tc000050_plan_and_manual_approve_no_outputs_test.go).

## v0.5.0

**Release date:** 2022-01-09

This pre-release contains the following changes.

Improvements:
  * Improve status messages.
  * `shouldDetectDrift` is now specified by [tc000180](controllers/tc000180_should_detect_drift_test.go)

## v0.4.3

**Release date:** 2022-01-09

This pre-release contains the following changes.

Improvements:
  * Improve `shouldDetectDrift` logic for new objects

## v0.4.2

**Release date:** 2022-01-09

This pre-release contains the following changes.

Improvements:
  * Add resource's shortName
  * Add display columns to the CRD.

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
