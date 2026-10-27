package controllers

import (
	"context"
	"fmt"
	"maps"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/mtls"
	"github.com/flux-iac/tofu-controller/runner"
)

func getRunnerPodObjectKey(terraform *infrav1.Terraform) types.NamespacedName {
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

func runnerPodTemplate(terraform *infrav1.Terraform, secretName string, revision string) (v1.Pod, error) {
	podNamespace := terraform.Namespace
	podName := fmt.Sprintf("%s-tf-runner", terraform.Name)
	podInstance, err := runnerPodInstance(terraform, revision)
	if err != nil {
		return v1.Pod{}, err
	}

	runnerPodTemplate := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: podNamespace,
			Name:      podName,
			// The controller ownerReference classifies the runner as a managed pod
			// (kube-state-metrics' created_by_kind) rather than a bare pod that appears
			// to restart on every reconcile, and lets Kubernetes GC the runner if the
			// Terraform object is deleted.
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: infrav1.GroupVersion.String(),
					Kind:       infrav1.TerraformKind,
					Name:       terraform.Name,
					UID:        terraform.UID,
					Controller: ptr.To(true),
				},
			},
			Labels: map[string]string{
				"app.kubernetes.io/created-by":   "tofu-controller",
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
		maps.Copy(runnerPodTemplate.Labels, terraform.Spec.RunnerPodTemplate.Metadata.Labels)
	}
	return runnerPodTemplate, nil
}

func (r *TerraformReconciler) LookupOrCreateRunner(ctx context.Context, terraform *infrav1.Terraform, revision string) (runner.RunnerClient, func() error, error) {
	// we have to make sure that the secret is valid before we can create the runner.
	secret, err := r.reconcileRunnerSecret(ctx, terraform)
	if err != nil {
		return nil, nil, err
	}

	var hostname string
	if os.Getenv("INSECURE_LOCAL_RUNNER") == "1" {
		hostname = "localhost"
	} else {
		podIP, err := r.reconcileRunnerPod(ctx, terraform, secret, revision)
		if err != nil {
			return nil, nil, err
		}
		if r.UsePodSubdomainResolution {
			hostname = terraform.GetRunnerHostname(terraform.Name, r.ClusterDomain)
		} else {
			hostname = terraform.GetRunnerHostname(podIP, r.ClusterDomain)
		}
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
	credentials, err := mtls.GetGRPCClientCredentials(tlsSecret)
	if err != nil {
		return nil, err
	}

	maxMsgSize := r.RunnerGRPCMaxMessageSize * 1024 * 1024
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials),
		grpc.WithDefaultServiceConfig(runnerServiceConfig),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxMsgSize),
			grpc.MaxCallSendMsgSize(maxMsgSize),
		),
		grpc.WithChainUnaryInterceptor(
			runnerErrorInterceptor(),
			runnerUnreachableInterceptor(r.RunnerUnreachableTimeout),
		),
	}
	if r.RunnerUnreachableTimeout > 0 {
		// If the runner's node disappears without closing the TCP connection, a
		// call waiting on its reply sends nothing, so the connection stays READY
		// until the OS keepalive gives up hours later. Ping during calls so the
		// watchdog above gets to see it. Time must stay above the runner's default
		// 5m ping enforcement minimum, or the runner answers with GOAWAY
		// "too_many_pings". Timeout is generous so that a short network blip does
		// not drop the transport of a healthy apply.
		opts = append(opts, grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:    6 * time.Minute,
			Timeout: 2 * time.Minute,
		}))
	}
	return grpc.NewClient(fmt.Sprintf("%s:%d", hostname, port), opts...)
}

