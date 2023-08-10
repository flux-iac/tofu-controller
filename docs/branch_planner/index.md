# Branch Planner User Guide

## Overview

The Branch Planner, a new component of the Terraform Controller, is specifically designed to enhance the flexibility and robustness of Terraform Controller planning operations. This feature, currently in its technology preview phase, facilitates Terraform planning across branches, creating a streamlined and familiar PR-based workflow for users.

### How does it work?

When the Branch Planner starts, it polls repositories that contain Terraform resources at regular intervals, in order to detect Pull Requests (PR) that change those resources. Upon detecting that a PR exists, the Branch Planner initialises a Terraform object in Plan Only mode for the corresponding branch. In this mode, Terraform Controller generates Terraform plans but does not apply them. Once the plan is generated, Branch Planner posts the plan under the PR as a comment enabling users to review the plan. When the Terraform files of the corresponding branch get updated, Branch Planner posts the updated plan under the PR as new comment, keeping the PR up-to-date with the latest Terraform plan.

### Replan commands

The Branch Planner also allows users to manually trigger the replan process. By simply commenting `!replan` under the PR, the Branch Planner will be instructed to generate a new plan and post it under the PR as a new comment.

Now that you know what Branch Planner can do for you, follow the [guide to get started](./getting-started.md).