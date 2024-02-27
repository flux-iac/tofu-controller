package controllers

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	types2 "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/elgohr/go-localstack"
	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000111_with_backend_s3_no_outputs_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource with backend configured, and `auto` approve.")
	It("should be reconciled from the plan state, to the apply state and have a correct TFSTATE stored inside the cluster as a Secret.")

	const (
		sourceName    = "test-tf-with-s3-backend-no-output"
		terraformName = "helloworld-with-s3-backend-no-outputs"
	)

	g := NewWithT(t)
	ctx := context.Background()

	Given("a GitRepository")
	By("defining a new Git repository resource.")
	updatedTime := time.Now()
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
	By("creating the GitRepository resource in the cluster.")
	It("should be created successfully.")
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())
	defer waitResourceToBeDelete(g, &testRepo)

	Given("the GitRepository's reconciled status.")
	By("setting the GitRepository's status, with the downloadable BLOB's URL, and the correct checksum.")
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
			URL:            server.URL() + "/file.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}

	It("should be updated successfully.")
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	It("should be retrievable by the k8s client.")
	By("getting it with the k8s client and succeed.")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, &createdRepo)).Should(Succeed())

	By("preparing s3-backend-configs secret")
	s3BackendConfigs := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "s3-backend-configs",
			Namespace: "flux-system",
		},
		Data: map[string][]byte{
			"access_key":  []byte("test"),
			"secret_key":  []byte("test"),
			"bucket":      []byte("s3-terraform-state"),
			"invalid_key": []byte("invalid-key"),
		},
		Type: corev1.SecretTypeOpaque,
	}
	g.Expect(k8sClient.Create(ctx, &s3BackendConfigs)).Should(Succeed())
	defer waitResourceToBeDelete(g, &s3BackendConfigs)

	var stack *localstack.Instance
	{
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		var err error
		stack, err = localstack.NewInstance()
		if err != nil {
			log.Fatalf("Could not connect to Docker %v", err)
		}
		if err := stack.StartWithContext(ctx,
			localstack.S3,
			localstack.DynamoDB,
			localstack.SQS); err != nil {
			log.Fatalf("Could not start localstack %v", err)
		}

		cfg, err := config.LoadDefaultConfig(ctx,
			config.WithRegion("us-east-1"),
			config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(func(_, _ string, _ ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					PartitionID:       "aws",
					URL:               stack.EndpointV2(localstack.SQS),
					SigningRegion:     "us-east-1",
					HostnameImmutable: true,
				}, nil
			})),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "test")),
		)
		if err != nil {
			log.Fatalf("Could not get config %v", err)
		}

		s3Client := s3.NewFromConfig(cfg)
		By("creating the S3 bucket.")

		_, err = s3Client.CreateBucket(context.TODO(), &s3.CreateBucketInput{
			Bucket: aws.String("s3-terraform-state"),
		})
		g.Expect(err).Should(Succeed())

		// create dynamo table
		By("creating the DynamoDB table.")
		dynamodbClient := dynamodb.NewFromConfig(cfg)
		_, err = dynamodbClient.CreateTable(ctx, &dynamodb.CreateTableInput{
			TableName: aws.String("terraformlock"),
			AttributeDefinitions: []types2.AttributeDefinition{
				{
					AttributeName: aws.String("LockID"),
					AttributeType: "S",
				},
			},
			KeySchema: []types2.KeySchemaElement{
				{
					AttributeName: aws.String("LockID"),
					KeyType:       "HASH",
				},
			},
			ProvisionedThroughput: &types2.ProvisionedThroughput{
				ReadCapacityUnits:  aws.Int64(5),
				WriteCapacityUnits: aws.Int64(5),
			},
		})
		g.Expect(err).Should(Succeed())

		stack.EndpointV2(localstack.DynamoDB)
	}

	Given("a Terraform resource with auto approve, backend configured, attached to the given GitRepository.")
	By("creating a new TF resource and attaching to the repo via `sourceRef`.")
	backendConfig := fmt.Sprintf(`
	backend "s3" {
		key                         = "dev/terraform.tfstate"
		region                      = "us-east-1"
		endpoint                    = "%s"
		skip_credentials_validation = true
		skip_metadata_api_check     = true
		force_path_style            = true
		dynamodb_table              = "terraformlock"
		dynamodb_endpoint           = "%s"
		encrypt                     = true
    }
	`, stack.EndpointV2(localstack.S3), stack.EndpointV2(localstack.DynamoDB))
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			BackendConfig: &infrav1.BackendConfigSpec{
				CustomConfiguration: backendConfig,
			},
			BackendConfigsFrom: []infrav1.BackendConfigsReference{
				{
					Kind: "Secret",
					Name: s3BackendConfigs.Name,
					Keys: []string{
						"access_key",
						"secret_key",
						"bucket",
					},
					Optional: false,
				},
			},
			Path: "./terraform-hello-world-example",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Interval: metav1.Duration{Duration: time.Second * 10},
		},
	}
	It("should be created and attached successfully.")
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer waitResourceToBeDelete(g, &helloWorldTF)

	By("checking that the TF resource existed inside the cluster.")
	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	It("should be reconciled and contain some status conditions.")
	By("checking that the TF resource's status conditions has some elements.")
	g.Eventually(func() int {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return -1
		}
		return len(createdHelloWorldTF.Status.Conditions)
	}, timeout, interval).ShouldNot(BeZero())

	It("should apply successfully.")
	By("checking that the status of the TF resource is `TerraformAppliedSucceed`.")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Apply" {
				return map[string]interface{}{
					"Type":   c.Type,
					"Reason": c.Reason,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":   infrav1.ConditionTypeApply,
		"Reason": infrav1.TFExecApplySucceedReason,
	}))

	It("should be reconciled successfully, and have some output available.")
	By("checking the output reason of the TF resource being `TerraformOutputsAvailable`.")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Output" {
				return map[string]interface{}{
					"Type":   c.Type,
					"Reason": c.Reason,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":   infrav1.ConditionTypeOutput,
		"Reason": "TerraformOutputsAvailable",
	}))

	By("checking that the .status.availableOutputs contains an output named `hello_world`.")
	g.Eventually(func() []string {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		return createdHelloWorldTF.Status.AvailableOutputs
	}, timeout, interval).Should(Equal([]string{"hello_world"}))

}
