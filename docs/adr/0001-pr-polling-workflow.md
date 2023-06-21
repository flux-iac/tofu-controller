# 1. Pull Request Polling Workflow

* Status: proposed
* Date: 2023-06-20 
* Authors: @yitsushi
* Deciders: @yitsushi @chanwit @squaremo @yiannistri

## Context

To detect pull request changes, we can use webhooks or polling using GitHub's
API.

## Decision

We decided to use polling for security reasons. Using webhook would require to
open an ingress to the cluster and that's not acceptable especially in an
air-gapped environment.

The Branch-Based Planner has two components:

1. Polling Server: Detect Pull Request changes and manage Teraform resource
   state.
2. Informer: Make a comment when new plan output is available.


Full workflow and responsibilities:

1. The poller reads information from Pull Requests (PRs).
2. Using the PR information, the poller creates a `Terraform` Custom Resource
   with `planOnly=true` and sets labels. When new `Terraform` resources are
   created, they are done so with `planOnly=true`, which means they are not
   `terraform apply`, only planned.
3. A `GitRepository` object is also created by the poller which points to the
   branch of the PR.
4. The poller ensures that every PR of interest is associated with
   a `GitRepository` and `Terraform` object, and it also triggers "replans"
   when necessary.
5. The informer is responsible for watching a set of `Terraform` Custom
   Resources with the labels set by the poller.
6. When there's a new commit in the PR branch, Flux's source controller automatically detects the change and updates the source.
7. In the case of specific comments like `!restart` or `!replan`, the poller
   initiates a "force restart" of the plan. This triggers a "replan".
8. The informer is also responsible for relaying the `Terraform` plan back to
   GitHub.
9. Once a PR is merged, the associated `Terraform` resource and `GitRepository`
   are deleted by the poller.
10. Existing Terraform resources managed by Git remain untouched.
11. All of the above communication happens via the Kubernetes API.

## Consequences

The list Pull Requests endpoint returns all required fields to detect new and
closed pull requests. It's one request per repository, but listing comments has
to use an API request per pull request. So we have to add a mechanism to avoid
hitting API rate limits.
