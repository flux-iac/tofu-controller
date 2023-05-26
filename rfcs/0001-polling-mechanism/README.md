# RFC-0001 Polling mechanism for the branch-based planner

**Status:** provisional

<!--
Status represents the current state of the RFC.
Must be one of `provisional`, `implementable`, `implemented`, `deferred`, `rejected`, `withdrawn`, or `replaced`.
-->

**Creation date:** 2023-05-17

**Last update:** 2023-05-17

## Summary

This RFC proposes a polling mechanism as a secure and portable solution for tracking changes in the
Branch-based Planner with GitHub. The proposed system reduces security risks associated with webhooks, which require
exposing a publicly accessible endpoint, by having the Kubernetes cluster periodically query GitHub for changes. The
polling method also offers portability across other Git providers like GitLab and Bitbucket, as it avoids the complexity
of dealing with varying webhook systems. The RFC also takes into account the potential drawbacks of the polling
mechanism, such as latency and load on the GitHub server, and suggests careful configuration of the polling frequency.
It further discusses considerations related to GitHub's API rate limit and recommends storing the GitHub token in a
Kubernetes Secret for secure and efficient rate limit management.

## Motivation

In order to effectively track changes in a branch-based planner, it is essential to establish a solid and secure method
of interaction between GitHub and the Kubernetes clusters. Two possible approaches are Webhooks and a polling mechanism.

Webhooks are a common choice because of their real-time nature. However, they do introduce a considerable security risk.
To use webhooks, a publicly accessible endpoint is required, which exposes the Kubernetes cluster to the outside world.
While security measures can be put in place to protect this endpoint (such as using secure tunneling or authentication
mechanisms), the exposure itself presents a risk. An attacker could potentially exploit vulnerabilities in the webhook
receiver or use the exposed endpoint for DDoS attacks.

A polling mechanism, on the other hand, does not require exposing an endpoint. Instead, the Kubernetes cluster reaches
out to GitHub at regular intervals to check for any changes, such as PR creation, PR description changes, or PR comment
changes. This method reduces the attack surface and is generally considered more secure.

### Goals

1. Implement a secure mechanism to track changes in a branch-based planner with GitHub, minimizing the exposure of
   Kubernetes clusters to potential security risks.
2. Develop a portable solution that can be adapted for different Git providers (e.g., GitLab, Bitbucket), reducing the
   complexity of dealing with varying webhook systems or APIs.
3. Manage the potential drawbacks of the polling mechanism, such as latency and load on the GitHub server, through
   careful configuration of the polling frequency.
4. Efficiently handle GitHub API rate limits to prevent the client from being temporarily blocked from making additional
   requests.
5. Securely store and manage GitHub tokens within Kubernetes Secrets to mitigate security risks associated with access
   tokens.

### Non-Goals

1. Creating a real-time update system. While desirable, real-time updates may compromise security and increase system
   complexity. The proposed polling mechanism will inherently introduce some latency.
2. Building custom parsing and handling code for each Git provider's webhook messages. Our aim is to standardize
   interactions with Git providers, rather than dealing with specificities of each provider's webhook system.
3. Eliminating all load on the GitHub server. Some load is inevitable due to the nature of polling; our goal is to
   minimize and manage this load rather than eliminate it completely.

## Portability of Polling Mechanism to Other Git Providers

One of the key advantages of using a polling mechanism is the ease of porting this implementation to other Git providers
such as GitLab, Bitbucket, etc. With the polling mechanism, we can standardize the way we interact with the Git
providers, reducing the complexity that comes with dealing with different webhook systems or APIs.

Webhook support varies widely among Git providers. Some providers might not support webhooks at all, or they might have
different ways of setting up and securing webhooks. Moreover, the structure and content of webhook messages can also
differ significantly between providers, which means the code to parse and handle these messages would need to be written
and maintained for each provider.

On the contrary, the polling mechanism works largely the same way for any Git provider. We need to make requests to the
provider's API to fetch the required data, which is typically available through similar RESTful APIs across different
providers. Therefore, we can easily adapt our polling code to a new provider by changing the API endpoints and adjusting
for any differences in the API responses.

In addition, the polling mechanism allows us to control the rate of requests, which can be beneficial for dealing with
rate limits imposed by different providers. With webhooks, the rate of incoming messages is determined by the activity
on the Git provider, which could potentially overwhelm our system or cause us to exceed rate limits.

## Proposed solution

We propose to implement a polling mechanism that operates from within the Kubernetes cluster to GitHub. This solution
negates the need to expose the Kubernetes cluster to external entities, hence reducing security risk.

However, it's important to note that the polling mechanism isn't without its drawbacks. It introduces latency due to the
time delay between the occurrence of an event and the next scheduled polling. Additionally, it can potentially increase
the load on the GitHub server if the polling frequency is high. But, these concerns can be mitigated by carefully
configuring the polling frequency based on the urgency of updates.

## Examples

### Example 1: Polling for PR creation

