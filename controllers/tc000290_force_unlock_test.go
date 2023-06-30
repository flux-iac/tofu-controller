package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/gomega"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"github.com/weaveworks/tf-controller/utils"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_000290_force_unlock_lock_identifier_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource with inventory enabled.")
	It("should be reconciled successfully with a set of inventory entries shown in the status.")

	const (
		sourceName    = "gr-force-unlock-lock-identifier"
		terraformName = "tf-force-unlock-lock-identifier"
	)
	tfstateLeaseHolderIdentity := "f2ab685b-f84d-ac0b-a125-378a22877e8d"
	g := NewWithT(t)
	ctx := context.Background()

	Given("a GitRepository")
	By("defining a new GitRepository object")
	testRepo := sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceName,
			Namespace: "flux-system",
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: "https://github.com/openshift-fluxv2-poc/podinfo",
			Reference: &sourcev1.GitRepositoryRef{
				Branch: "master",
			},
			Interval: metav1.Duration{Duration: time.Second * 30},
		},
	}
	By("creating the GitRepository object")
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &testRepo)).Should(Succeed()) }()

	Given("that the GitRepository got reconciled")
	By("setting the GitRepository's status, with the BLOB's URL, and the correct checksum")
	updatedTime := time.Now()
	testRepo.Status = sourcev1.GitRepositoryStatus{
		ObservedGeneration: int64(1),
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: updatedTime},
				Reason:             "GitOperationSucceed",
				Message:            "Fetched revision: master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			},
		},

		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/tf-k8s-configmap.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:c3bf30bad9621b5110a3761a70754170d1dae6c525a63098b6ec9919efac3555", // must be the real checksum value
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	testEnvKubeConfigPath, err := findKubeConfig(testEnv)
	g.Expect(err).Should(BeNil())

	Given("a Terraform object with auto approve, and attaching it to the GitRepository object")
	By("creating a new TF resource and attaching to the repo via sourceRef")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			BackendConfig: &infrav1.BackendConfigSpec{
				SecretSuffix:    terraformName,
				InClusterConfig: false,
				ConfigPath:      testEnvKubeConfigPath,
			},
			ApprovePlan: "auto",
			Interval:    metav1.Duration{Duration: 5 * time.Second},
			Path:        "./tf-k8s-configmap",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Vars: []infrav1.Variable{
				{
					Name:  "kubeconfig",
					Value: utils.MustJSONEncodeBytes(t, []byte(testEnvKubeConfigPath)),
				},
				{
					Name:  "context",
					Value: utils.MustJSONEncodeBytes(t, []byte("envtest")),
				},
				{
					Name:  "config_name",
					Value: utils.MustJSONEncodeBytes(t, []byte("cm-"+terraformName)),
				},
			},
		},
	}
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &helloWorldTF)).Should(Succeed()) }()

	It("should be created")
	By("checking that the hello world TF got created")
	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	It("should have conditions reconciled")
	By("checking that the hello world TF's status conditions has some elements")
	g.Eventually(func() int {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return -1
		}
		return len(createdHelloWorldTF.Status.Conditions)
	}, timeout, interval).ShouldNot(BeZero())

	It("should have its plan reconciled")
	By("checking that the Plan's Status of the TF program is Planned Succeed.")
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == infrav1.ConditionTypePlan {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
				}
			}
		}
		return createdHelloWorldTF.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":    infrav1.ConditionTypePlan,
		"Reason":  "TerraformPlannedWithChanges",
		"Message": "Plan generated",
	}))

	It("should generate the Secret containing the plan named with branch and commit id")
	By("checking that the Secret contains plan-master-b8e362c206 in its labels")
	tfplanKey := types.NamespacedName{Namespace: "flux-system", Name: "tfplan-default-" + terraformName}
	tfplanSecret := corev1.Secret{}
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, tfplanKey, &tfplanSecret)
		if err != nil {
			return nil
		}
		return map[string]interface{}{
			"SavedPlan":             tfplanSecret.Annotations["savedPlan"],
			"Is TFPlan empty ?":     string(tfplanSecret.Data["tfplan"]) == "",
			"HasEncodingAnnotation": tfplanSecret.Annotations["encoding"] == "gzip",
		}
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"SavedPlan":             "plan-master-b8e362c206",
		"Is TFPlan empty ?":     false,
		"HasEncodingAnnotation": true,
	}))

	By("checking that the applied status of the TF program Successfully, and plan-master-b8e3 is applied")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == infrav1.ConditionTypeApply {
				return map[string]interface{}{
					"Type":            c.Type,
					"Reason":          c.Reason,
					"Message":         c.Message,
					"LastAppliedPlan": createdHelloWorldTF.Status.Plan.LastApplied,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":            infrav1.ConditionTypeApply,
		"Reason":          infrav1.TFExecApplySucceedReason,
		"Message":         "Applied successfully",
		"LastAppliedPlan": "plan-master-b8e362c206",
	}))

	var tfstateLease coordinationv1.Lease
	tfstateLeaseKey := types.NamespacedName{Namespace: "flux-system", Name: fmt.Sprintf("lock-tfstate-default-%s", terraformName)}
	err = k8sClient.Get(ctx, tfstateLeaseKey, &tfstateLease)

	if err != nil {
		t.Error(err)
	}

	patch := client.MergeFrom(tfstateLease.DeepCopy())
	tfstateLease.Annotations = map[string]string{
		"app.terraform.io/lock-info": fmt.Sprintf("{\"ID\": \"%s\",\"Operation\": \"OperationTypeApply\", \"Info\": \"\", \"Who\":\"%s\", \"Version\": \"1.2.7\", \"Created\": \"2022-08-17T15:44:07.0Z\", \"Path\": \"\"}", tfstateLeaseHolderIdentity, terraformName),
	}
	tfstateLease.Spec.HolderIdentity = &tfstateLeaseHolderIdentity
	err = k8sClient.Patch(ctx, &tfstateLease, patch)

	if err != nil {
		t.Error(err)
	}

	It("should fail to reconcile")
	By("checking that the StateLocked condition exists with Status True")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)

		if err != nil {
			return nil
		}

		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == infrav1.ConditionTypeStateLocked {
				return map[string]interface{}{
					"Type":           c.Type,
					"Status":         c.Status,
					"Reason":         c.Reason,
					"Message":        c.Message,
					"LockIdentifier": createdHelloWorldTF.Status.Lock.Pending,
				}
			}
		}

		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":           infrav1.ConditionTypeStateLocked,
		"Status":         metav1.ConditionTrue,
		"Reason":         infrav1.TFExecLockHeldReason,
		"Message":        fmt.Sprintf("Terraform Locked with Lock Identifier: %s", tfstateLeaseHolderIdentity),
		"LockIdentifier": tfstateLeaseHolderIdentity,
	}))
}

