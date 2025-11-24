package controllers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fluxcd/pkg/runtime/logger"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	controllerruntime "sigs.k8s.io/controller-runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/mtls"
	"github.com/flux-iac/tofu-controller/runner"
)

func getRunnerPodObjectKey(terraform infrav1.Terraform) types.NamespacedName {
	return types.NamespacedName{Namespace: terraform.Namespace, Name: fmt.Sprintf("%s-tf-runner", terraform.Name)}
}

func getRunnerPodImage(image string) string {
	runnerPodImage := image
	if runnerPodImage == "" {
		runnerPodImage = os.Getenv("RUNNER_POD_IMAGE")
	}
	if runnerPodImage == "" {
		runnerPodImage = "ghcr.io/flux-iac/tf-runner:latest"
	}
	return runnerPodImage
}

func getRunnerImagePullPolicy(imagePullPolicy string) v1.PullPolicy {
	runnerImagePullPolicy := v1.PullPolicy(imagePullPolicy)
	if runnerImagePullPolicy == "" {
		runnerImagePullPolicy = v1.PullIfNotPresent
	}
	return runnerImagePullPolicy
}

func runnerPodTemplate(terraform infrav1.Terraform, secretName string, revision string) (v1.Pod, error) {
	podNamespace := terraform.Namespace
	podName := fmt.Sprintf("%s-tf-runner", terraform.Name)
	podInstance, err := runnerPodInstance(revision)
	if err != nil {
		return v1.Pod{}, err
	}

	runnerPodTemplate := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: podNamespace,
			Name:      podName,
			Labels: map[string]string{
				"app.kubernetes.io/created-by":   "tf-controller",
				"app.kubernetes.io/name":         "tf-runner",
				"app.kubernetes.io/instance":     podInstance,
				infrav1.RunnerLabel:              terraform.Namespace,
				"tf.weave.works/tls-secret-name": secretName,
			},
			Annotations: terraform.Spec.RunnerPodTemplate.Metadata.Annotations,
		},
	}

	// add runner pod custom labels
	if len(terraform.Spec.RunnerPodTemplate.Metadata.Labels) != 0 {
		for k, v := range terraform.Spec.RunnerPodTemplate.Metadata.Labels {
			runnerPodTemplate.Labels[k] = v
		}
	}
	return runnerPodTemplate, nil
}

func (r *TerraformReconciler) LookupOrCreateRunner(ctx context.Context, terraform infrav1.Terraform, revision string) (runner.RunnerClient, func() error, error) {
	log := ctrl.LoggerFrom(ctx)
	traceLog := log.V(logger.TraceLevel).WithValues("function", "TerraformReconciler.lookupOrCreateRunner_000")
	// we have to make sure that the secret is valid before we can create the runner.
	traceLog.Info("Validate the secret used for the Terraform resource")
	secret, err := r.reconcileRunnerSecret(ctx, &terraform)
	traceLog.Info("Check for an error")
	if err != nil {
		traceLog.Error(err, "Hit an error")
		return nil, nil, err
	}

	var hostname string
	traceLog.Info("Check if we're running a local Runner")
	if os.Getenv("INSECURE_LOCAL_RUNNER") == "1" {
		traceLog.Info("Local Runner, set hostname")
		hostname = "localhost"
	} else {
		traceLog.Info("Get Runner pod IP")
		podIP, err := r.reconcileRunnerPod(ctx, terraform, secret, revision)
		traceLog.Info("Check for an error")
		if err != nil {
			traceLog.Error(err, "Hit an error")
			return nil, nil, err
		}
		traceLog.Info("Get pod coordinates", "pod-ip", podIP, "pod-hostname", terraform.Name)
		if r.UsePodSubdomainResolution {
			hostname = terraform.GetRunnerHostname(terraform.Name, r.ClusterDomain)
		} else {
			hostname = terraform.GetRunnerHostname(podIP, r.ClusterDomain)
		}
	}

	traceLog.Info("Pod hostname set", "hostname", hostname)

	traceLog.Info("Create a new context for the runner connection")
	dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
	traceLog.Info("Defer dialCancel")
	defer dialCancel()
	traceLog.Info("Get the Runner connection")
	conn, err := r.getRunnerConnection(dialCtx, secret, hostname, r.RunnerGRPCPort)
	traceLog.Info("Check for an error")
	if err != nil {
		traceLog.Error(err, "Hit an error")
		return nil, nil, err
	}
	traceLog.Info("Create a close connection function")
	connClose := func() error { return conn.Close() }
	traceLog.Info("Create a new Runner client")
	runnerClient := runner.NewRunnerClient(conn)
	traceLog.Info("Return the client and close connection function")
	return runnerClient, connClose, nil
}

