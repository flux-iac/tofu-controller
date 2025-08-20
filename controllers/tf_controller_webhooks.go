package controllers

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/template"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	"github.com/hashicorp/go-cleanhttp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func shouldProcessPostPlanningWebhooks(terraform infrav1.Terraform) bool {
	if terraform.Spec.Webhooks == nil || len(terraform.Spec.Webhooks) < 1 {
		return false
	}

	for _, webhook := range terraform.Spec.Webhooks {
		if webhook.Stage == infrav1.PostPlanningWebhook {
			return true
		}
	}

	// TODO add better condition here

	return false
}

func (r *TerraformReconciler) prepareWebhookPayload(terraform infrav1.Terraform, runnerClient runner.RunnerClient, payloadType string, tfInstance string) ([]byte, error) {
	toBytes, err := terraform.ToBytes(r.Scheme)
	if err != nil {
		err = fmt.Errorf("failed to marshal Terraform resource: %w", err)
		return nil, err
	}

	reply, err := runnerClient.ShowPlanFile(context.Background(), &runner.ShowPlanFileRequest{
		TfInstance: tfInstance,
		Filename:   runner.TFPlanName,
	})
	if err != nil {
		err = fmt.Errorf("failed to get plan file: %w", err)
		return nil, err
	}

	planInJSON := reply.JsonOutput
	planObj, err := yaml.ConvertJSONToYamlNode(string(planInJSON))
	if err != nil {
		err = fmt.Errorf("failed to convert plan file to YAML: %w", err)
		return nil, err
	}

	obj, err := yaml.ConvertJSONToYamlNode(string(toBytes))
	if err != nil {
		err = fmt.Errorf("failed to convert Terraform resource to YAML: %w", err)
		return nil, err
	}

	if payloadType == "SpecAndPlan" {
		obj, err = obj.Pipe(
			yaml.Tee(yaml.Clear("status")),
			yaml.Tee(
				yaml.LookupCreate(yaml.MappingNode, "status"),
				yaml.SetField("tfplan", planObj),
			),
		)
	} else if payloadType == "SpecOnly" {
		obj, err = obj.Pipe(
			yaml.Tee(yaml.Clear("status")),
		)
	} else if payloadType == "PlanOnly" {
		obj = planObj
	} else {
		return nil, fmt.Errorf("unknown payload type: %s", payloadType)
	}

	if err != nil {
		err = fmt.Errorf("failed to add tfplan to Terraform resource: %w", err)
		return nil, err
	}

	jsonBytes, err := obj.MarshalJSON()
	if err != nil {
		err = fmt.Errorf("failed to marshal Terraform resource with plan: %w", err)
		return nil, err
	}

	return jsonBytes, nil
}