func Test_000290_force_unlock_yes_unlock_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource with inventory enabled.")
	It("should be reconciled successfully with a set of inventory entries shown in the status.")

	const (
		sourceName    = "gr-force-unlock-yes-unlock-test"
		terraformName = "tf-force-unlock-yes-unlock-test"
	)
	tfstateLeaseHolderIdentity := "f2ab685b-f84d-ac0b-a125-378a22877e8d"
	g := NewWithT(t)
	ctx := context.Background()

	Given("a GitRepository")
	By("defining a new GitRepository object")
	testRepo := sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceName,
			Namespace: "flux-system",
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: "https://github.com/openshift-fluxv2-poc/podinfo",
			Reference: &sourcev1.GitRepositoryRef{
				Branch: "master",
			},
			Interval: metav1.Duration{Duration: time.Second * 30},
		},
	}
	By("creating the GitRepository object")
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &testRepo)).Should(Succeed()) }()

	Given("that the GitRepository got reconciled")
	By("setting the GitRepository's status, with the BLOB's URL, and the correct checksum")
	updatedTime := time.Now()
	testRepo.Status = sourcev1.GitRepositoryStatus{
		ObservedGeneration: int64(1),
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: updatedTime},
				Reason:             "GitOperationSucceed",
				Message:            "Fetched revision: master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			},
		},

		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/tf-k8s-configmap.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:c3bf30bad9621b5110a3761a70754170d1dae6c525a63098b6ec9919efac3555", // must be the real checksum value
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	testEnvKubeConfigPath, err := findKubeConfig(testEnv)
	g.Expect(err).Should(BeNil())

	Given("a Terraform object with auto approve, and attaching it to the GitRepository object")
	By("creating a new TF resource and attaching to the repo via sourceRef")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			BackendConfig: &infrav1.BackendConfigSpec{
				SecretSuffix:    terraformName,
				InClusterConfig: false,
				ConfigPath:      testEnvKubeConfigPath,
			},
			ApprovePlan: "auto",
			Interval:    metav1.Duration{Duration: 5 * time.Second},
			Path:        "./tf-k8s-configmap",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Vars: []infrav1.Variable{
				{
					Name:  "kubeconfig",
					Value: utils.MustJSONEncodeBytes(t, []byte(testEnvKubeConfigPath)),
				},
				{
					Name:  "context",
					Value: utils.MustJSONEncodeBytes(t, []byte("envtest")),
				},
				{
					Name:  "config_name",
					Value: utils.MustJSONEncodeBytes(t, []byte("cm-"+terraformName)),
				},
			},
		},
	}
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &helloWorldTF)).Should(Succeed()) }()

	It("should be created")
	By("checking that the hello world TF got created")
	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	It("should have conditions reconciled")
	By("checking that the hello world TF's status conditions has some elements")
	g.Eventually(func() int {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return -1
		}
		return len(createdHelloWorldTF.Status.Conditions)
	}, timeout, interval).ShouldNot(BeZero())

	It("should have its plan reconciled")
	By("checking that the Plan's Status of the TF program is Planned Succeed.")
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == infrav1.ConditionTypePlan {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
				}
			}
		}
		return createdHelloWorldTF.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":    infrav1.ConditionTypePlan,
		"Reason":  "TerraformPlannedWithChanges",
		"Message": "Plan generated",
	}))

	It("should generate the Secret containing the plan named with branch and commit id")
	By("checking that the Secret contains plan-master-b8e362c206 in its labels")
	tfplanKey := types.NamespacedName{Namespace: "flux-system", Name: "tfplan-default-" + terraformName}
	tfplanSecret := corev1.Secret{}
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, tfplanKey, &tfplanSecret)
		if err != nil {
			return nil
		}
		return map[string]interface{}{
			"SavedPlan":             tfplanSecret.Annotations["savedPlan"],
			"Is TFPlan empty ?":     string(tfplanSecret.Data["tfplan"]) == "",
			"HasEncodingAnnotation": tfplanSecret.Annotations["encoding"] == "gzip",
		}
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"SavedPlan":             "plan-master-b8e362c206",
		"Is TFPlan empty ?":     false,
		"HasEncodingAnnotation": true,
	}))

	By("checking that the applied status of the TF program Successfully, and plan-master-b8e3 is applied")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == infrav1.ConditionTypeApply {
				return map[string]interface{}{
					"Type":            c.Type,
					"Reason":          c.Reason,
					"Message":         c.Message,
					"LastAppliedPlan": createdHelloWorldTF.Status.Plan.LastApplied,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":            infrav1.ConditionTypeApply,
		"Reason":          infrav1.TFExecApplySucceedReason,
		"Message":         "Applied successfully",
		"LastAppliedPlan": "plan-master-b8e362c206",
	}))

	var tfstateLease coordinationv1.Lease
	tfstateLeaseKey := types.NamespacedName{Namespace: "flux-system", Name: fmt.Sprintf("lock-tfstate-default-%s", terraformName)}
	err = k8sClient.Get(ctx, tfstateLeaseKey, &tfstateLease)

	if err != nil {
		t.Error(err)
	}

	patch := client.MergeFrom(tfstateLease.DeepCopy())
	tfstateLease.Annotations = map[string]string{
		"app.terraform.io/lock-info": fmt.Sprintf("{\"ID\": \"%s\",\"Operation\": \"OperationTypeApply\", \"Info\": \"\", \"Who\":\"%s\", \"Version\": \"1.2.7\", \"Created\": \"2022-08-17T15:44:07.0Z\", \"Path\": \"\"}", tfstateLeaseHolderIdentity, terraformName),
	}
	tfstateLease.Spec.HolderIdentity = &tfstateLeaseHolderIdentity
	err = k8sClient.Patch(ctx, &tfstateLease, patch)

	if err != nil {
		t.Error(err)
	}

	It("should fail to reconcile")
	By("checking that the StateLocked condition exists with Status True")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == infrav1.ConditionTypeStateLocked {
				return map[string]interface{}{
					"Type":           c.Type,
					"Status":         c.Status,
					"Reason":         c.Reason,
					"Message":        c.Message,
					"LockIdentifier": createdHelloWorldTF.Status.Lock.Pending,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":           infrav1.ConditionTypeStateLocked,
		"Status":         metav1.ConditionTrue,
		"Reason":         infrav1.TFExecLockHeldReason,
		"Message":        fmt.Sprintf("Terraform Locked with Lock Identifier: %s", tfstateLeaseHolderIdentity),
		"LockIdentifier": tfstateLeaseHolderIdentity,
	}))

	patch = client.MergeFrom(createdHelloWorldTF.DeepCopy())
	createdHelloWorldTF.Spec.TFState = &infrav1.TFStateSpec{
		ForceUnlock:    infrav1.ForceUnlockEnumYes,
		LockIdentifier: tfstateLeaseHolderIdentity,
	}
	k8sClient.Patch(ctx, &createdHelloWorldTF, patch)
	expectedTFState := &infrav1.TFStateSpec{
		ForceUnlock:    infrav1.ForceUnlockEnumYes,
		LockIdentifier: tfstateLeaseHolderIdentity,
	}

	It("should reconcile")
	By("checking that the StateLocked condition exists with Status False")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == infrav1.ConditionTypeStateLocked {
				return map[string]interface{}{
					"Type":    c.Type,
					"Status":  c.Status,
					"Reason":  c.Reason,
					"Message": c.Message,
					"TFState": createdHelloWorldTF.Spec.TFState,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":    infrav1.ConditionTypeStateLocked,
		"Status":  metav1.ConditionFalse,
		"Reason":  infrav1.TFExecForceUnlockReason,
		"Message": fmt.Sprintf("Terraform Force Unlock with Lock Identifier: %s", tfstateLeaseHolderIdentity),
		"TFState": expectedTFState,
	}))
}

