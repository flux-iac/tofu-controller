package controllers

import (
	"context"
	"strings"
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// newRunnerPodReconciler returns a reconciler that talks to the envtest API
// server directly, so reconcileRunnerPod's pod watch and deletes behave as
// they do against a cluster. Nothing runs the pods it creates.
func newRunnerPodReconciler(t *testing.T, timeout time.Duration) *TerraformReconciler {
	t.Helper()
	clientset, err := kubernetes.NewForConfig(testEnv.Config)
	if err != nil {
		t.Fatal(err)
	}
	return &TerraformReconciler{
		Client:                k8sClient,
		Clientset:             clientset,
		RunnerGRPCPort:        30000,
		RunnerCreationTimeout: timeout,
	}
}

func runnerPodTestTerraform(name string) *infrav1.Terraform {
	obj := fakeTerraform(name)
	obj.UID = types.UID(name + "-uid")
	obj.Spec.RunnerTerminationGracePeriodSeconds = new(int64)
	return obj
}

func runnerPodTestSecret() *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "terraform-runner.tls-1", Namespace: "flux-system"}}
}

func runnerPodKey(obj *infrav1.Terraform) client.ObjectKey {
	return client.ObjectKey{Namespace: obj.Namespace, Name: obj.Name + "-tf-runner"}
}

// runnerPodDeleted reports whether the runner pod is gone or being deleted.
// envtest runs no garbage collector, so a foreground delete leaves the pod in
// place with a deletion timestamp.
func runnerPodDeleted(key client.ObjectKey) bool {
	var pod corev1.Pod
	err := k8sClient.Get(context.Background(), key, &pod)
	return apierrors.IsNotFound(err) || (err == nil && pod.DeletionTimestamp != nil)
}

// removeRunnerPod removes the runner pod for good, finalizers included, when
// the test ends.
func removeRunnerPod(t *testing.T, key client.ObjectKey) {
	t.Cleanup(func() {
		var pod corev1.Pod
		if err := k8sClient.Get(context.Background(), key, &pod); err != nil {
			return
		}
		pod.Finalizers = nil
		_ = k8sClient.Update(context.Background(), &pod)
		_ = k8sClient.Delete(context.Background(), &pod, client.GracePeriodSeconds(0))
		_ = wait.PollUntilContextTimeout(context.Background(), 100*time.Millisecond, 10*time.Second, true, func(ctx context.Context) (bool, error) {
			return apierrors.IsNotFound(k8sClient.Get(ctx, key, &corev1.Pod{})), nil
		})
	})
}

