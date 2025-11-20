package controllers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	eventv1 "github.com/fluxcd/pkg/apis/event/v1beta1"
	"github.com/fluxcd/pkg/runtime/logger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *TerraformReconciler) shouldDoHealthChecks(terraform *infrav1.Terraform) bool {
	if terraform.Spec.HealthChecks == nil || len(terraform.Spec.HealthChecks) < 1 {
		return false
	}

	var applyCondition metav1.Condition
	var hcCondition metav1.Condition
	for _, c := range terraform.Status.Conditions {
		if c.Type == infrav1.ConditionTypeApply {
			applyCondition = c
		} else if c.Type == infrav1.ConditionTypeHealthCheck {
			hcCondition = c
		}
	}

	// health checks were previously performed but failed
	// do health check again
	if hcCondition.Reason == infrav1.HealthChecksFailedReason {
		return true
	}

	// terraform was applied and no health check performed yet
	// do health check
	if applyCondition.Reason == infrav1.TFExecApplySucceedReason &&
		hcCondition.Reason == "" {
		return true
	}

	return false
}

func (r *TerraformReconciler) doHealthChecks(ctx context.Context, terraform *infrav1.Terraform, revision string, runnerClient runner.RunnerClient) (*infrav1.Terraform, error) {
	log := ctrl.LoggerFrom(ctx)
	traceLog := log.V(logger.TraceLevel).WithValues("function", "TerraformReconciler.doHealthChecks")
	log.Info("calling doHealthChecks ...")

	// get terraform output data for health check urls
	traceLog.Info("Create a map for outputs")
	outputs := make(map[string]string)
	traceLog.Info("Check for a name for our outputs secret")
	if terraform.Spec.WriteOutputsToSecret != nil && terraform.Spec.WriteOutputsToSecret.Name != "" {
		traceLog.Info("Get outputs from the runner")
		getOutputsReply, err := runnerClient.GetOutputs(ctx, &runner.GetOutputsRequest{
			Namespace:  terraform.Namespace,
			SecretName: terraform.Spec.WriteOutputsToSecret.Name,
		})
		traceLog.Info("Check for an error")
		if err != nil {
			err = fmt.Errorf("error getting terraform output for health checks: %s", err)
			traceLog.Error(err, "Hit an error")
			return infrav1.TerraformHealthCheckFailed(
				terraform,
				err.Error(),
			), err
		}
		traceLog.Info("Set outputs")
		outputs = getOutputsReply.Outputs
	}

	traceLog.Info("Loop over the health checks")
	for _, hc := range terraform.Spec.HealthChecks {
		// perform health check based on type
		traceLog.Info("Check the health check type")
		switch hc.Type {
		case infrav1.HealthCheckTypeTCP:
			traceLog = traceLog.WithValues("health-check-type", infrav1.HealthCheckTypeTCP)
			traceLog.Info("Parse Address and outputs into a template")
			parsed, err := r.parseHealthCheckTemplate(outputs, hc.Address)
			traceLog.Info("Check for an error")
			if err != nil {
				err = fmt.Errorf("error getting terraform output for health checks: %s", err)
				traceLog.Error(err, "Hit an error")
				return infrav1.TerraformHealthCheckFailed(
					terraform,
					err.Error(),
				), err
			}

			traceLog.Info("Run TCP health check and check for an error")
			if err := r.doTCPHealthCheck(ctx, hc.Name, parsed, hc.GetTimeout()); err != nil {
				traceLog.Error(err, "Hit an error")
				msg := fmt.Sprintf("TCP health check error: %s, url: %s", hc.Name, hc.Address)
				traceLog.Info("Record an event")
				r.event(ctx, terraform, revision, eventv1.EventSeverityError, msg, nil)
				traceLog.Info("Return failed health check")
				return infrav1.TerraformHealthCheckFailed(
					terraform,
					err.Error(),
				), err
			}
		case infrav1.HealthCheckTypeHttpGet:
			traceLog = traceLog.WithValues("health-check-type", infrav1.HealthCheckTypeHttpGet)
			traceLog.Info("Parse Address and outputs into a template")
			parsed, err := r.parseHealthCheckTemplate(outputs, hc.URL)
			traceLog.Info("Check for an error")
			if err != nil {
				err = fmt.Errorf("error getting terraform output for health checks: %s", err)
				traceLog.Error(err, "Hit an error")
				return infrav1.TerraformHealthCheckFailed(
					terraform,
					err.Error(),
				), err
			}

			traceLog.Info("Run HTTP health check and check for an error")
			if err := r.doHTTPHealthCheck(ctx, hc.Name, parsed, hc.GetTimeout()); err != nil {
				traceLog.Error(err, "Hit an error")
				msg := fmt.Sprintf("HTTP health check error: %s, url: %s", hc.Name, hc.URL)
				traceLog.Info("Record an event")
				r.event(ctx, terraform, revision, eventv1.EventSeverityError, msg, nil)
				traceLog.Info("Return failed health check")
				return infrav1.TerraformHealthCheckFailed(
					terraform,
					err.Error(),
				), err
			}
		}
	}

	traceLog.Info("Health Check successful")
	terraform = infrav1.TerraformHealthCheckSucceeded(terraform, "Health checks succeeded")
	return terraform, nil
}

