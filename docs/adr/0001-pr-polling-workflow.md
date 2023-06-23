# 1. Pull Request Polling

* Status: proposed
* Date: 2023-06-20 
* Authors: @yitsushi
* Deciders: @yitsushi @chanwit @yiannistri

## Context

To detect pull request changes, we can use webhooks or polling using GitHub's
API.

## Decision

We decided to start with polling for security reasons. Using webhooks would require users to
open an ingress to the cluster. Because of this requirement, we think security conscious folks may refuse to roll this out on production clusters especially in an
air-gapped environment. This does not mean that we will never consider using webhooks for this, but that initially, polling is what we have chosen to implement.

The Branch-Based Planner has two components:

1. Polling Server: Detect Pull Request changes and manage Teraform resource
   state.
2. Informer: Make a comment when new plan output is available.

## Consequences

The list Pull Requests endpoint returns all required fields to detect new and
closed pull requests. It's one request per repository, but listing comments has
to use an API request per pull request. So we have to add a mechanism to avoid
hitting API rate limits.
