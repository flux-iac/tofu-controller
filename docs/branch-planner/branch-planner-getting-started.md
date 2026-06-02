# Getting Started With Branch Planner

## Prerequisites

1. Flux is installed on the cluster.
2. An API token for your Git provider (GitHub or GitLab).
   - **GitHub**: For public repositories, it's sufficient to enable `Public Repositories` without
   any additional permissions. For private repositories, you need the following permissions:
     - `Pull requests` with Read-Write access. This is required to check Pull Request
     changes, list comments, and create or update comments.
     - `Metadata` with Read-only access. This is automatically marked as "mandatory"
     because of the permissions listed above.
   - **GitLab**: Create a [Personal Access Token](https://docs.gitlab.com/ee/user/profile/personal_access_tokens.html)
   or [Project Access Token](https://docs.gitlab.com/ee/user/project/settings/project_access_tokens.html)
   with the `api` scope. This is required to list merge requests, read changes, and
   create or update comments on merge requests.
3. General knowledge about Tofu-Controller [(see docs)](https://flux-iac.github.io/tofu-controller/).

## Quick Start (GitHub)

This section describes how to install Branch Planner using a HelmRelease object in the `flux-system` namespace with minimum configuration on a KinD cluster.

1. Create a KinD cluster.

    ```shell
    kind create cluster
    ```

2. Install Flux. Make sure you have the latest version of Flux (v2 GA).

    ```shell
    flux install
    ```

3. Create a secret that contains a GitHub API token. If you do not use the `gh` CLI, copy and paste the token from GitHub's website.

    ```shell
    export GITHUB_TOKEN=$(gh auth token)

    kubectl create secret generic branch-planner-token \
        --namespace=flux-system \
        --from-literal="token=${GITHUB_TOKEN}"
    ```

4. Install Branch Planner from a HelmRelease provided by the Tofu Controller repository. Use Tofu Controller v0.16.0-rc.6 or later.

    ```shell
    kubectl apply -f https://raw.githubusercontent.com/flux-iac/tofu-controller/fa4b3b85d316340d897fda4fed757265ba2cd30e/docs/branch_planner/release.yaml
    ```

5. Create a Terraform object with a Source pointing to a repository. Your repository must contain a Terraform file—for example, `main.tf`. Check out [this demo](https://github.com/flux-iac/branch-planner-demo) for an example.

    ```bash
    export GITHUB_USER=<your user>
    export GITHUB_REPO=<your repo>

    cat <<EOF | kubectl apply -f -
    ---
    apiVersion: source.toolkit.fluxcd.io/v1
    kind: GitRepository
    metadata:
      name: branch-planner-demo
      namespace: flux-system
    spec:
      interval: 30s
      url: https://github.com/${GITHUB_USER}/${GITHUB_REPO}
      ref:
        branch: main
    ---
    apiVersion: infra.contrib.fluxcd.io/v1alpha2
    kind: Terraform
    metadata:
      name: branch-planner-demo
      namespace: flux-system
    spec:
      approvePlan: auto
      path: ./
      interval: 1m
      sourceRef:
        kind: GitRepository
        name: branch-planner-demo
        namespace: flux-system
    EOF
    ```

6. Now you can create a pull request on your GitHub repo. The Branch Planner will create a new Terraform object with the plan-only mode enabled and will generate a new plan for you. It will post the plan as a new comment in the pull request.

## Quick Start (GitLab)

Branch Planner supports GitLab (both gitlab.com and self-hosted instances). The provider is automatically detected from the GitRepository source URL.

1. Follow steps 1 and 2 above to create a cluster and install Flux.

2. Create a secret that contains a GitLab API token.

    ```shell
    kubectl create secret generic branch-planner-token \
        --namespace=flux-system \
        --from-literal="token=${GITLAB_TOKEN}"
    ```

3. Install Branch Planner (same as step 4 above).

4. Create a Terraform object with a Source pointing to your GitLab repository.

    ```bash
    export GITLAB_GROUP=<your group or user>
    export GITLAB_PROJECT=<your project>

    cat <<EOF | kubectl apply -f -
    ---
    apiVersion: source.toolkit.fluxcd.io/v1
    kind: GitRepository
    metadata:
      name: branch-planner-demo
      namespace: flux-system
    spec:
      interval: 30s
      url: https://gitlab.com/${GITLAB_GROUP}/${GITLAB_PROJECT}
      ref:
        branch: main
    ---
    apiVersion: infra.contrib.fluxcd.io/v1alpha2
    kind: Terraform
    metadata:
      name: branch-planner-demo
      namespace: flux-system
    spec:
      approvePlan: auto
      path: ./
      interval: 1m
      sourceRef:
        kind: GitRepository
        name: branch-planner-demo
        namespace: flux-system
    EOF
    ```

5. Now you can create a merge request on your GitLab project. The Branch Planner will create a new Terraform object with the plan-only mode enabled and will generate a new plan for you. It will post the plan as a new comment on your merge request.

### Self-Hosted GitLab

For self-hosted GitLab instances, use the URL of your instance in the GitRepository source. The Branch Planner automatically extracts the hostname from the source URL and configures the API client accordingly.

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: my-infra
  namespace: flux-system
spec:
  interval: 30s
  url: https://gitlab.mycompany.com/infrastructure/terraform-modules
  ref:
    branch: main
```

No additional configuration is required beyond pointing the source URL to your self-hosted instance.

## Configure Branch Planner

Branch Planner uses a ConfigMap as configuration. The ConfigMap is optional but useful for fine-tuning Branch Planner.

### Configuration

By default, Branch Planner will look for the `branch-planner` ConfigMap in the same namespace as where the Tofu Controller is installed.
That ConfigMap allows users to specify which Terraform resources in a cluster the Brach Planner should monitor.

The ConfigMap has two fields:

1. `secretName`, which contains the API token to access your Git provider (GitHub or GitLab).
2. `resources`, which defines a list of resources to watch.

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  namespace: flux-system
  name: branch-planner
data:
  secretName: branch-planner-token
  resources: |-
    - namespace: terraform
    - namespace: flux-system
```

#### Secret

Branch Planner uses the referenced Secret for authentication. The auth type
is detected automatically from the keys present in the Secret.

##### API Token (GitHub PAT or GitLab Token)

The simplest option. Use a GitHub Personal Access Token, a GitLab Project
Access Token, or a GitLab Group Access Token. For production Flux clusters,
prefer Project/Group Access Tokens over personal tokens.

For GitHub:

```bash
kubectl create secret generic branch-planner-token \
    --namespace=flux-system \
    --from-literal="token=${GITHUB_TOKEN}"
```

For GitLab (Personal, Project, or Group Access Token with `api` scope):

```bash
kubectl create secret generic branch-planner-token \
    --namespace=flux-system \
    --from-literal="token=${GITLAB_TOKEN}"
```

##### GitHub App (recommended for GitHub)

GitHub Apps provide scoped, short-lived tokens that are automatically
refreshed. This is the recommended approach for production clusters and
organizations. The App needs the following repository permissions:

- **Pull requests**: Read & Write
- **Metadata**: Read-only (automatically selected)

```bash
kubectl create secret generic branch-planner-token \
    --namespace=flux-system \
    --from-literal="githubAppID=${APP_ID}" \
    --from-literal="githubAppInstallationID=${INSTALLATION_ID}" \
    --from-file="githubAppPrivateKey=${PATH_TO_PEM_FILE}"
```

The installation token is generated and refreshed automatically by the
controller. You do not need to manage token expiry.

##### GitLab OAuth2 (recommended for GitLab)

GitLab OAuth2 provides automatic token refresh, removing the need to
manually rotate tokens before they expire. This is the recommended
approach for production GitLab deployments.

1. Register an [OAuth2 application](https://docs.gitlab.com/ee/integration/oauth_provider.html)
   in your GitLab instance (under User Settings > Applications, or
   Group/Admin settings for broader scope). Set the redirect URI to
   `http://localhost` (it is only used during the initial token grant)
   and select the `api` scope.

2. Perform the initial OAuth2 authorization to obtain a refresh token.
   You can use any OAuth2 tool or `curl`:

    ```bash
    # 1. Visit this URL in your browser and authorize:
    # https://gitlab.com/oauth/authorize?client_id=${CLIENT_ID}&redirect_uri=http://localhost&response_type=code&scope=api

    # 2. Copy the 'code' parameter from the redirect URL, then exchange it:
    curl -s -X POST "https://gitlab.com/oauth/token" \
      -d "client_id=${CLIENT_ID}" \
      -d "client_secret=${CLIENT_SECRET}" \
      -d "code=${AUTH_CODE}" \
      -d "grant_type=authorization_code" \
      -d "redirect_uri=http://localhost"
    ```

3. Create the Secret with the OAuth2 credentials:

    ```bash
    kubectl create secret generic branch-planner-token \
        --namespace=flux-system \
        --from-literal="gitlabOAuthClientID=${CLIENT_ID}" \
        --from-literal="gitlabOAuthClientSecret=${CLIENT_SECRET}" \
        --from-literal="gitlabOAuthRefreshToken=${REFRESH_TOKEN}"
    ```

The access token is obtained and refreshed automatically by the
controller using the refresh token. You do not need to manage token
expiry.

#### Resources

If the `resources` list is empty, nothing will be watched. The resource definition
can be exact or namespace-wide.

With the following configuration file, the Branch Planner will watch all Terraform objects in
the `terraform` namespace, and the `exact-terraform-object` Terraform object in
`default` namespace.

```yaml
data:
  resources:
    - namespace: default
      name: exact-terraform-object
    - namespace: terraform
```

### Default Configuration

If no ConfigMap is found, the Branch Planner will not watch any namespaces for Terraform resources and look for a token in a secret named `branch-planner-token` in the `flux-system` namespace. Supplying a secret with a token is a necessary task, otherwise Branch Planner will not be able to interact with the Git provider API.

### Supported Providers

Branch Planner automatically detects the Git provider from the GitRepository source URL:

| Provider | Example URL | Detected As | Comment Editing |
|----------|-------------|-------------|-----------------|
| GitHub | `https://github.com/org/repo` | GitHub | Full |
| GitHub Enterprise | `https://github.mycompany.com/org/repo` | GitHub | Full |
| GitLab | `https://gitlab.com/group/project` | GitLab | Full |
| Self-hosted GitLab | `https://gitlab.mycompany.com/group/project` | GitLab | Full |
| Bitbucket Cloud | `https://bitbucket.org/team/repo` | Bitbucket | Create only |
| Bitbucket Server | Self-hosted (set via `WithDomain`) | Bitbucket Server | Full |
| Gitea | Self-hosted (set via `WithDomain`) | Gitea | Full |
| Azure DevOps | `https://dev.azure.com/org/project/_git/repo` | Azure | Experimental |

The same `branch-planner-token` Secret is used for all providers. The API token format depends on your provider.

**Note:** Bitbucket Cloud does not support editing existing comments through the API. Each plan update will create a new comment instead of updating the previous one.

**Note:** Bitbucket Server and Gitea are self-hosted only and require the hostname to be
derived from the GitRepository source URL (e.g. `https://gitea.mycompany.com/org/repo`).

**Note:** Azure DevOps support is experimental. The underlying go-scm library has limited
implementation of the comment APIs for Azure DevOps — listing PRs and changes works, but
commenting may return errors. This will improve as go-scm adds full Azure DevOps support.
Azure DevOps URLs include a project component (e.g. `dev.azure.com/org/project/_git/repo`)
which is automatically extracted.

**Note:** Gogs is not supported because the go-scm library does not implement the comment
APIs required by Branch Planner.

### OAuth2 Auto-Refresh (Bitbucket Cloud and Gitea)

Bitbucket Cloud and Gitea support OAuth2 with refresh tokens, which eliminates
manual token rotation. The token is refreshed automatically before it expires.

For **Bitbucket Cloud**, register an [OAuth consumer](https://support.atlassian.com/bitbucket-cloud/docs/use-oauth-on-bitbucket-cloud/)
with the `pullrequest:write` scope and perform the initial authorization flow to
obtain a refresh token.

For **Gitea**, register an [OAuth2 application](https://docs.gitea.com/development/oauth2-provider)
in your Gitea instance settings and perform the initial authorization flow.

Create the Secret with OAuth2 credentials:

```bash
kubectl create secret generic branch-planner-token \
    --namespace=flux-system \
    --from-literal="oauthClientID=${CLIENT_ID}" \
    --from-literal="oauthClientSecret=${CLIENT_SECRET}" \
    --from-literal="oauthRefreshToken=${REFRESH_TOKEN}"
```

The access token is obtained and refreshed automatically. The token endpoint
is determined by the provider (Bitbucket Cloud uses `/site/oauth2/access_token`,
Gitea uses `/login/oauth/access_token`).

### Authentication Summary

| Provider | Static Token | Auto-Refresh |
|----------|-------------|--------------|
| GitHub | `token` (PAT) | `githubAppID` + `githubAppInstallationID` + `githubAppPrivateKey` |
| GitLab | `token` (PAT/Project/Group Token) | `gitlabOAuthClientID` + `gitlabOAuthClientSecret` + `gitlabOAuthRefreshToken` |
| Bitbucket Cloud | `token` (App password) | `oauthClientID` + `oauthClientSecret` + `oauthRefreshToken` |
| Bitbucket Server | `token` (PAT) | Not available (OAuth 1.0a only) |
| Gitea | `token` (PAT) | `oauthClientID` + `oauthClientSecret` + `oauthRefreshToken` |
| Azure DevOps | `token` (PAT) | Not available |

## Enable Branch Planner

To enable branch planner, set the `branchPlanner.enabled` to `true` in the Helm
values files.

```
---
branchPlanner:
  enabled: true
```