const runnerServiceConfig = `{
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

func (r *TerraformReconciler) runnerPodSpec(terraform *infrav1.Terraform, tlsSecretName string) v1.PodSpec {
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
				ImagePullPolicy: v1.PullIfNotPresent,
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

func (r *TerraformReconciler) reconcileRunnerPod(ctx context.Context, terraform *infrav1.Terraform, tlsSecret *v1.Secret, revision string) (string, error) {
	log := ctrl.LoggerFrom(ctx)
	type state string
	const (
		stateUnknown       state = "unknown"
		stateFailed        state = "failed"
		stateRunning       state = "running"
		stateNotFound      state = "not-found"
		stateMustBeDeleted state = "must-be-deleted"
		stateTerminating   state = "terminating"
	)

	const interval = time.Second * 15
	timeout := r.RunnerCreationTimeout // default is 120 seconds
	tlsSecretName := tlsSecret.Name

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

	runnerPodTemplate, err := runnerPodTemplate(terraform, tlsSecretName, revision)
	if err != nil {
		return "", err
	}

	runnerPod := *runnerPodTemplate.DeepCopy()
	runnerPodKey := client.ObjectKeyFromObject(&runnerPod)
	err = r.Get(ctx, runnerPodKey, &runnerPod)

	gracefulTermPeriod := *terraform.Spec.RunnerTerminationGracePeriodSeconds
	if err != nil && errors.IsNotFound(err) {
		podState = stateNotFound
	} else if err != nil {
		return "", fmt.Errorf("failed to get the runner pod: %w", err)
	} else if err == nil {
		label, found := runnerPod.Labels["tf.weave.works/tls-secret-name"]
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
		} else if runnerPod.Status.Phase == v1.PodFailed {
			podState = stateFailed
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
			client.GracePeriodSeconds(gracefulTermPeriod),
			client.PropagationPolicy(metav1.DeletePropagationForeground),
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
	case stateFailed:
		// A failed pod will never serve gRPC again, so delete it and create a
		// replacement. Other non-running phases are left alone to continue
		// starting up.
		log.Info("runner pod failed, force-deleting", "name", terraform.Name)
		if err := r.Delete(ctx, &runnerPod,
			client.GracePeriodSeconds(1), // force kill = 1 second
			client.PropagationPolicy(metav1.DeletePropagationForeground),
		); err != nil && !errors.IsNotFound(err) {
			return "", err
		}
		if err := waitForPodToBeTerminated(); err != nil {
			return "", fmt.Errorf("failed to wait for the stale pod termination: %v", err)
		}
		if err := createNewPod(); err != nil {
			return "", err
		}
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
	watchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

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

				// A failed pod will never serve, for example one the kubelet
				// rejects or evicts under disk pressure. One evicted after it
				// ran still reports its last IP, so check before the IP.
				if runnerPod.Status.Phase == corev1.PodFailed {
					return "", r.deleteUnusableRunnerPod(ctx, runnerPod,
						fmt.Errorf("runner pod failed: %s: %s", runnerPod.Status.Reason, runnerPod.Status.Message))
				}

				// The kubelet gives the pod its IP before it pulls the image
				// or builds the container config, so this too comes first.
				if waiting := runnerContainerCannotStart(runnerPod); waiting != nil {
					return "", r.deleteUnusableRunnerPod(ctx, runnerPod,
						fmt.Errorf("runner pod cannot start: %s: %s", waiting.Reason, waiting.Message))
				}

				if runnerPod.Status.PodIP != "" {
					return runnerPod.Status.PodIP, nil
				}
			}
		case <-watchCtx.Done():
			return "", r.deleteUnusableRunnerPod(ctx, &runnerPod, fmt.Errorf("failed to create and obtain pod ip"))
		}
	}
}

// containerCannotStartReasons are the kubelet's waiting reasons for a
// container that will not start until someone fixes the pod's image, spec or
// secrets. Image pull failures (ErrImagePull, ImagePullBackOff) are left out:
// the kubelet reports them after a single failed pull, including a transient
// registry error, and retries with its own backoff.
var containerCannotStartReasons = map[string]bool{
	"InvalidImageName":           true,
	"ErrImageNeverPull":          true,
	"CreateContainerConfigError": true,
}

// runnerContainerCannotStart returns the waiting state of a container in pod
// that cannot start, or nil.
func runnerContainerCannotStart(pod *corev1.Pod) *corev1.ContainerStateWaiting {
	for _, status := range pod.Status.ContainerStatuses {
		if waiting := status.State.Waiting; waiting != nil && containerCannotStartReasons[waiting.Reason] {
			return waiting
		}
	}
	return nil
}

// deleteUnusableRunnerPod force-deletes a runner pod that will never serve,
// so the next pass creates a new one, and returns cause. The delete gets a
// context of its own: the wait that gave up on the pod may have used up ctx's
// deadline.
func (r *TerraformReconciler) deleteUnusableRunnerPod(ctx context.Context, pod *corev1.Pod, cause error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	if err := r.Delete(ctx, pod,
		client.GracePeriodSeconds(1), // force kill = 1 second
		client.PropagationPolicy(metav1.DeletePropagationForeground),
	); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("%w; failed to delete the runner pod: %v", cause, err)
	}

	return cause
}

// reconcileRunnerSecret reconciles the runner secret used for mTLS
//
// It should create the secret if it doesn't exist and then verify that the cert is valid
// if the cert is not present in the secret or is invalid, it will generate a new cert and
// write it to the secret. One secret per namespace is created in order to sidestep the need
// for specifying a pod ip in the certificate SAN field.
func (r *TerraformReconciler) reconcileRunnerSecret(ctx context.Context, terraform *infrav1.Terraform) (*v1.Secret, error) {
	log := ctrl.LoggerFrom(ctx)

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
	if err := r.Get(ctx, types.NamespacedName{Name: result.Secret.Name, Namespace: terraform.Namespace}, secret); err != nil {
		if errors.IsNotFound(err) {
			// If secret does not exist, create it
			result.Secret.SetResourceVersion("")
			result.Secret.SetUID("")
			result.Secret.SetGeneration(0)

			if err := r.Create(ctx, result.Secret); err != nil && !errors.IsAlreadyExists(err) {
				return nil, err
			}
		} else {
			// For any other type of error, return it
			return nil, err
		}
	}

	return result.Secret, nil
}

// finalizeRunnerPodInstance is the runner pod instance label when there is no
// revision to derive it from. It cannot collide with tf-runner-<sha8>: it is
// not hex.
const finalizeRunnerPodInstance = "tf-runner-finalize"

// runnerPodInstance returns the runner pod's app.kubernetes.io/instance label,
// tf-runner-<sha8>, from the source revision. An unparsable source revision
// is an error. A deletion without destroy has no source revision (""), so the
// label comes from status.lastAttemptedRevision, then lastAppliedRevision, and
// otherwise is finalizeRunnerPodInstance.
func runnerPodInstance(terraform *infrav1.Terraform, revision string) (string, error) {
	if revision != "" {
		return runnerPodInstanceOf(revision)
	}
	for _, rev := range []string{terraform.Status.LastAttemptedRevision, terraform.Status.LastAppliedRevision} {
		if instance, err := runnerPodInstanceOf(rev); err == nil {
			return instance, nil
		}
	}
	return finalizeRunnerPodInstance, nil
}

func runnerPodInstanceOf(revision string) (string, error) {
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

// cleanupRunnerPod deletes the runner pod and waits until it is gone, unless
// the runner is local or spec.alwaysCleanupRunnerPod is false.
func (r *TerraformReconciler) cleanupRunnerPod(ctx context.Context, terraform *infrav1.Terraform) {
	log := ctrl.LoggerFrom(ctx)

	if os.Getenv("INSECURE_LOCAL_RUNNER") == "1" {
		// nothing to delete
		log.Info("insecure local runner")
		return
	}

	if !terraform.Spec.GetAlwaysCleanupRunnerPod() {
		return
	}

	// wait for runner pod complete termination
	var (
		interval = time.Second * 5
		timeout  = time.Second * 120
	)
	err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
		var runnerPod corev1.Pod
		err := r.Get(ctx, getRunnerPodObjectKey(terraform), &runnerPod)

		if err != nil && errors.IsNotFound(err) {
			return true, nil
		}

		if err := r.Delete(ctx, &runnerPod,
			client.GracePeriodSeconds(1), // force kill = 1 second
			client.PropagationPolicy(metav1.DeletePropagationForeground),
		); err != nil {
			log.Error(err, "unable to delete pod")
			return false, nil
		}

		return false, err
	})

	if err != nil {
		log.Error(fmt.Errorf("failed waiting for the terminating runner pod: %v", err), "error in polling")
	}
}
