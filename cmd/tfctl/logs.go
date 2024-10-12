package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/util"
	"k8s.io/kubectl/pkg/util/podutils"
)

type logsFlags struct {
	logLevel            string
	follow              bool
	tail                int64
	kind                string
	name                string
	controllerNamespace string
	allNamespaces       bool
	sinceTime           string
	sinceSeconds        time.Duration
	runner              bool
}

var logsArgs = logsFlags{
	tail: -1,
}

const (
	runnerLogsSleep     = 30 * time.Second
	controllerContainer = "manager"
)

func buildLogsCmd() *cobra.Command {
	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Display formatted logs for Terraform controller",
		Long:  "The logs command displays formatted logs from Terraform controller.",
		Example: `  # Print the reconciliation logs of all Terraform custom resources in your cluster
	  tfctl logs --all-namespaces

	  # Print all logs of all Terraform custom resources newer than 2 minutes
	  tfctl logs --all-namespaces --since=2m

	  # Stream logs for a particular log level
	  tfctl logs --follow --level=error --all-namespaces

	  # Filter logs by name and namespace
	  tfctl logs --name=podinfo --namespace=default

	  # Print logs when Flux is installed in a different namespace than flux-system
	  tfctl logs --controller-namespace=my-namespace

	  # Print runner logs
	  tfctl logs --runner --name=podinfo
		`,
		RunE: logsCmdRun,
	}

	logsCmd.Flags().StringVarP(&logsArgs.logLevel, "level", "", logsArgs.logLevel, "log level, available options are: debug, info and error")
	logsCmd.Flags().StringVarP(&logsArgs.name, "name", "", logsArgs.name, "specifies the name of the object logs to be displayed")
	logsCmd.Flags().BoolVarP(&logsArgs.follow, "follow", "f", logsArgs.follow, "specifies if the logs should be streamed")
	logsCmd.Flags().Int64VarP(&logsArgs.tail, "tail", "", logsArgs.tail, "lines of recent log file to display")
	logsCmd.Flags().StringVarP(&logsArgs.controllerNamespace, "controller-namespace", "", defaultNamespace, "the namespace where the Terraform controller is running")
	logsCmd.Flags().BoolVarP(&logsArgs.allNamespaces, "all-namespaces", "A", false, "displays logs for objects across all namespaces")
	logsCmd.Flags().DurationVar(&logsArgs.sinceSeconds, "since", logsArgs.sinceSeconds, "Only return logs newer than a relative duration like 5s, 2m, or 3h. Defaults to all logs. Only one of since-time / since may be used.")
	logsCmd.Flags().StringVar(&logsArgs.sinceTime, "since-time", logsArgs.sinceTime, "Only return logs after a specific date (RFC3339). Defaults to all logs. Only one of since-time / since may be used.")
	logsCmd.Flags().BoolVarP(&logsArgs.runner, "runner", "", false, "displays logs for the Terraform runner")

	return logsCmd
}

func logsCmdRun(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cfg, err := kubeconfigArgs.ToRESTConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}

	if len(args) > 0 {
		return fmt.Errorf("no argument required")
	}

	if logsArgs.runner {
		return runnerLogs(ctx, logsArgs, clientset)
	}

	pods, err := getPods(ctx, clientset, logsArgs.controllerNamespace)
	if err != nil {
		return err
	}

	logOpts := &corev1.PodLogOptions{
		Follow: logsArgs.follow,
	}

	if logsArgs.tail > -1 {
		logOpts.TailLines = &logsArgs.tail
	}

	if len(logsArgs.sinceTime) > 0 && logsArgs.sinceSeconds != 0 {
		return fmt.Errorf("at most one of `sinceTime` or `sinceSeconds` may be specified")
	}

	if len(logsArgs.sinceTime) > 0 {
		t, err := util.ParseRFC3339(logsArgs.sinceTime, metav1.Now)
		if err != nil {
			return fmt.Errorf("%s is not a valid (RFC3339) time", logsArgs.sinceTime)
		}
		logOpts.SinceTime = &t
	}

	if logsArgs.sinceSeconds != 0 {
		// round up to the nearest second
		sec := int64(logsArgs.sinceSeconds.Round(time.Second).Seconds())
		logOpts.SinceSeconds = &sec
	}

	var requests []rest.ResponseWrapper
	for _, pod := range pods {
		logOpts := logOpts.DeepCopy()
		if len(pod.Spec.Containers) > 1 {
			logOpts.Container = controllerContainer
		}
		req := clientset.CoreV1().Pods(logsArgs.controllerNamespace).GetLogs(pod.Name, logOpts)
		requests = append(requests, req)
	}

	if logsArgs.follow && len(requests) > 1 {
		return parallelPodLogs(ctx, requests)
	}

	return podLogs(ctx, requests)
}

// runnerLogs fetch logs from the Terraform runner
func runnerLogs(ctx context.Context, logFlags logsFlags, c *kubernetes.Clientset) error {
	logOpts := &corev1.PodLogOptions{
		Follow: logsArgs.follow,
	}
	podName := fmt.Sprintf("%s-tf-runner", logFlags.name)

LOOP:
	req := c.CoreV1().Pods(logsArgs.controllerNamespace).GetLogs(podName, logOpts)

	stream, err := req.Stream(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			fmt.Printf("%s/%s runner pod is not running, waiting...\n", logsArgs.controllerNamespace, podName)
			time.Sleep(runnerLogsSleep)

			goto LOOP
		}

		return err
	}
	defer stream.Close()

	_, err = io.Copy(os.Stdout, stream)

	return err
}

