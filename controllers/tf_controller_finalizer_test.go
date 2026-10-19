package controllers

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
)

// finalizeSecretsRunner fails FinalizeSecrets with err. Any other call panics on
// the nil embedded client, which would mean finalize went further than expected.
type finalizeSecretsRunner struct {
	runner.RunnerClient
	err error
}

func (f finalizeSecretsRunner) FinalizeSecrets(context.Context, *runner.FinalizeSecretsRequest, ...grpc.CallOption) (*runner.FinalizeSecretsReply, error) {
	return nil, f.err
}

func TestFinalizeRequeuesWhenRunnerUnavailable(t *testing.T) {
	g := NewWithT(t)

	terraform := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "finalize-unavailable",
			Namespace:  "flux-system",
			Finalizers: []string{infrav1.TerraformFinalizer},
		},
	}
	runnerClient := finalizeSecretsRunner{err: status.Error(codes.Unavailable, "runner unreachable")}

	reconciler := &TerraformReconciler{}
	terraform, result, err := reconciler.finalize(context.Background(), nil, terraform, runnerClient, nil, "")

	g.Expect(status.Code(err)).To(Equal(codes.Unavailable))
	g.Expect(result.Requeue).To(BeTrue())
	g.Expect(terraform.Finalizers).To(ContainElement(infrav1.TerraformFinalizer))
}
