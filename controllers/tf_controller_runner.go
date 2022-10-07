package controllers

import (
	"context"
	"fmt"
	"os"
	"time"

	errors2 "github.com/pkg/errors"
	"github.com/weaveworks/tf-controller/api/v1alpha1"
	"github.com/weaveworks/tf-controller/mtls"
	"github.com/weaveworks/tf-controller/runner"
	"google.golang.org/grpc"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func getRunnerPodObjectKey(terraform v1alpha1.Terraform) types.NamespacedName {
	return types.NamespacedName{Namespace: terraform.Namespace, Name: fmt.Sprintf("%s-tf-runner", terraform.Name)}
}

func getRunnerPodImage(image string) string {
	runnerPodImage := image
	if runnerPodImage == "" {
		runnerPodImage = os.Getenv("RUNNER_POD_IMAGE")
	}
	if runnerPodImage == "" {
		runnerPodImage = "ghcr.io/weaveworks/tf-runner:latest"
	}
	return runnerPodImage
}

func runnerPodTemplate(terraform v1alpha1.Terraform, secretName string) v1.Pod {
	podNamespace := terraform.Namespace
	podName := fmt.Sprintf("%s-tf-runner", terraform.Name)
	runnerPodTemplate := v1.Pod{
		ObjectMeta: v12.ObjectMeta{
			Namespace: podNamespace,
			Name:      podName,
			Labels: map[string]string{
				"app.kubernetes.io/created-by":   "tf-controller",
				"app.kubernetes.io/name":         "tf-runner",
				"app.kubernetes.io/instance":     podName,
				v1alpha1.RunnerLabel:             terraform.Namespace,
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
	return runnerPodTemplate
}

func (r *TerraformReconciler) LookupOrCreateRunner(ctx context.Context, terraform v1alpha1.Terraform) (runner.RunnerClient, func() error, error) {
	// we have to make sure that the secret is valid before we can create the runner.
	secret, err := r.reconcileRunnerSecret(ctx, &terraform)
	if err != nil {
		return nil, nil, err
	}

	var hostname string
	if os.Getenv("INSECURE_LOCAL_RUNNER") == "1" {
		hostname = "localhost"
	} else {
		podIP, err := r.reconcileRunnerPod(ctx, terraform, secret)
		if err != nil {
			return nil, nil, err
		}
		hostname = terraform.GetRunnerHostname(podIP)
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
	defer dialCancel()
	conn, err := r.getRunnerConnection(dialCtx, secret, hostname, r.RunnerGRPCPort)
	if err != nil {
		return nil, nil, err
	}
	connClose := func() error { return conn.Close() }
	runnerClient := runner.NewRunnerClient(conn)
	return runnerClient, connClose, nil
}

func (r *TerraformReconciler) getRunnerConnection(ctx context.Context, tlsSecret *v1.Secret, hostname string, port int) (*grpc.ClientConn, error) {
	addr := fmt.Sprintf("%s:%d", hostname, port)
	credentials, err := mtls.GetGRPCClientCredentials(tlsSecret)
	if err != nil {
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

	return grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(credentials),
		grpc.WithBlock(),
		grpc.WithDefaultServiceConfig(retryPolicy),
	)
}

func (r *TerraformReconciler) runnerPodSpec(terraform v1alpha1.Terraform, tlsSecretName string) v1.PodSpec {
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

	return v1.PodSpec{
		TerminationGracePeriodSeconds: gracefulTermPeriod,
		Containers: []v1.Container{
			{
				Name: "tf-runner",
				Args: []string{
					"--grpc-port", fmt.Sprintf("%d", r.RunnerGRPCPort),
					"--tls-secret-name", tlsSecretName,
					"--grpc-max-message-size", fmt.Sprintf("%d", r.RunnerGRPCMaxMessageSize),
				},
				Image:           getRunnerPodImage(terraform.Spec.RunnerPodTemplate.Spec.Image),
				ImagePullPolicy: v1.PullIfNotPresent,
				Ports: []v1.ContainerPort{
					{
						Name:          "grpc",
						ContainerPort: int32(r.RunnerGRPCPort),
					},
				},
				Env:     envvars,
				EnvFrom: terraform.Spec.RunnerPodTemplate.Spec.EnvFrom,
				// TODO: this security context might break OpenShift because of SCC. We need verification.
				// TODO how to support it via Spec or Helm Chart
				SecurityContext: &v1.SecurityContext{
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
				},
				VolumeMounts: podVolumeMounts,
			},
		},
		Volumes:            podVolumes,
		ServiceAccountName: serviceAccountName,
		NodeSelector:       terraform.Spec.RunnerPodTemplate.Spec.NodeSelector,
		Affinity:           terraform.Spec.RunnerPodTemplate.Spec.Affinity,
		Tolerations:        terraform.Spec.RunnerPodTemplate.Spec.Tolerations,
	}
}

func (r *TerraformReconciler) reconcileRunnerPod(ctx context.Context, terraform v1alpha1.Terraform, tlsSecret *v1.Secret) (string, error) {
	log := controllerruntime.LoggerFrom(ctx)
	type state string
	const (
		stateUnknown       state = "unknown"
		stateRunning       state = "running"
		stateNotFound      state = "not-found"
		stateMustBeDeleted state = "must-be-deleted"
		stateTerminating   state = "terminating"
	)

	const interval = time.Second * 15
	timeout := r.RunnerCreationTimeout // default is 120 seconds
	tlsSecretName := tlsSecret.Name

	createNewPod := func() error {
		runnerPodTemplate := runnerPodTemplate(terraform, tlsSecretName)
		newRunnerPod := *runnerPodTemplate.DeepCopy()
		newRunnerPod.Spec = r.runnerPodSpec(terraform, tlsSecretName)
		if err := r.Create(ctx, &newRunnerPod); err != nil {
			return err
		}
		return nil
	}

	waitForPodToBeTerminated := func() error {
		runnerPodTemplate := runnerPodTemplate(terraform, tlsSecretName)
		runnerPod := *runnerPodTemplate.DeepCopy()
		runnerPodKey := client.ObjectKeyFromObject(&runnerPod)
		err := wait.PollImmediate(interval, timeout, func() (bool, error) {
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

	runnerPodTemplate := runnerPodTemplate(terraform, tlsSecretName)
	runnerPod := *runnerPodTemplate.DeepCopy()
	runnerPodKey := client.ObjectKeyFromObject(&runnerPod)
	err := r.Get(ctx, runnerPodKey, &runnerPod)
	if err != nil && errors.IsNotFound(err) {
		podState = stateNotFound
	} else if err == nil {
		label, found := runnerPod.Labels["tf.weave.works/tls-secret-name"]
		if !found || label != tlsSecretName {
			podState = stateMustBeDeleted
		} else if runnerPod.DeletionTimestamp != nil {
			podState = stateTerminating
		} else if runnerPod.Status.Phase == v1.PodRunning {
			podState = stateRunning
		}
	}

	log.Info("show runner pod state: ", "name", terraform.Name, "state", podState)

	switch podState {
	case stateNotFound:
		// create new pod
		err := createNewPod()
		if err != nil {
			return "", err
		}
	case stateMustBeDeleted:
		// delete old pod
		if err := r.Delete(ctx, &runnerPod,
			client.GracePeriodSeconds(1), // force kill = 1 second
			client.PropagationPolicy(v12.DeletePropagationForeground),
		); err != nil {
			return "", err
		}
		// wait for pod to be terminated
		if err := waitForPodToBeTerminated(); err != nil {
			return "", fmt.Errorf("failed to wait for the old pod termination: %v", err)
		}
		// create new pod
		if err := createNewPod(); err != nil {
			return "", err
		}
	case stateTerminating:
		// wait for pod to be terminated
		if err := waitForPodToBeTerminated(); err != nil {
			return "", fmt.Errorf("failed to wait for the old pod termination: %v", err)
		}
		// create new pod
		err := createNewPod()
		if err != nil {
			return "", err
		}
	case stateRunning:
		// do nothing
	}

	// wait for pod ip
	if wait.Poll(interval, timeout, func() (bool, error) {
		if err := r.Get(ctx, runnerPodKey, &runnerPod); err != nil {
			return false, fmt.Errorf("failed to get runner pod: %w", err)
		}

		if runnerPod.Status.PodIP != "" {
			return true, nil
		}

		return false, nil
	}) != nil {

		if err := r.Delete(ctx, &runnerPod,
			client.GracePeriodSeconds(1), // force kill = 1 second
			client.PropagationPolicy(v12.DeletePropagationForeground),
		); err != nil {
			return "", fmt.Errorf("failed to obtain pod ip and delete runner pod: %w", err)
		}

		return "", fmt.Errorf("failed to create and obtain pod ip")
	}

	return runnerPod.Status.PodIP, nil
}

// reconcileRunnerSecret reconciles the runner secret used for mTLS
//
// It should create the secret if it doesn't exist and then verify that the cert is valid
// if the cert is not present in the secret or is invalid, it will generate a new cert and
// write it to the secret. One secret per namespace is created in order to sidestep the need
// for specifying a pod ip in the certificate SAN field.
func (r *TerraformReconciler) reconcileRunnerSecret(ctx context.Context, terraform *v1alpha1.Terraform) (*v1.Secret, error) {
	log := controllerruntime.LoggerFrom(ctx)

	log.Info("trigger namespace tls secret generation")

	trigger := mtls.Trigger{
		Namespace: terraform.Namespace,
		Ready:     make(chan *mtls.TriggerResult),
	}
	r.CertRotator.TriggerNamespaceTLSGeneration <- trigger

	result := <-trigger.Ready
	if result.Err != nil {
		return nil, errors2.Wrap(result.Err, "failed to get tls generation result")
	}

	return result.Secret, nil
}