func Test_000290_force_unlock_auto_unlock_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource with inventory enabled.")
	It("should be reconciled successfully with a set of inventory entries shown in the status.")

	const (
		sourceName    = "gr-force-unlock-auto-unlock-test"
		terraformName = "tf-force-unlock-auto-unlock-test"
	)
	tfstateLeaseHolderIdentity := "f2ab685b-f84d-ac0b-a125-378a22877e8d"
	g := NewWithT(t)
	ctx := context.Background()

	Given("a GitRepository")
	By("defining a new GitRepository object")
	testRepo := sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceName,
			Namespace: "flux-system",
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: "https://github.com/openshift-fluxv2-poc/podinfo",
			Reference: &sourcev1.GitRepositoryRef{
				Branch: "master",
			},
			Interval: metav1.Duration{Duration: time.Second * 30},
		},
	}
	By("creating the GitRepository object")
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &testRepo)).Should(Succeed()) }()

	Given("that the GitRepository got reconciled")
	By("setting the GitRepository's status, with the BLOB's URL, and the correct checksum")
	updatedTime := time.Now()
	testRepo.Status = sourcev1.GitRepositoryStatus{
		ObservedGeneration: int64(1),
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: updatedTime},
				Reason:             "GitOperationSucceed",
				Message:            "Fetched revision: master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			},
		},

		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/tf-k8s-configmap.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:c3bf30bad9621b5110a3761a70754170d1dae6c525a63098b6ec9919efac3555", // must be the real checksum value
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	testEnvKubeConfigPath, err := findKubeConfig(testEnv)
	g.Expect(err).Should(BeNil())

	Given("a Terraform object with auto approve, and attaching it to the GitRepository object")
	By("creating a new TF resource and attaching to the repo via sourceRef")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			BackendConfig: &infrav1.BackendConfigSpec{
				SecretSuffix:    terraformName,
				InClusterConfig: false,
				ConfigPath:      testEnvKubeConfigPath,
			},
			ApprovePlan: "auto",
			Interval:    metav1.Duration{Duration: 5 * time.Second},
			Path:        "./tf-k8s-configmap",
			TFState: &infrav1.TFStateSpec{
				ForceUnlock: infrav1.ForceUnlockEnumAuto,
			},
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Vars: []infrav1.Variable{
				{
					Name:  "kubeconfig",
					Value: utils.MustJSONEncodeBytes(t, []byte(testEnvKubeConfigPath)),
				},
				{
					Name:  "context",
					Value: utils.MustJSONEncodeBytes(t, []byte("envtest")),
				},
				{
					Name:  "config_name",
					Value: utils.MustJSONEncodeBytes(t, []byte("cm-"+terraformName)),
				},
			},
		},
	}
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &helloWorldTF)).Should(Succeed()) }()

	It("should be created")
	By("checking that the hello world TF got created")
	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	It("should have conditions reconciled")
	By("checking that the hello world TF's status conditions has some elements")
	g.Eventually(func() int {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return -1
		}
		return len(createdHelloWorldTF.Status.Conditions)
	}, timeout, interval).ShouldNot(BeZero())

	It("should have its plan reconciled")
	By("checking that the Plan's Status of the TF program is Planned Succeed.")
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == infrav1.ConditionTypePlan {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
				}
			}
		}
		return createdHelloWorldTF.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":    infrav1.ConditionTypePlan,
		"Reason":  "TerraformPlannedWithChanges",
		"Message": "Plan generated",
	}))

	It("should generate the Secret containing the plan named with branch and commit id")
	By("checking that the Secret contains plan-master-b8e362c206 in its labels")
	tfplanKey := types.NamespacedName{Namespace: "flux-system", Name: "tfplan-default-" + terraformName}
	tfplanSecret := corev1.Secret{}
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, tfplanKey, &tfplanSecret)
		if err != nil {
			return nil
		}
		return map[string]interface{}{
			"SavedPlan":             tfplanSecret.Annotations["savedPlan"],
			"Is TFPlan empty ?":     string(tfplanSecret.Data["tfplan"]) == "",
			"HasEncodingAnnotation": tfplanSecret.Annotations["encoding"] == "gzip",
		}
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"SavedPlan":             "plan-master-b8e362c206",
		"Is TFPlan empty ?":     false,
		"HasEncodingAnnotation": true,
	}))

	By("checking that the applied status of the TF program Successfully, and plan-master-b8e3 is applied")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == infrav1.ConditionTypeApply {
				return map[string]interface{}{
					"Type":            c.Type,
					"Reason":          c.Reason,
					"Message":         c.Message,
					"LastAppliedPlan": createdHelloWorldTF.Status.Plan.LastApplied,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":            infrav1.ConditionTypeApply,
		"Reason":          infrav1.TFExecApplySucceedReason,
		"Message":         "Applied successfully",
		"LastAppliedPlan": "plan-master-b8e362c206",
	}))

	var tfstateLease coordinationv1.Lease
	tfstateLeaseKey := types.NamespacedName{Namespace: "flux-system", Name: fmt.Sprintf("lock-tfstate-default-%s", terraformName)}
	err = k8sClient.Get(ctx, tfstateLeaseKey, &tfstateLease)

	if err != nil {
		t.Error(err)
	}

	patch := client.MergeFrom(tfstateLease.DeepCopy())
	tfstateLease.Annotations = map[string]string{
		"app.terraform.io/lock-info": fmt.Sprintf("{\"ID\": \"%s\",\"Operation\": \"OperationTypeApply\", \"Info\": \"\", \"Who\":\"%s\", \"Version\": \"1.2.7\", \"Created\": \"2022-08-17T15:44:07.0Z\", \"Path\": \"\"}", tfstateLeaseHolderIdentity, terraformName),
	}
	tfstateLease.Spec.HolderIdentity = &tfstateLeaseHolderIdentity
	err = k8sClient.Patch(ctx, &tfstateLease, patch)

	if err != nil {
		t.Error(err)
	}

	It("should reconcile")
	By("checking that the StateLocked condition exists with Status False")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			//t.Logf("====\n\n%#v\n\n====", c)
			if c.Type == infrav1.ConditionTypeStateLocked {
				return map[string]interface{}{
					"Type":           c.Type,
					"Status":         c.Status,
					"Reason":         c.Reason,
					"Message":        c.Message,
					"LockIdentifier": createdHelloWorldTF.Status.Lock.Pending,
					"ForceUnlock":    createdHelloWorldTF.Spec.TFState.ForceUnlock,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":           infrav1.ConditionTypeStateLocked,
		"Status":         metav1.ConditionFalse,
		"Reason":         infrav1.TFExecForceUnlockReason,
		"Message":        fmt.Sprintf("Terraform Force Unlock with Lock Identifier: %s", tfstateLeaseHolderIdentity),
		"LockIdentifier": "",
		"ForceUnlock":    infrav1.ForceUnlockEnumAuto,
	}))
}
