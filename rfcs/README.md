# Tofu Controller RFCs

In many situations, enhancements and new features are proposed
on [tofu-controller/discussions](https://github.com/flux-iac/tofu-controller/discussions).
These proposals are evaluated publicly by maintainers, contributors, users, and any other interested parties. After a
consensus is reached among the participants, the proposed changes proceed to the pull request stage where the
implementation specifics are reviewed, and either approved or rejected by the maintainers.

For **substantial** proposals, we necessitate a systematic design process to ensure all stakeholders can be confident
about the direction in which the Tofu Controller project is progressing.

The "RFC" (request for comments) process aims to offer a uniform and controlled pathway for significant changes to be
incorporated into the Tofu Controller project.

Examples of substantial changes include:

- API additions (new types of resources, new relationships between existing APIs)
- API breaking changes (new mandatory fields, field removals)
- Security-related changes (Tofu Controller permissions, tenant isolation and impersonation)
- Drop capabilities (discontinuation of an existing integration with an external service due to security concerns)

## RFC Process

- Prior to submitting an RFC, please discuss the proposal with the Tofu Controller community. Initiate a discussion
  on GitHub and solicit feedback. You must find a sponsor for the RFC who could be a project maintainer, a flux-iac
  Tech Lead, a flux-iac Product Manager, or a flux-iac Principal Engineer.
- Submit an RFC by opening a pull request using [RFC-0000](RFC-0000/README.md) as a template.
- The sponsor will assign the PR to themselves, label the PR with `area/RFC`, and request other maintainers to begin the
  review process.
- Incorporate feedback by adding commits without altering the history.
- The proposal can only be merged after it has been approved by either **two project maintainers**, or **one project
  maintainer and one individual from the following**: a flux-iac Tech Lead, a flux-iac Product Manager, or a
  flux-iac Principal Engineer. The approvers must be satisfied that a [suitable level of consensus](DECISION_MAKING.md) has been achieved.
- Before the merge, an RFC number is assigned by the sponsor and the PR branch must be rebased with the main.
- Once merged, the proposal may be implemented in Tofu Controller. The progress can be tracked using the RFC
  number (used as a prefix for issues and PRs).
- Once the proposal implementation is available in a release candidate or final release, the RFC should be updated with
  the Tofu Controller version added to the "Implementation History" section.
- During the implementation phase, the RFC may be discarded due to security or performance concerns. In this scenario,
  the RFC "Implementation History" should outline the reasons for rejection. The final decision on the feasibility of a
  particular implementation rests with the maintainers who reviewed the code changes.
- A new RFC can be submitted with the aim of replacing an RFC that was rejected during implementation. The new RFC must
  propose a solution to the issues that led to the rejection of the previous RFC.
