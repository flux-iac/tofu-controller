# Terraform Cloud Integration with Branch Planner

Terraform Cloud is a secure and robust platform designed to store the Terraform states 
for your production systems. When working with Infrastructure as Code, 
managing and ensuring the state is both secure and consistent is critical. 

The introduction of TF-Controller’s support for Terraform Cloud has further enhanced 
the capabilities of managing Terraform operations through Kubernetes.

## First-class Support Terraform Cloud

TF-Controller is not just limited to supporting Terraform Cloud, 
but it also extends its capabilities to Terraform Enterprise. 
By utilizing the `spec.cloud` in Terraform CRD, users can seamlessly
integrate their Kubernetes configurations with Terraform workflows 
both with Terraform Cloud and Terraform Enterprise.

To get started, all you need to do is putting your Terraform Cloud token in a Kubernetes Secret
and specify it in the `spec.cliConfigSecretRef` field of the Terraform CR.
Field `spec.cloud` is used to specify the organization and workspace name.

After connecting your Terraform CR with Terraform Cloud,
TF-Controller can now send your Terraform resources to be planned and applied via Terraform Cloud. 
What’s more, states are automatically stored in your Terraform Cloud's workspace. 
To use TF-Controller with Terraform Cloud `spec.approvalPlan` must be set to `auto`. 

Here's a quick look at how the configuration looks:

```yaml
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: branch-planner-tfc
  namespace: flux-system
spec:
  interval: 2m
  approvePlan: auto
  cloud:
    organization: weaveworks
    workspaces:
      name: branch-planner-tfc
  cliConfigSecretRef:
    name: tfc-cli-config
    namespace: flux-system
```

## Enhancing the GitOps Workflow with Branch Planner

The GitOps methodology aims to streamline the infrastructure provisioning and management using Git as the source of truth.
The newly introduced Branch Planner is a component of TF-Controller that aims to take this a notch higher.

Branch Planner allows developers and operations teams to plan Terraform configurations specifically on a separate branch.
With this feature, the `main` branch can be provisioned directly on Terraform Cloud. 
However, if you’re looking to test or review changes, you can simply create a new branch.

The most important feature of Brach Planner is the seamless integration with the PR (Pull Request) user interface, 
which is familiar territory for many developers. When changes are proposed on this new branch, 
Branch Planner runs a plan in the cluster, and displays the results directly as comments under your PR.
This makes it easier to review and understand the potential impact of your changes before they are applied.

Once you're satisfied with the results, merging your branch into the `main` branch triggers the TF-Controller. 
It communicates with Terraform Cloud to run the necessary plans and apply your approved code. 
The state, as always, is securely stored on Terraform Cloud.

**Note:** In its tech preview version, Branch Planner currently only supports GitHub as the Git provider.

## Step-by-step Guide

### Step 1: Create a Terraform Cloud Token

You can use `terraform login` command to obtain a Terraform Cloud token.
Then use the token to create a Kubernetes Secret.
```bash
kubectl create secret generic \
  tfc-cli-config \
  --namespace=flux-system \
  --from-file=terraform.tfrc=/dev/stdin << EOF
credentials "app.terraform.io" {
  token = "xxxxxxxxxxxxxx.atlasv1.zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"           
}
EOF
```
### Step 2: Create a Terraform CR

The following example shows how to create a Terraform CR to automatically plan and apply Terraform configurations on Terraform Cloud.
It reads the Terraform configurations from a Git repository, plan, apply and stores the state in a Terraform Cloud workspace.
The token from Step 1 is specified as the value of `spec.cliConfigSecretRef` and used to authenticate with Terraform Cloud.

