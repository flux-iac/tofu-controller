# Changelog

All notable changes of this project are documented in this file.

## v0.16.0-rc.6

**Release date:** 2025-11-25

This is the second release candidate of Tofu Controlle this calendar year, and contains various new features and bug fixes.

We would like to thank our contributors for their continued support and effort in improving the Tofu-Controller.

Please follow the [upgrade guide in the documentation](https://flux-iac.github.io/tofu-controller/use-tf-controller/upgrade-tf-controller/) to ensure a smooth transition to the latest version.

New Features and Bug Fixing:

- feat: controller refactoring ([#1640](https://github.com/flux-iac/tofu-controller/pull/1640)) ([38a6bd1c](https://github.com/flux-iac/tofu-controller/commit/38a6bd1cf87bafa196b8b92dd62bd7bf7572cf2a))
- chore(docs): update docs to refer to CNCF Slack Channel ([#1644](https://github.com/flux-iac/tofu-controller/pull/1644)) ([be1c0445](https://github.com/flux-iac/tofu-controller/commit/be1c044561e286a6f7ce224832b14dd3166a4353))
- feat: enable controller priority queue ([13b5f452](https://github.com/flux-iac/tofu-controller/commit/13b5f452a75fff5ad3fb67f7688fd91b576ce838))
- fix(docker): use native build platform for go build stages ([#1636](https://github.com/flux-iac/tofu-controller/pull/1636)) ([516a098f](https://github.com/flux-iac/tofu-controller/commit/516a098f87b3e3f4544d4e1bf023cf56440abe38))
- chore: add self to MAINTAINERS ([#1637](https://github.com/flux-iac/tofu-controller/pull/1637)) ([59238cae](https://github.com/flux-iac/tofu-controller/commit/59238caefa011e4b57cf0a5493c4abbfe599bf01))
- fix(ci): speed up docker builds ([#1635](https://github.com/flux-iac/tofu-controller/pull/1635)) ([96eb5290](https://github.com/flux-iac/tofu-controller/commit/96eb52904bb8103bac13a8f11cf322d4b85fb75b))
- docker: drop unused libretls ([#1634](https://github.com/flux-iac/tofu-controller/pull/1634)) ([3b1e6953](https://github.com/flux-iac/tofu-controller/commit/3b1e69535ea0540ae2a888d566fa8889d94cc391))
- chore: move condition types and reasons, and add comments ([#1633](https://github.com/flux-iac/tofu-controller/pull/1633)) ([0eaef3b6](https://github.com/flux-iac/tofu-controller/commit/0eaef3b6509d1bc4cadb5bc24a3074460fb7f0dd))
- fix(chart): handling additional deployment labels ([#1632](https://github.com/flux-iac/tofu-controller/pull/1632)) ([f6ee8377](https://github.com/flux-iac/tofu-controller/commit/f6ee837716a58339a6911ec270b53540c351fc98))
- docs: fix code display on exposed using hostname subdomain page ([#1540](https://github.com/flux-iac/tofu-controller/pull/1540)) ([bc4ee033](https://github.com/flux-iac/tofu-controller/commit/bc4ee0338bba14ef5e3a8c2fb3b80f23fa2d4110))
- fix(ci): avoid e2e race condition on cert gc ([978bf29e](https://github.com/flux-iac/tofu-controller/commit/978bf29e2410df398051efe8a4694b03fb5ebca5))
- chore: upgrade controller-runtime to v0.22.0 and their dependencies ([#1602](https://github.com/flux-iac/tofu-controller/pull/1602)) ([e4060875](https://github.com/flux-iac/tofu-controller/commit/e4060875faa130171f4759750ab1be4f2d5416d8))
- fix: implement a Terraform Exec Wrapper to detect State Lock Errors ([#1623](https://github.com/flux-iac/tofu-controller/pull/1623)) ([b67e9c7a](https://github.com/flux-iac/tofu-controller/commit/b67e9c7a93cefb80c902b261060b95e8b1e6477f))
- fix: implement more descriptive errors during instance id mismatches ([#1622](https://github.com/flux-iac/tofu-controller/pull/1622)) ([536b9b4a](https://github.com/flux-iac/tofu-controller/commit/536b9b4a415575ffaa12565468d1400f21d0d815))
- Bump the deprecated FluxCD versions ([#1539](https://github.com/flux-iac/tofu-controller/pull/1539)) ([446d3b13](https://github.com/flux-iac/tofu-controller/commit/446d3b139c48afe0d7bd34f561fdcb609227efce))
- Bump github.com/docker/docker ([#1594](https://github.com/flux-iac/tofu-controller/pull/1594)) ([13c7438d](https://github.com/flux-iac/tofu-controller/commit/13c7438d4d46cdb5ed57e781b37fd3397b40e6aa))
- chore: upgrade to go 1.24 + upgrade deps and tooling ([#1588](https://github.com/flux-iac/tofu-controller/pull/1588)) ([7d88a637](https://github.com/flux-iac/tofu-controller/commit/7d88a6374fd329be36a8085a2b7491c52eb1132a))
- Bump golang.org/x/net from 0.34.0 to 0.38.0 ([#1562](https://github.com/flux-iac/tofu-controller/pull/1562)) ([d0b0910a](https://github.com/flux-iac/tofu-controller/commit/d0b0910ad33d432046aaa47850552cfbf3f9bc38))
- Bump golang.org/x/crypto from 0.32.0 to 0.35.0 ([#1561](https://github.com/flux-iac/tofu-controller/pull/1561)) ([358235ee](https://github.com/flux-iac/tofu-controller/commit/358235eeeac210ec21e47fb8c6ac082cfc9094dd))
- chore: bump libcrypto to 3.3.5-r0 and alpine to 3.22 ([#1583](https://github.com/flux-iac/tofu-controller/pull/1583)) ([d3d858be](https://github.com/flux-iac/tofu-controller/commit/d3d858bebc26d844357df8d7fa16b26f3ae9b9b8))
- Fix broken github link ([#1578](https://github.com/flux-iac/tofu-controller/pull/1578)) ([5c7c6420](https://github.com/flux-iac/tofu-controller/commit/5c7c64209d12e3127cd9dddf391b9c88095e45f2))
- Bump the gh-minor group across 1 directory with 17 updates ([#1548](https://github.com/flux-iac/tofu-controller/pull/1548)) ([67198a27](https://github.com/flux-iac/tofu-controller/commit/67198a27f35b97119492965ea1fe7a9034cc4476))
- Rename Weave GitOps to tofu-controller ([#1549](https://github.com/flux-iac/tofu-controller/pull/1549)) ([da8f516a](https://github.com/flux-iac/tofu-controller/commit/da8f516a6569a925bea6645fa4a3e766eeec91af))
- Replace github.com/pkg/errors with errors wrapping using stdlib ([#1526](https://github.com/flux-iac/tofu-controller/pull/1526)) ([90ae7db8](https://github.com/flux-iac/tofu-controller/commit/90ae7db8a627e37ff77409e8a0fbd9227dfd7781))
- chore: bump libcrypto to 3.3.3-r0 ([#1525](https://github.com/flux-iac/tofu-controller/pull/1525)) ([88287639](https://github.com/flux-iac/tofu-controller/commit/88287639ba5c5605279250e79dab96856f76bd9b))
- fix(oci): allow unlimited layer size ([#1519](https://github.com/flux-iac/tofu-controller/pull/1519)) ([a5c2ca77](https://github.com/flux-iac/tofu-controller/commit/a5c2ca77acabe8d937ca7304b3f4ba4232ef5fc7))
- Bump the go-patch group across 3 directories with 11 updates ([#1518](https://github.com/flux-iac/tofu-controller/pull/1518)) ([36cfee53](https://github.com/flux-iac/tofu-controller/commit/36cfee53c18d9b19bc03f958a2a6cc09b157a139))
- Bump actions/checkout from 4.2.0 to 4.2.2 in the gh-patch group across 1 directory ([#1517](https://github.com/flux-iac/tofu-controller/pull/1517)) ([b07f185a](https://github.com/flux-iac/tofu-controller/commit/b07f185a253f8fe32123e6b343ddd6aad55e0c47))
- fix(ci): setup terraform ([#1510](https://github.com/flux-iac/tofu-controller/pull/1510)) ([f4893afd](https://github.com/flux-iac/tofu-controller/commit/f4893afda8d7e1dec36299216852bc9fef39e3ad))
- Control init -upgrade behaviour ([#1471](https://github.com/flux-iac/tofu-controller/pull/1471)) ([7ca23dc1](https://github.com/flux-iac/tofu-controller/commit/7ca23dc1c64f15bb6ca69ef3c8173eaca7313667))
- Fix the HelmRelease scripts to install the latest helm chart ([#1509](https://github.com/flux-iac/tofu-controller/pull/1509)) ([4c3c1552](https://github.com/flux-iac/tofu-controller/commit/4c3c1552346f3171f0735429e4dae6b2be69789b))

## v0.16.0-rc.5

**Release date:** 2025-01-14

This release introduces several enhancements, fixes, and updates to improve functionality and stability. It also addresses ongoing community feedback and includes dependency updates to keep the project secure and up-to-date.

We would like to thank our contributors for their continued support and effort in improving the Tofu-Controller.

Please follow the [upgrade guide in the documentation](https://flux-iac.github.io/tofu-controller/use-tf-controller/upgrade-tf-controller/) to ensure a smooth transition to the latest version.

**BREAKING CHANGES**

* Helm Chart Renaming: The Helm chart has been renamed from tf-controller to tofu-controller.

New Features and Bug Fixing:

  * add private registries integration docs by @ArieLevs in https://github.com/flux-iac/tofu-controller/pull/1237
  * helm-chart: add watchAllNamespaces argument by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1239
  * bump chart and doc yamls to v0.16.0-rc.4 by @chanwit in https://github.com/flux-iac/tofu-controller/pull/1242
  * Fix runner-serviceaccount helm template by @ayanevbg in https://github.com/flux-iac/tofu-controller/pull/1251
  * Update Repo URLs in Docs by @tech1ndex in https://github.com/flux-iac/tofu-controller/pull/1253
  * Update development docs by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1248
  * Add Aksel Skaar Leirvaag (@akselleirv) as a MAINTAINER by @chanwit in https://github.com/flux-iac/tofu-controller/pull/1258
  * #1247 helm readme update to tofo-controller by @dgem in https://github.com/flux-iac/tofu-controller/pull/1260
  * Helm chart rename by @ilithanos in https://github.com/flux-iac/tofu-controller/pull/1264
  * Tilt: fix references to helm chart after renaming by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1272
  * branch planner is now checking for both PostPlanningWebhookFailedReason and TFExecInitFailedReason for failing PR by @raz-bn in https://github.com/flux-iac/tofu-controller/pull/1271
  * docs: remove weaveworks.github.io references from docs by @zonorti in https://github.com/flux-iac/tofu-controller/pull/1276
  * Improve docs by @vishu42 in https://github.com/flux-iac/tofu-controller/pull/1275
  * fix(helm-chart): missing renamings by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1282
  * fix: build of tf-runner-azure by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1290
  * Fix Incorrect Metric Reporting Post-Update in Terraform Controller by @TarasLykhenko in https://github.com/flux-iac/tofu-controller/pull/1287
  * Removed mentions of team wild-watermelon by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1299
  * Speedup wait for pod ip by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1302
  * Move hardcoded "flux-system" namespace from templates to default values by @artem-nefedov in https://github.com/flux-iac/tofu-controller/pull/1303
  * Enable non-security dependency upgrades by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1305
  * Enable dependabot for github actions by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1311
  * Upgrade go version to 1.22 and set version via go.mod in workflow by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1310
  * Bump aws-sdk-go-v2 deps and fix deprecated usage by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1331
  * Print plan before apply by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1304
  * Bump controller-runtime and k8s.io/* by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1334
  * bump libcrypto to 3.1.6-r2 by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1401
  * bump github.com/fluxcd/pkg/ssa to v0.39.1 by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1402
  * Delete stale metrics on object delete  by @TarasLykhenko in https://github.com/flux-iac/tofu-controller/pull/1362
  * fix(helm-chart): ensure helm release namespace is applied by @mloiseleur in https://github.com/flux-iac/tofu-controller/pull/1425
  * Bump alpine and libcrypto by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1439
  * feat(helm-chart): Add additionalLabels for deployments by @adberger in https://github.com/flux-iac/tofu-controller/pull/1400
  * docs: provide instructions for autocomplete by @mloiseleur in https://github.com/flux-iac/tofu-controller/pull/1448
  * fix(helm-chart): rbac for branch planner by @mloiseleur in https://github.com/flux-iac/tofu-controller/pull/1447
  * Added exponential backoff on reconciliation failure by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1460
  * Fix deprecated usage of the k8s.io/apimachinery/pkg/util/wait package by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1461
  * README.md: remove roadmap by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1459
  * Upgrade Go version to 1.23.4 and deps by @akselleirv in https://github.com/flux-iac/tofu-controller/pull/1502

## v0.16.0-rc.4

**Release date:** 2024-03-14

This is the first release of the project after rebooting under its new name and organization: Tofu Controller, now part of the Flux-IaC organization. Fully driven by our community, Tofu Controller and Flux-IaC aim to help innovate the development of Infrastructure as Code (IaC) controllers for Flux.

Thank you so much to our vibrant community, which propelled us to reach 1,000 stars on GitHub recently.

With the renaming of the controller, our community has identified several breaking changes, although some may have been missed. The transition from Weave TF-Controller to Flux-IaC Tofu-Controller could be challenging. We advise:

  * Backing up your Terraform states (tfstates)
  * Setting `spec.destroyResourcesOnDeletion=false` to avoid unintentional resource deletion
  * Pausing all Terraform CRs

before doing the upgrade.

**BREAKING CHANGES**

  * The renaming of the controller.
  * Reorganization of CRDs in the Helm Chart, which may lead to their uninstallation and reinstallation.

New Features and Bug Fixing:

  * Pass missing build arg TARGETARCH to docker-build.
  * Implement BLOB encryption within the tf-runner.
  * Add `tfvars` feature and API.
  * Generate checksum for cache blobs.
  * Implement remediation retry.
  * Add unique hash to cloned source to avoid conflict.
  * Speed up compiling of binaries.
  * Added config for building tf-runner image and using it in helloworld example.
  * Bump version in manifest used in user guide to reflect latest RC.
  * Document a fix to "terraform objects stuck on deletion" issue.
  * Fix docker build breaking due to LIBCRYPTO_VERSION.
  * Fix issues surfaced around the polling server.
  * Add documentation about IPv6 support.
  * Update branch planner default configuration.

## v0.16.0-rc.3

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

## v0.16.0-rc.2

New Features and Bug Fixing:

  * Fix NPE in the Branch Planner @chanwit
  * Capture StdErr from Terraform Init and send it back to the controller @chanwit
  * Implementing the Branch Planner system @yitsushi @luizbafilho @squaremo @yiannistri @chanwit

## v0.16.0-rc.1

New Features and Bug Fixing:
  * patch: static replica count for branch planner @yitsushi
  * feat: ability to set resource limits and security context for branch planner @yitsushi
  * fix: clear comment id after replan @yitsushi
  * Add RecordDuration metrics and using functions from fluxcd @luizbafilho
  * patch: use allowedNamespaces in Branch Planner @yitsushi
  * feat: post error as comment to a pull request @yitsushi
  * Fix source deletion when using branch planner @luizbafilho
  * Improve the Plan Only mode @yitsushi

## v0.15.1

**Release date:** 2023-06-06

This version is a bug fix release for v0.15.

Bug Fixing:
  * Fix type information suffix (@chanwit)
  * Update gRPC dependency for CVE-2023-32731 (@chanwit)

## v0.15.0

**Release date:** 2023-06-04

This version is the first stable release of Terraform Controller to support Flux v2 GA.

Bug Fixing:
  * Add OIDC go-client plugin to `tfctl` (@chanwit)
  * Update documents for v0.15.0 (@chanwit)

## v0.15.0-rc.6

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

## v0.15.0-rc.5

New Features and Bug Fixing:
   * Fix logging in tf-runner (@chanwit)
   * Fix broken metrics due to the Flux v2 upgrade (@chanwit)
   * Upgrade Alpine to v3.18 (@chanwit)
   * Fix logging in terraform output (@chanwit)

## v0.15.0-rc.4

New Features and Bug Fixing:
   * Upgrade Flux to v2.0.0-rc.4 (@chanwit)

## v0.15.0-rc.3

New Features and Bug Fixing:
   * Allow passing cluster domain (default is cluster.local) (@chanwit)
   * Add host aliases to the runner pod template (@chanwit)

## v0.15.0-rc.2

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

## v0.15.0-rc.1

**Release date:** 2023-04-15

This release has a notable breaking change as we started supporting Flux v2.0.0 release candidates.
Please note that you need to upgrade your Flux to v2.0.0-rc.1 or later to use this release.
And this version is not compatible with Flux v2 0.41.x or earlier.

Breaking changes:
  * Upgrade Flux to v2.0.0-rc.1 (@chanwit)
  * Bump Terraform API to v1alpha2 and deprecated v1alpha1 (@chanwit)

## v0.14.0

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

## v0.13.1

**Release date:** 2022-11-06

New Features and Bug Fixing:
  * Update Source controller to v0.31.0 / Flux v0.36.0 (@chanwit)
  * Improve `tfctl` commands (@chanwit)

## v0.13.0

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

## v0.12.0

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

## v0.11.0

**Release date:** 2022-08-12

This release is another milestone of the project as it is the first release of TF-controller
that supports Flux's OCIRepository.

New Features and Bug Fixing:
  * Added support for Flux's OCIRepository (@chanwit)
  * Fixed EnvVars to pick up `valueFrom` to work with Secrets and ConfigMaps (@Nalum)
  * Fixed tfctl to show plan in the working directory (@github-vincent-miszczak)
  * Updated tfexec to v0.16.1 for the force-lock option (@chanwit)
  * Updated the Source controller to v0.26.1 (@chanwit)

## v0.10.1

**Release date:** 2022-08-05

This release is a huge improvement as we have successfully reconciled 1,500 Terraform modules concurrently.
This pre-release contains the following changes.

Bug Fixing:
  * Fix pod deletion process (@chanwit)
  * Make the gRPC dial process more reliable (@chanwit)
  * Add the runner pod creation timeout, default at 5m0s (@chanwit)
  * Fix another race condition secret (@chanwit)
  * Map runner's home to a volume to make it writeable (@chanwit)

## v0.10.0

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

## v0.9.5

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

## v0.9.4

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

## v0.9.3

**Release date:** 2022-03-28

This pre-release contains the following changes.

Bug Fixing:
  * Fix runner pod pointer variables so that getting pods works correctly (@chanwit)

## v0.9.2

**Release date:** 2022-03-25

This pre-release contains the following changes.

Bug Fixing:
  * Wait for runner pods to be completely terminated before reconcile a new one (@chanwit)

## v0.9.0

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

## v0.8.0

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

## v0.7.0

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
