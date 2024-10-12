# Use tofu-controller with External Webhooks

The tofu-controller provides a way to integrate with webhooks to further validate Terraform plans and manage the Terraform execution process. 
With the webhook feature, you can implement custom policy checks, validations, and other logic to determine if the Terraform process should proceed.

## Setting up the Webhook

1. **Webhook URL:** Specify the URL of your webhook, ensuring it points to a valid HTTPS endpoint.
2. **Expected Return:** The webhook should return a valid JSON object. For instance:
```json
{"passed": true}
```
3. **Accepted True Values:** The true values can be `true`, `"true"`, and `"yes"`.
4. **Accepted False Values:** The false values can be `flse`, `"false"`, and `"no"`.

Below is a breakdown of the relevant parts of the configuration:

1. `webhooks:` This is the section where you specify all webhook related configurations.
2. `stage:` Define at which stage the webhook will be triggered. Currenly, we support only the `post-planning` stage.
3. `url:` The URL pointing to your webhook endpoint.
4. `testExpression:` This expression is used to evaluate the response from the webhook. If it evaluates to true, the controller proceeds with the operation. In the example, the expression checks for the passed value from the webhook's JSON response.
5. `errorMessageTemplate:` If testExpression evaluates to false, this template is used to extract the error message from the webhook's JSON response. This message will be displayed to the user.

## Configuration Example

Here's a configuration example on how to use the webhook feature to integrate with Weave Policy Engine.
```yaml
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: helloworld-tf
spec:
  path: ./terraform
  approvePlan: "auto"
  interval: 1m
  storeReadablePlan: human
  sourceRef:
    kind: GitRepository
    name: helloworld-tf
  webhooks:
  - stage: post-planning
    url: https://policy-agent.policy-system.svc/terraform/admission
    testExpression: "${{ .passed }}"
    errorMessageTemplate: "Violation: ${{ (index (index .violations 0).occurrences 0).message }}"
  writeOutputsToSecret:
    name: helloworld-outputs
```

Important Considerations:

- Ensure that your webhook endpoint is secure, as the tofu-controller will be sending potentially sensitive Terraform plan data to it.
- Test your webhook implementation thoroughly before deploying to production, as any issues could interrupt or halt your Terraform process.

With the webhook feature, you can create a more robust and flexible GitOps Terraform pipeline that respects custom organizational policies and other requirements.