// setRunnerPodStatus waits for the runner pod to be created, then applies
// set to its status, standing in for the kubelet.
func setRunnerPodStatus(key client.ObjectKey, set func(*corev1.PodStatus)) {
	go func() {
		for {
			var pod corev1.Pod
			if err := k8sClient.Get(context.Background(), key, &pod); err == nil {
				set(&pod.Status)
				if err := k8sClient.Status().Update(context.Background(), &pod); err == nil {
					return
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
}

// TestReconcileRunnerPodTimeoutDeletesPod checks that a runner pod that never
// gets an IP is deleted once the creation timeout passes, so the next pass
// starts a fresh one.
func TestReconcileRunnerPodTimeoutDeletesPod(t *testing.T) {
	g := NewWithT(t)
	r := newRunnerPodReconciler(t, 2*time.Second)
	obj := runnerPodTestTerraform("runner-pod-timeout")
	removeRunnerPod(t, runnerPodKey(obj))

	_, err := r.reconcileRunnerPod(context.Background(), obj, runnerPodTestSecret(), "main@sha1:5f75359fced864fd73935cf0a0972e45da1dc626")
	g.Expect(err).To(MatchError("failed to create and obtain pod ip"))

	g.Expect(runnerPodDeleted(runnerPodKey(obj))).To(BeTrue(), "the runner pod should be deleted")
}

// TestReconcileRunnerPodFailedPod checks that a failed runner pod ends the
// wait at once with the pod's reason, and is deleted. That holds both for a
// pod the kubelet rejected before it ran, which has no IP, and for one it
// evicted after it ran, which keeps reporting its last IP.
func TestReconcileRunnerPodFailedPod(t *testing.T) {
	for name, podIP := range map[string]string{
		"rejected before running": "",
		"evicted after running":   "10.0.0.1",
	} {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			timeout := time.Minute
			r := newRunnerPodReconciler(t, timeout)
			obj := runnerPodTestTerraform("runner-pod-failed-" + strings.ReplaceAll(name, " ", "-"))
			key := runnerPodKey(obj)
			removeRunnerPod(t, key)

			setRunnerPodStatus(key, func(status *corev1.PodStatus) {
				status.Phase = corev1.PodFailed
				status.Reason = "Evicted"
				status.Message = "The node had condition: [DiskPressure]."
				status.PodIP = podIP
			})

			start := time.Now()
			_, err := r.reconcileRunnerPod(context.Background(), obj, runnerPodTestSecret(), "main@sha1:5f75359fced864fd73935cf0a0972e45da1dc626")
			g.Expect(err).To(MatchError("runner pod failed: Evicted: The node had condition: [DiskPressure]."))
			g.Expect(time.Since(start)).To(BeNumerically("<", timeout/2), "the wait should end when the pod fails")

			g.Expect(runnerPodDeleted(key)).To(BeTrue(), "the failed runner pod should be deleted")
		})
	}
}

// TestReconcileRunnerPodCannotStart checks that a runner pod whose container
// cannot be created until someone fixes its image, spec or secrets ends the
// wait at once with the kubelet's reason, and is deleted. The kubelet gives
// the pod its IP before it pulls the image, so the status carries an IP.
// An image pull failure, which may be transient, leaves the pod in use as
// before.
func TestReconcileRunnerPodCannotStart(t *testing.T) {
	for name, tc := range map[string]struct {
		reason  string
		message string
		wantErr string // empty: the pod IP is returned and the pod is kept
	}{
		"InvalidImageName": {
			reason:  "InvalidImageName",
			message: `Failed to apply default image tag "tf-runner:": couldn't parse image name`,
			wantErr: `runner pod cannot start: InvalidImageName: Failed to apply default image tag "tf-runner:": couldn't parse image name`,
		},
		"ErrImageNeverPull": {
			reason:  "ErrImageNeverPull",
			message: `Container image "example.com/tf-runner:missing" is not present with pull policy of Never`,
			wantErr: `runner pod cannot start: ErrImageNeverPull: Container image "example.com/tf-runner:missing" is not present with pull policy of Never`,
		},
		"CreateContainerConfigError": {
			reason:  "CreateContainerConfigError",
			message: `secret "runner-env" not found`,
			wantErr: `runner pod cannot start: CreateContainerConfigError: secret "runner-env" not found`,
		},
		"ErrImagePull is used as before": {
			reason:  "ErrImagePull",
			message: "rpc error: code = Unknown desc = failed to pull and unpack image",
		},
		"ImagePullBackOff is used as before": {
			reason:  "ImagePullBackOff",
			message: `Back-off pulling image "example.com/tf-runner:missing"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			timeout := 30 * time.Second
			r := newRunnerPodReconciler(t, timeout)
			obj := runnerPodTestTerraform("runner-pod-" + strings.ToLower(tc.reason))
			key := runnerPodKey(obj)
			removeRunnerPod(t, key)

			setRunnerPodStatus(key, func(status *corev1.PodStatus) {
				status.Phase = corev1.PodPending
				status.PodIP = "10.0.0.2"
				status.ContainerStatuses = []corev1.ContainerStatus{{
					Name:  "tf-runner",
					Image: "example.com/tf-runner:missing",
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
						Reason:  tc.reason,
						Message: tc.message,
					}},
				}}
			})

			start := time.Now()
			ip, err := r.reconcileRunnerPod(context.Background(), obj, runnerPodTestSecret(), "main@sha1:5f75359fced864fd73935cf0a0972e45da1dc626")
			g.Expect(time.Since(start)).To(BeNumerically("<", timeout/2), "the wait should end with the pod status")
			if tc.wantErr == "" {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(ip).To(Equal("10.0.0.2"))
				g.Expect(runnerPodDeleted(key)).To(BeFalse(), "the runner pod should be kept")
				return
			}
			g.Expect(err).To(MatchError(tc.wantErr))
			g.Expect(runnerPodDeleted(key)).To(BeTrue(), "the runner pod should be deleted")
		})
	}
}