func (r *TerraformReconciler) getRunnerConnection(ctx context.Context, tlsSecret *v1.Secret, hostname string, port int) (*grpc.ClientConn, error) {
	log := ctrl.LoggerFrom(ctx)
	traceLog := log.V(logger.TraceLevel).WithValues("function", "TerraformReconciler.getRunnerConnection")
	addr := fmt.Sprintf("%s:%d", hostname, port)
	traceLog.Info("Set address for target", "addr", addr)
	traceLog.Info("Get GRPC Credentials")
	credentials, err := mtls.GetGRPCClientCredentials(tlsSecret)
	traceLog.Info("Check for an error")
	if err != nil {
		traceLog.Error(err, "Hit an error")
		return nil, err
	}

	const retryPolicy = `{
"methodConfig": [{
  "name": [{"service": "runner.Runner"}],
  "waitForReady": true,
  "retryPolicy": {
    "MaxAttempts": 4,
    "InitialBackoff": ".01s",
    "MaxBackoff": ".01s",
    "BackoffMultiplier": 1.0,
    "RetryableStatusCodes": [ "UNAVAILABLE" ]
  }
}]}`

	traceLog.Info("Return dial context")
	return grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(credentials),
		grpc.WithBlock(),
		grpc.WithDefaultServiceConfig(retryPolicy),
	)
}