// getPods searches for all Deployments in the given namespace that match the given label and returns a list of Pods
// from these Deployments. For each Deployment a single Pod is chosen (based on various factors such as the running
// state). If no Pod is found, an error is returned.
func getPods(ctx context.Context, c *kubernetes.Clientset, ns string) ([]corev1.Pod, error) {
	var ret []corev1.Pod

	opts := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", "app.kubernetes.io/name", "tofu-controller"),
	}
	deployList, err := c.AppsV1().Deployments(ns).List(ctx, opts)
	if err != nil {
		return ret, err
	}

	for _, deploy := range deployList.Items {
		label := deploy.Spec.Template.Labels
		opts := metav1.ListOptions{
			LabelSelector: createLabelStringFromMap(label),
		}
		podList, err := c.CoreV1().Pods(ns).List(ctx, opts)
		if err != nil {
			return ret, err
		}
		pods := []*corev1.Pod{}
		for i := range podList.Items {
			pod := podList.Items[i]
			pods = append(pods, &pod)
		}

		if len(pods) > 0 {
			// sort pods to prioritize running pods over others
			sort.Sort(podutils.ByLogging(pods))
			ret = append(ret, *pods[0])
		}
	}

	if len(ret) == 0 {
		return nil, fmt.Errorf("no Flux pods found in namespace %q", ns)
	}

	return ret, nil
}

func parallelPodLogs(ctx context.Context, requests []rest.ResponseWrapper) error {
	reader, writer := io.Pipe()
	errReader, errWriter := io.Pipe()
	wg := &sync.WaitGroup{}
	wg.Add(len(requests))

	for _, request := range requests {
		go func(req rest.ResponseWrapper) {
			defer wg.Done()
			if err := logRequest(ctx, req, writer); err != nil {
				fmt.Fprintf(errWriter, "failed getting logs: %s\n", err)
				return
			}
		}(request)
	}

	go func() {
		wg.Wait()
		writer.Close()
		errWriter.Close()
	}()

	stdoutErrCh := asyncCopy(os.Stdout, reader)
	stderrErrCh := asyncCopy(os.Stderr, errReader)

	return errors.Join(<-stdoutErrCh, <-stderrErrCh)
}

// asyncCopy copies all data from from dst to src asynchronously and returns a channel for reading an error value.
// This is basically an asynchronous wrapper around `io.Copy`. The returned channel is unbuffered and always is sent
// a value (either nil or the error from `io.Copy`) as soon as `io.Copy` returns.
// This function lets you copy from multiple sources into multiple destinations in parallel.
func asyncCopy(dst io.Writer, src io.Reader) <-chan error {
	errCh := make(chan error)
	go func(errCh chan error) {
		_, err := io.Copy(dst, src)
		errCh <- err
	}(errCh)

	return errCh
}

func podLogs(ctx context.Context, requests []rest.ResponseWrapper) error {
	var retErr error
	for _, req := range requests {
		if err := logRequest(ctx, req, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "failed getting logs: %s\n", err)
			retErr = fmt.Errorf("failed to collect logs from all Flux pods")
			continue
		}
	}

	return retErr
}

func createLabelStringFromMap(m map[string]string) string {
	var strArr []string
	for key, val := range m {
		pair := fmt.Sprintf("%v=%v", key, val)
		strArr = append(strArr, pair)
	}

	return strings.Join(strArr, ",")
}

func logRequest(ctx context.Context, request rest.ResponseWrapper, w io.Writer) error {
	stream, err := request.Stream(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()

	scanner := bufio.NewScanner(stream)

	const logTmpl = "{{.Timestamp}} {{.Level}} {{or .Kind .ControllerKind}}{{if .Name}}/{{.Name}}.{{.Namespace}}{{end}} - {{.Message}} {{.Error}}\n"
	t, err := template.New("log").Parse(logTmpl)
	if err != nil {
		return fmt.Errorf("unable to create template, err: %s", err)
	}

	bw := bufio.NewWriter(w)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var l ControllerLogEntry
		if err := json.Unmarshal([]byte(line), &l); err != nil {
			fmt.Printf("failed parsing log line: %s\n", err)
			break
		}
		filterPrintLog(t, &l, bw)
		bw.Flush()
	}

	return nil
}

func filterPrintLog(t *template.Template, l *ControllerLogEntry, w io.Writer) {
	if (logsArgs.logLevel == "" || logsArgs.logLevel == l.Level) &&
		(logsArgs.kind == "" || strings.EqualFold(logsArgs.kind, l.Kind) || strings.EqualFold(logsArgs.kind, l.ControllerKind)) &&
		(logsArgs.name == "" || strings.EqualFold(logsArgs.name, l.Name)) &&
		(logsArgs.allNamespaces || strings.EqualFold(*kubeconfigArgs.Namespace, l.Namespace)) {
		err := t.Execute(w, l)
		if err != nil {
			fmt.Printf("log template error: %s\n", err)
		}
	}
}

type ControllerLogEntry struct {
	Timestamp      string `json:"ts"`
	Level          string `json:"level"`
	Message        string `json:"msg"`
	Error          string `json:"error,omitempty"`
	Logger         string `json:"logger"`
	Kind           string `json:"reconciler kind,omitempty"`
	ControllerKind string `json:"controllerKind,omitempty"`
	Name           string `json:"name,omitempty"`
	Namespace      string `json:"namespace,omitempty"`
}