```yaml
---
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: branch-planner-demo
  namespace: flux-system
spec:
  interval: 30s
  url: https://github.com/tf-controller/branch-planner-demo
  ref:
    branch: main
---
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: branch-planner-tfc
  namespace: flux-system
spec:
  interval: 2m
  approvePlan: auto
  cloud:
    organization: weaveworks
    workspaces:
      name: branch-planner-tfc
  cliConfigSecretRef:
    name: tfc-cli-config
    namespace: flux-system
  vars:
  - name: subject
    value: "world"
  path: ./
  sourceRef:
    kind: GitRepository
    name: branch-planner-demo
    namespace: flux-system
```

### Step 3: Edit file, Create a Branch, and Open a Pull Request

1. **Navigate to Your Repository:** Open a web browser and visit your GitHub repository. 
For our example, navigate to https://github.com/tf-controller/branch-planner-demo.

2. **Locate the File to Edit:** Browse through the repository's file structure and 
click on the Terraform configuration file you wish to edit.

3. **Edit the File:** Click on the pencil icon (edit) located on the top right of the file content.
Make your desired changes to the Terraform configurations. For instance, you might change the Hello world content in the `main.tf` file.
Once you've made your edits, scroll down to prepare to commit the changes.

4. **Commit the Changes to a New Branch:** Instead of committing directly to the `main` branch, 
choose the option to "Create a new branch" for this commit and start a pull request.
Name the branch something descriptive, for example, `change-hello-world-message`.
Click on the Propose Changes button.

5. **Open a Pull Request (PR):** After proposing your changes, you'll be led to the "Open a pull request" page.
Fill in the details of your PR, explaining the changes you made, their purpose, and any other pertinent information.
Click on the **[Create Pull Request]** button.

6. **Review Terraform Plan in PR Comments:** Once the PR is created,
the Branch Planner will trigger a Terraform plan. After the plan is completed,
the results will be posted as a comment on the PR.
This provides an opportunity for you and your team to review the expected changes before they're applied.

### Step 4: Review, Approve and Merge the Pull Request

1. **Review the Changes**:
    - Navigate to the `Pull Requests` tab in your GitHub repository.
    - Click on the title of your pull request to see the details.
    - Examine the `Files changed` section to see the exact modifications made to the Terraform configurations.
    - Check the comments for the Terraform plan output generated by Branch Planner. Ensure the plan matches your expectations.

2. **Iterate on Changes if Necessary**:
    - If you spot any discrepancies or wish to make further adjustments, click on the file in the `Files changed` section.
    - After making the desired edits, commit the changes to the same branch. This will automatically prompt TF-Controller and Branch Planner to generate a new plan.
    - If, for any reason, the automatic replan doesn't occur or you believe there might be an inconsistency, you can manually trigger a new plan by commenting `!replan` on the PR. Branch Planner will then process the request and display the new plan results.

3. **Approve the Changes**:
    - If you're content with the changes and the associated Terraform plan, move to the `Review changes` button on the PR page.
    - Select the `Approve` option from the dropdown and optionally add any final comments.
    - Click `Submit review` to finalize your approval.

4. **Merge the Pull Request**:
    - With the changes approved, click on the `Merge pull request` button.
    - Choose your desired merge strategy from the options provided, such as "Squash and merge" or "Rebase and merge".
    - Click `Confirm merge`.
    - Following the merge, TF-Controller will take over. It will send the updated Terraform configuration to Terraform Cloud, where the changes will be planned and then applied. The resulting infrastructure state will be securely stored within your Terraform Cloud workspace.

## Conclusion

Combining tools like Terraform Cloud, TF-Controller with Branch Planner, and GitHub offers an innovative way for organization to streamline their infrastructure management. Being able to easily review and understand changes in a familiar platform like GitHub ensures clarity. With the immediate feedback provided by Branch Planner, teams can anticipate and discuss potential changes on different branches, before they're implemented. This combination doesn't just make updates safer and more predictable, but it promotes team-wide involvement. Furthermore, the collaboration between TF-Controller and Terraform Cloud guarantees consistency, minimizing errors, and being GitOps. As we navigate an increasingly complex IaC landscape, such simplified, integrated approaches are key to efficient, secure and error-free operations.
