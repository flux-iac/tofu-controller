package config_test

import (
	"context"
	"os"
	"testing"

	"github.com/flux-iac/tofu-controller/internal/config"
	"github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_ObjectKeyFromName_invalid(t *testing.T) {
	for _, s := range []string{
		"foo/bar/baz",
		"/bar",
		"foo/",
		"",
	} {
		t.Run(s, func(t *testing.T) {
			_, err := config.ObjectKeyFromName(s)
			if err == nil {
				t.Fatal("expected error due to invalid object name, got nil")
			}
		})
	}
}

func Test_ReadConfig_empty(t *testing.T) {
	g := gomega.NewWithT(t)
	targetNS := "separate-ns"

	os.Setenv("RUNTIME_NAMESPACE", targetNS)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "branch-planner-config",
			Namespace: "separate-ns",
		},
		Data: map[string]string{},
	}
	objects := []client.Object{configMap}

	fakeClient := fake.NewClientBuilder().WithObjects(objects...).Build()

	conf, err := config.ReadConfig(context.Background(), fakeClient, types.NamespacedName{
		Name: "branch-planner-config",
	})
	g.Expect(err).To(gomega.Succeed())

	g.Expect(conf.SecretName).To(gomega.Equal(""))
	g.Expect(conf.SecretNamespace).To(gomega.Equal(targetNS))
	g.Expect(conf.Resources).To(gomega.HaveLen(0))
}

func Test_ReadConfig_resources(t *testing.T) {
	g := gomega.NewWithT(t)
	targetNS := "separate-ns"

	os.Setenv("RUNTIME_NAMESPACE", targetNS)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "branch-planner-config",
			Namespace: "separate-ns",
		},
		Data: map[string]string{
			"resources": func() string {
				resources := []client.ObjectKey{
					{Name: "mytf-1", Namespace: "myns"},
					{Name: "mytf-2"},
					{Namespace: "myns"},
				}

				data, _ := yaml.Marshal(resources)

				return string(data)
			}(),
		},
	}

	objects := []client.Object{configMap}

	fakeClient := fake.NewClientBuilder().WithObjects(objects...).Build()

	conf, err := config.ReadConfig(context.Background(), fakeClient, types.NamespacedName{
		Name: "branch-planner-config",
	})
	g.Expect(err).To(gomega.Succeed())

	g.Expect(conf.SecretName).To(gomega.Equal(""))
	g.Expect(conf.SecretNamespace).To(gomega.Equal(targetNS))
	g.Expect(conf.Resources).To(gomega.HaveLen(3))
	g.Expect(conf.Resources[0].Name).To(gomega.Equal("mytf-1"))
	g.Expect(conf.Resources[0].Namespace).To(gomega.Equal("myns"))
	g.Expect(conf.Resources[1].Name).To(gomega.Equal("mytf-2"))
	g.Expect(conf.Resources[1].Namespace).To(gomega.Equal(targetNS))
	g.Expect(conf.Resources[2].Name).To(gomega.Equal(""))
	g.Expect(conf.Resources[2].Namespace).To(gomega.Equal("myns"))
}

func Test_RuntimeNamespace(t *testing.T) {
	g := gomega.NewWithT(t)
	runtimeNamespace := "runtime-namespace"

	os.Setenv("RUNTIME_NAMESPACE", runtimeNamespace)
	g.Expect(config.RuntimeNamespace(), runtimeNamespace)

	os.Unsetenv("RUNTIME_NAMESPACE")
	g.Expect(config.RuntimeNamespace(), config.DefaultNamespace)
}