// isValidLabel checks if a given label is valid as per RFC 952, updated by RFC 1152.
func isValidLabel(label string) bool {
	if len(label) == 0 || len(label) > 63 {
		return false
	}
	if label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for _, ch := range label {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '-' {
			return false
		}
	}
	return true
}

// validateTCPAddress validates the provided address as a valid TCP address format.
func validateTCPAddress(address string) error {
	if strings.Contains(address, "://") {
		return errors.New("URL schemas are not allowed")
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}

	// Check if host is a valid IP address or conforms to RFC 952, updated by RFC 1152.
	if net.ParseIP(host) == nil {
		labels := strings.Split(host, ".")
		for _, label := range labels {
			if !isValidLabel(label) {
				return errors.New("invalid host format")
			}
		}
	}

	// Check if the port is numeric and within the valid range.
	portInt, err := strconv.Atoi(port)
	if err != nil || portInt <= 0 || portInt > 65535 {
		return errors.New("invalid port number")
	}

	return nil
}

func (r *TerraformReconciler) doTCPHealthCheck(ctx context.Context, name string, address string, timeout time.Duration) error {
	log := ctrl.LoggerFrom(ctx)

	// validate tcp address
	if err := validateTCPAddress(address); err != nil {
		return fmt.Errorf("invalid address for tcp health check: %s, %s", address, err)
	}

	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return fmt.Errorf("failed to perform tcp health check for %s on %s: %s", name, address, err.Error())
	}

	err = conn.Close()
	if err != nil {
		log.Error(err, "Unexpected error closing TCP health check socket")
	}

	return nil
}

func (r *TerraformReconciler) doHTTPHealthCheck(ctx context.Context, name string, urlString string, timeout time.Duration) error {
	log := ctrl.LoggerFrom(ctx)

	// validate url
	_, err := url.ParseRequestURI(urlString)
	if err != nil {
		return fmt.Errorf("invalid url for http health check: %s, %s", urlString, err)
	}

	req, err := http.NewRequest("GET", urlString, nil)
	if err != nil {
		log.Error(err, "Unexpected error creating HTTP request")
		return err
	}

	ctxt, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()

	re, err := http.DefaultClient.Do(req.WithContext(ctxt))
	if err != nil {
		return fmt.Errorf("failed to perform http health check for %s on %s: %s", name, urlString, err.Error())
	}
	defer func() {
		if rerr := re.Body.Close(); rerr != nil {
			log.Error(err, "Unexpected error closing HTTP health check socket")
		}
	}()

	// read http body
	b, err := io.ReadAll(re.Body)
	if err != nil {
		return fmt.Errorf("failed to perform http health check for %s on %s, error reading body: %s", name, urlString, err.Error())
	}

	// check http status code
	if re.StatusCode >= http.StatusOK && re.StatusCode < http.StatusBadRequest {
		log.Info("HTTP health check succeeded for %s on %s, response: %v", name, urlString, *re)
		return nil
	}

	err = fmt.Errorf("failed to perform http health check for %s on %s, response body: %v", name, urlString, string(b))
	log.Error(err, "failed to perform http health check for %s on %s, response body: %v", name, urlString, string(b))
	return err
}

// parse template string from map[string]string
func (r *TerraformReconciler) parseHealthCheckTemplate(content map[string]string, text string) (string, error) {
	var b bytes.Buffer
	tmpl, err := template.
		New("tmpl").
		Delims("${{", "}}").
		Parse(text)
	if err != nil {
		return "", err
	}
	err = tmpl.Execute(&b, content)
	if err != nil {
		err = fmt.Errorf("error getting terraform output for health checks: %s", err)
		return "", err
	}
	return b.String(), nil
}