```go
package main

import (
	"fmt"
	"context"
	"time"

	"github.com/google/go-github/v52/github"
	"golang.org/x/oauth2"
)

type PRState struct {
	Number    int
	Title     string
	FileNames []string
}

var previousPRState = make(map[int]PRState)

func main() {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: "..."}, // Replace with your GitHub token.
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	for {
		// Replace owner and repo.
		prs, _, _ := client.PullRequests.List(ctx, "weaveworks", "tf-controller", nil)

		for _, pr := range prs {
			// Only track PRs to the main branch
			if *pr.Base.Ref != "main" {
				continue
			}

			files, _, _ := client.PullRequests.ListFiles(ctx, "weaveworks", "tf-controller", *pr.Number, nil)
			var fileNames []string
			for _, file := range files {
				fileNames = append(fileNames, *file.Filename)
			}

			currentPRState := PRState{
				Number:    *pr.Number,
				Title:     *pr.Title,
				FileNames: fileNames,
			}

			if _, ok := previousPRState[currentPRState.Number]; !ok {
				// Handle new PR, examine branch name, file paths, etc.
				fmt.Println(currentPRState)
			}

			// Update the stored PR state.
			previousPRState[currentPRState.Number] = currentPRState
		}

		time.Sleep(10 * time.Minute) // Poll every 10 minutes.
	}

}
```

### Example 2: Polling for PR comment changes

```go
package main

import (
	"context"
	"time"

	"github.com/google/go-github/v52/github"
	"golang.org/x/oauth2"
)

type PRCommentState struct {
	ID     int64
	Author string
	Body   string
}

var previousPRCommentState = make(map[int64]PRCommentState)

func main() {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: "..."}, // Replace with your GitHub token.
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	for {
		// Replace owner, repo, and author.
		prs, _, _ := client.PullRequests.List(ctx, "weaveworks", "tf-controller", nil)
		targetAuthor := "chanwit" // replace with target username

		for _, pr := range prs {
			comments, _, _ := client.Issues.ListComments(ctx, "weaveworks", "tf-controller", *pr.Number, nil)

			for _, comment := range comments {
				if *comment.User.Login != targetAuthor {
					continue
				}

				currentPRCommentState := PRCommentState{
					ID:     *comment.ID,
					Author: *comment.User.Login,
					Body:   *comment.Body,
				}

				if prevState, ok := previousPRCommentState[currentPRCommentState.ID]; ok {
					// Check if there's a change.
					if prevState.Body != currentPRCommentState.Body {
						// Handle the comment change.
						// ...
					}
				} else {
					// Handle the new comment.
					// ...
				}

				// Update the stored comment state.
				previousPRCommentState[currentPRCommentState.ID] = currentPRCommentState
			}
		}

		time.Sleep(10 * time.Minute) // Poll every 10 minutes.
	}
}
```

## Rate Limit Consideration

The GitHub API enforces a rate limit to control the number of requests a client can make in a given period of time to
ensure fair usage. For authenticated requests, you can make up to 5,000 requests per hour.

In the context of our polling mechanism, this means we need to plan the frequency of our polling requests carefully.
Making requests too frequently could quickly exhaust the limit and result in the client being temporarily blocked from
making additional requests.

For example, if we poll GitHub every minute, we could make up to 60 requests per hour for each unique item we are
polling. If we are monitoring 100 pull requests, this will total 6,000 requests per hour, exceeding the rate limit.
Therefore, we might need to increase our polling interval or reduce the number of items we are monitoring.

Also, it's a good practice to handle the X-RateLimit-Remaining HTTP header in the API response, which indicates the
number of requests that you can make before hitting the limit. If the remaining limit is low, we can decide to pause or
slow down requests until the limit is reset.

To effectively manage GitHub API rate limits and maintain the security of access tokens, we recommend storing the GitHub
token in a Kubernetes Secret and referencing that Secret in the Terraform Custom Resource (CR).

## Handling Pod Restarts and In-Memory State Management

From examples, the state of PRs and PR comments is integral to the efficiency of the polling mechanism, enabling it to track changes effectively. In the proposed implementation, these states are maintained in memory during the lifetime of a polling process.

One might raise concerns about the transient nature of in-memory data, particularly in cases where a pod restarts. It's crucial to note, however, that this is a deliberate design decision based on the nature of the data and the operation of the system itself.

In the event of a pod restart, while the in-memory state data would indeed be lost, the system is designed to be stateless and idempotent. This means that it can regenerate the necessary state data by querying the GitHub API again upon restart. The state information is primarily used to determine changes between consecutive polls. Therefore, even if a pod restarts, once it's up again, it can retrieve the necessary state information from GitHub (as the single source of truth), compare it with the subsequent poll, and continue tracking changes effectively.

In essence, the temporary nature of in-memory state storage doesn't pose a risk to the functionality or reliability of the system. Instead, it simplifies the system design and reduces dependencies on external storage systems, while ensuring the effective tracking of changes in the GitHub repository.

## Implementation History

<!--
Major milestones in the lifecycle of the RFC such as:
- The first Terraform Controller release where an initial version of the RFC was available.
- The version of Terraform Controller where the RFC graduated to general availability.
- The version of Terraform Controller where the RFC was retired or superseded.
-->