func (r *TerraformReconciler) processPostPlanningWebhooks(ctx context.Context, terraform infrav1.Terraform, runnerClient runner.RunnerClient, revision string, tfInstance string) (infrav1.Terraform, error) {
	log := ctrl.LoggerFrom(ctx)

	hooks := []infrav1.Webhook{}
	for _, webhook := range terraform.Spec.Webhooks {
		if webhook.Stage == infrav1.PostPlanningWebhook {
			hooks = append(hooks, webhook)
		}
	}

	if len(hooks) == 0 {
		return terraform, nil
	}

	disableWebhookTLSVerification := os.Getenv("DISABLE_WEBHOOK_TLS_VERIFY") == "1"

	for _, webhook := range hooks {
		log.Info("processing post-planning webhook", "webhook", webhook.URL)

		// We skip webhook if it's not enabled
		if webhook.IsEnabled() == false {
			continue
		}

		log.Info("webhook is enabled, processing")

		payloadBytes, err := r.prepareWebhookPayload(terraform, runnerClient, webhook.PayloadType, tfInstance)
		if err != nil {
			err = fmt.Errorf("failed to prepare webhook payload: %w", err)
			return terraform, err
		}

		log.Info("webhook payload prepared")

		cli := cleanhttp.DefaultClient()

		if disableWebhookTLSVerification == false {

			log.Info("webhook TLS verification is enabled")

			// parse webhook.URL and get the server name
			u, err := url.Parse(webhook.URL)
			if err != nil {
				err = fmt.Errorf("failed to parse webhook URL: %w", err)
				return terraform, err
			}

			log.Info("webhook URL parsed", "host", u.Host)

			caCertPath := "/etc/certs/" + u.Hostname() + "/ca.crt"
			caCertPool := x509.NewCertPool()
			caCert, err := os.ReadFile(caCertPath)
			if err == nil {
				caCertPool.AppendCertsFromPEM(caCert)
			}

			log.Info("webhook CA cert loaded", "path", caCertPath)

			tlsCertPath := "/etc/certs/" + u.Hostname() + "/tls.crt"
			tlsKeyPath := "/etc/certs/" + u.Hostname() + "/tls.key"
			certificate, err := tls.LoadX509KeyPair(tlsCertPath, tlsKeyPath)
			if err != nil {
				err = fmt.Errorf("failed to load webhook TLS certificate: %w", err)
				return terraform, err
			}

			log.Info("webhook TLS cert loaded", "path", tlsCertPath, "keypath", tlsKeyPath)

			cli.Transport.(*http.Transport).TLSClientConfig = &tls.Config{
				RootCAs:      caCertPool,
				Certificates: []tls.Certificate{certificate},
			}

			log.Info("webhook TLS config set")
		}

		post, err := cli.Post(webhook.URL, "application/json", bytes.NewReader(payloadBytes))
		if err != nil {
			err = fmt.Errorf("failed to send webhook: %w", err)
			return terraform, err
		}

		log.Info("webhook sent")

		if post.StatusCode != 200 {
			return terraform, fmt.Errorf("webhook %s returned %d: %s", webhook.URL, post.StatusCode, post.Status)
		}

		log.Info(fmt.Sprintf("webhook returned %d: %s", post.StatusCode, post.Status))

		// read json from post.Body, unmarshall to map[string]interface{}
		jsonReply := map[string]interface{}{}
		err = json.NewDecoder(post.Body).Decode(&jsonReply)
		if err != nil {
			err = fmt.Errorf("failed to decode webhook reply: %w", err)
			return terraform, err
		}

		log.Info("webhook reply decoded")

		// Test if the reply contains a good result
		testExprTpl, err := template.
			New("testexpr").
			Delims("${{", "}}").
			Parse(webhook.TestExpression)
		if err != nil {
			err = fmt.Errorf("failed to parse webhook test expression: %w", err)
			return terraform, err
		}

		log.Info("webhook test expression parsed")

		var testExprBuf bytes.Buffer
		err = testExprTpl.Execute(&testExprBuf, jsonReply)
		if err != nil {
			err = fmt.Errorf("failed to execute webhook test expression: %w", err)
			return terraform, err
		}

		log.Info("webhook test expression executed")

		testResult := strings.TrimSpace(testExprBuf.String())
		if testResult == "true" || testResult == "yes" {
			log.Info("webhook test expression returned true, webhook is successful")
			continue
		} else if testResult == "false" || testResult == "no" {
			// do nothing
		} else {
			return terraform, fmt.Errorf("webhook test expression %q returned unexpected result: %s", webhook.TestExpression, testResult)
		}

		log.Info("webhook test expression returned false, webhook is not successful - prepare error message")

		// Extract the error message from the webhook response
		errMsgTpl, err := template.
			New("errmsg").
			Delims("${{", "}}").
			Parse(webhook.ErrorMessageTemplate)
		if err != nil {
			err = fmt.Errorf("failed to parse webhook error message template: %w", err)
			return terraform, err
		}

		log.Info("webhook error message template parsed")

		var errorMessage bytes.Buffer
		err = errMsgTpl.Execute(&errorMessage, jsonReply)
		if err != nil {
			err = fmt.Errorf("failed to execute webhook error message template: %w", err)
			return terraform, err
		}

		log.Info("webhook error message template executed")

		terraform = infrav1.TerraformPostPlanningWebhookFailed(terraform, revision, errorMessage.String())
		webhookErr := errors.New(errorMessage.String())
		return terraform, webhookErr
	}

	return terraform, nil
}