func (r *TerraformReconciler) runnerPodSpec(terraform infrav1.Terraform, tlsSecretName string) v1.PodSpec {
	serviceAccountName := terraform.Spec.ServiceAccountName
	if serviceAccountName == "" {
		serviceAccountName = "tf-runner"
	}

	gracefulTermPeriod := terraform.Spec.RunnerTerminationGracePeriodSeconds
	envvars := []v1.EnvVar{}
	envvarsMap := map[string]v1.EnvVar{
		"POD_NAME": {
			Name: "POD_NAME",
			ValueFrom: &v1.EnvVarSource{
				FieldRef: &v1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		"POD_NAMESPACE": {
			Name: "POD_NAMESPACE",
			ValueFrom: &v1.EnvVarSource{
				FieldRef: &v1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
	}

	for _, envName := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"} {
		if envValue := os.Getenv(envName); envValue != "" {
			envvarsMap[envName] = v1.EnvVar{
				Name:  envName,
				Value: envValue,
			}
		}
	}

	for _, env := range terraform.Spec.RunnerPodTemplate.Spec.Env {
		envvarsMap[env.Name] = env
	}

	for _, env := range envvarsMap {
		envvars = append(envvars, env)
	}

	vFalse := false
	vTrue := true
	vUser := int64(65532)

	podVolumes := []v1.Volume{
		{
			Name: "temp",
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "home",
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		},
	}
	if len(terraform.Spec.RunnerPodTemplate.Spec.Volumes) != 0 {
		podVolumes = append(podVolumes, terraform.Spec.RunnerPodTemplate.Spec.Volumes...)
	}
	podVolumeMounts := []v1.VolumeMount{
		{
			Name:      "temp",
			MountPath: "/tmp",
		},
		{
			Name:      "home",
			MountPath: "/home/runner",
		},
	}
	if len(terraform.Spec.RunnerPodTemplate.Spec.VolumeMounts) != 0 {
		podVolumeMounts = append(podVolumeMounts, terraform.Spec.RunnerPodTemplate.Spec.VolumeMounts...)
	}

	securityContext := &v1.SecurityContext{
		Capabilities: &v1.Capabilities{
			Drop: []v1.Capability{"ALL"},
		},
		AllowPrivilegeEscalation: &vFalse,
		RunAsNonRoot:             &vTrue,
		RunAsUser:                &vUser,
		SeccompProfile: &v1.SeccompProfile{
			Type: v1.SeccompProfileTypeRuntimeDefault,
		},
		ReadOnlyRootFilesystem: &vTrue,
	}

	if terraform.Spec.RunnerPodTemplate.Spec.SecurityContext != nil {
		securityContext = terraform.Spec.RunnerPodTemplate.Spec.SecurityContext
	}

	resources := v1.ResourceRequirements{}
	if terraform.Spec.RunnerPodTemplate.Spec.Resources != nil {
		resources = *terraform.Spec.RunnerPodTemplate.Spec.Resources
	}

	podSpec := v1.PodSpec{
		TerminationGracePeriodSeconds: gracefulTermPeriod,
		InitContainers:                terraform.Spec.RunnerPodTemplate.Spec.InitContainers,
		Containers: []v1.Container{
			{
				Name: "tf-runner",
				Args: []string{
					"--grpc-port", fmt.Sprintf("%d", r.RunnerGRPCPort),
					"--tls-secret-name", tlsSecretName,
					"--grpc-max-message-size", fmt.Sprintf("%d", r.RunnerGRPCMaxMessageSize),
				},
				Image:           getRunnerPodImage(terraform.Spec.RunnerPodTemplate.Spec.Image),
				ImagePullPolicy: getRunnerImagePullPolicy(terraform.Spec.RunnerPodTemplate.Spec.ImagePullPolicy),
				Ports: []v1.ContainerPort{
					{
						Name:          "grpc",
						ContainerPort: int32(r.RunnerGRPCPort),
					},
				},
				Env:             envvars,
				EnvFrom:         terraform.Spec.RunnerPodTemplate.Spec.EnvFrom,
				SecurityContext: securityContext,
				VolumeMounts:    podVolumeMounts,
				Resources:       resources,
			},
		},
		Volumes:            podVolumes,
		ServiceAccountName: serviceAccountName,
		NodeSelector:       terraform.Spec.RunnerPodTemplate.Spec.NodeSelector,
		Affinity:           terraform.Spec.RunnerPodTemplate.Spec.Affinity,
		Tolerations:        terraform.Spec.RunnerPodTemplate.Spec.Tolerations,
		HostAliases:        terraform.Spec.RunnerPodTemplate.Spec.HostAliases,
		PriorityClassName:  terraform.Spec.RunnerPodTemplate.Spec.PriorityClassName,
	}

	if r.UsePodSubdomainResolution {
		podSpec.Hostname = terraform.Name
		podSpec.Subdomain = "tf-runner"
	}

	return podSpec
}

func (r *TerraformReconciler) reconcileRunnerPod(ctx context.Context, terraform infrav1.Terraform, tlsSecret *v1.Secret, revision string) (string, error) {
	log := controllerruntime.LoggerFrom(ctx)
	traceLog := log.V(logger.TraceLevel).WithValues("function", "TerraformReconciler.reconcileRunnerPod")
	traceLog.Info("Begin reconcile of the runner pod")
	type state string
	const (
		stateUnknown       state = "unknown"
		stateRunning       state = "running"
		stateNotFound      state = "not-found"
		stateMustBeDeleted state = "must-be-deleted"
		stateTerminating   state = "terminating"
	)

	const interval = time.Second * 15
	traceLog.Info("Set interval", "interval", interval)
	timeout := r.RunnerCreationTimeout // default is 120 seconds
	traceLog.Info("Set timeout", "timeout", timeout)
	tlsSecretName := tlsSecret.Name
	traceLog.Info("Set tlsSecretName", "tlsSecretName", tlsSecretName)

	traceLog.Info("Setup create new pod function")
	createNewPod := func() error {
		runnerPodTemplate, err := runnerPodTemplate(terraform, tlsSecretName, revision)
		if err != nil {
			return err
		}

		newRunnerPod := *runnerPodTemplate.DeepCopy()
		newRunnerPod.Spec = r.runnerPodSpec(terraform, tlsSecretName)
		if err := r.Create(ctx, &newRunnerPod); err != nil {
			return err
		}
		return nil
	}

	traceLog.Info("Setup wait for pod to be terminated function")
	waitForPodToBeTerminated := func() error {
		runnerPodTemplate, err := runnerPodTemplate(terraform, tlsSecretName, revision)
		if err != nil {
			return err
		}

		runnerPod := *runnerPodTemplate.DeepCopy()
		runnerPodKey := client.ObjectKeyFromObject(&runnerPod)
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			err := r.Get(ctx, runnerPodKey, &runnerPod)
			if err != nil && errors.IsNotFound(err) {
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			return err
		}
		return nil
	}

	podState := stateUnknown
	traceLog.Info("Set pod state", "pod-state", podState)

	runnerPodTemplate, err := runnerPodTemplate(terraform, tlsSecretName, revision)
	if err != nil {
		return "", err
	}

	runnerPod := *runnerPodTemplate.DeepCopy()
	runnerPodKey := client.ObjectKeyFromObject(&runnerPod)
	traceLog.Info("Get pod state")
	err = r.Get(ctx, runnerPodKey, &runnerPod)
	traceLog.Info("Check for an error")

	gracefulTermPeriod := *terraform.Spec.RunnerTerminationGracePeriodSeconds
	if err != nil && errors.IsNotFound(err) {
		podState = stateNotFound
	} else if err != nil {
		traceLog.Error(err, "Error getting the Runner Pod", "runner-pod-key", runnerPodKey)
	} else if err == nil {
		label, found := runnerPod.Labels["tf.weave.works/tls-secret-name"]
		traceLog.Info("Set label and found", "label", label, "found", found)
		if !found {
			// this is the pod created by something else but with the same name
			podState = stateMustBeDeleted
			gracefulTermPeriod = int64(1) // force kill = 1 second
		} else if label != tlsSecretName {
			// this is the old pod, created by the previous instance of the controller
			podState = stateMustBeDeleted
			gracefulTermPeriod = *terraform.Spec.RunnerTerminationGracePeriodSeconds // honor the value from the spec
		} else if runnerPod.DeletionTimestamp != nil {
			podState = stateTerminating
		} else if runnerPod.Status.Phase == v1.PodRunning {
			podState = stateRunning
		}
	}

	traceLog.Info("Updated Pod State", "pod-state", podState)
	log.Info("show runner pod state: ", "name", terraform.Name, "state", podState)
	traceLog.Info("Switch on Pod State")

	switch podState {
	case stateNotFound:
		// create new pod
		traceLog.Info("Create a new pod")
		err := createNewPod()
		traceLog.Info("Check for an error")
		if err != nil {
			traceLog.Error(err, "Hit an error")
			return "", err
		}
	case stateMustBeDeleted:
		// delete old pod
		traceLog.Info("Pod must be deleted, attempt deletion")
		if err := r.Delete(ctx, &runnerPod,
			client.GracePeriodSeconds(gracefulTermPeriod),
			client.PropagationPolicy(metav1.DeletePropagationForeground),
		); err != nil {
			traceLog.Error(err, "Hit an error")
			return "", err
		}
		// wait for pod to be terminated
		traceLog.Info("Wait for pod to be terminated and check for an error")
		if err := waitForPodToBeTerminated(); err != nil {
			traceLog.Error(err, "Hit an error")
			return "", fmt.Errorf("failed to wait for the old pod termination: %v", err)
		}
		// create new pod
		traceLog.Info("Create a new pod and check for an error")
		if err := createNewPod(); err != nil {
			traceLog.Error(err, "Hit an error")
			return "", err
		}
	case stateTerminating:
		// wait for pod to be terminated
		traceLog.Info("Check for an error")
		if err := waitForPodToBeTerminated(); err != nil {
			traceLog.Error(err, "Hit an error")
			return "", fmt.Errorf("failed to wait for the old pod termination: %v", err)
		}
		// create new pod
		traceLog.Info("Create a new pod")
		err := createNewPod()
		traceLog.Info("Check for an error")
		if err != nil {
			traceLog.Error(err, "Hit an error")
			return "", err
		}
	case stateRunning:
		// do nothing
		traceLog.Info("Pod is running, do nothing")
	}

	// wait for pod ip

	watcher, err := r.Clientset.CoreV1().Pods(runnerPodKey.Namespace).Watch(ctx, metav1.SingleObject(metav1.ObjectMeta{
		Name:      runnerPodKey.Name,
		Namespace: runnerPodKey.Namespace,
	}))
	if err != nil {
		return "", fmt.Errorf("failed to create a watch on the pod: %w", err)
	}

	defer watcher.Stop()
	// set a timeout for the watch
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	traceLog.Info("Wait for pod to receive an IP and check for an error")
	for {
		select {
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return "", fmt.Errorf("watch channel closed")
			}

			switch event.Type {
			case watch.Added, watch.Modified:
				runnerPod, ok := event.Object.(*corev1.Pod)
				if !ok {
					return "", fmt.Errorf("failed to cast object to pod: %v", event.Object)
				}

				traceLog.Info("Check if the pod has an IP")
				if runnerPod.Status.PodIP != "" {
					traceLog.Info("Success, pod has an IP")
					return runnerPod.Status.PodIP, nil
				}

				traceLog.Info("Pod does not have an IP yet")
			}
		case <-ctx.Done():
			traceLog.Info("Failed to get the pod, force kill the pod")
			traceLog.Error(err, "Error getting the Pod")

			if err := r.Delete(ctx, &runnerPod,
				client.GracePeriodSeconds(1), // force kill = 1 second
				client.PropagationPolicy(metav1.DeletePropagationForeground),
			); err != nil {
				traceLog.Error(err, "Hit an error")
				return "", fmt.Errorf("failed to obtain pod ip and delete runner pod: %w", err)
			}

			return "", fmt.Errorf("failed to create and obtain pod ip")
		}
	}
}

// reconcileRunnerSecret reconciles the runner secret used for mTLS
//
// It should create the secret if it doesn't exist and then verify that the cert is valid
// if the cert is not present in the secret or is invalid, it will generate a new cert and
// write it to the secret. One secret per namespace is created in order to sidestep the need
// for specifying a pod ip in the certificate SAN field.
func (r *TerraformReconciler) reconcileRunnerSecret(ctx context.Context, terraform *infrav1.Terraform) (*v1.Secret, error) {
	log := controllerruntime.LoggerFrom(ctx)

	log.Info("trigger namespace tls secret generation")

	trigger := mtls.Trigger{
		Namespace: terraform.Namespace,
		Ready:     make(chan *mtls.TriggerResult),
	}
	r.CertRotator.TriggerNamespaceTLSGeneration <- trigger

	result := <-trigger.Ready
	if result.Err != nil {
		return nil, result.Err
	}

	// Check if the secret already exists
	secret := &v1.Secret{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: result.Secret.Name, Namespace: terraform.Namespace}, secret); err != nil {
		if errors.IsNotFound(err) {
			// If secret does not exist, create it
			result.Secret.SetResourceVersion("")
			result.Secret.SetUID("")
			result.Secret.SetGeneration(0)

			if err := r.Client.Create(ctx, result.Secret); err != nil {
				return nil, err
			}
		} else {
			// For any other type of error, return it
			return nil, err
		}
	}

	return result.Secret, nil
}

func runnerPodInstance(revision string) (string, error) {
	parts := strings.Split(revision, ":")

	if len(parts) < 2 {
		return "", fmt.Errorf("invalid revision: %s", revision)
	}

	gitSHA := parts[1]
	if len(gitSHA) < 8 {
		return "", fmt.Errorf("invalid git sha: %s", gitSHA)
	}

	return fmt.Sprintf("tf-runner-%s", gitSHA[0:8]), nil
}
