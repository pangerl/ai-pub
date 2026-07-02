package executor

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"ai-pub/internal/domain"
	"ai-pub/internal/repository"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

type fakeK8sClusters map[string]domain.K8sCluster

func (f fakeK8sClusters) GetK8sCluster(_ context.Context, id string) (domain.K8sCluster, error) {
	item, ok := f[id]
	if !ok {
		return domain.K8sCluster{}, repository.ErrNotFound
	}
	return item, nil
}

func TestK8sExecutorPatchesOnlyTargetContainerImage(t *testing.T) {
	replicas := int32(2)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "order-api",
			Namespace:  "default",
			Generation: 3,
			Labels:     map[string]string{"app": "order-api"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "order-api"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "registry.example.com/order-api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						Env:   []corev1.EnvVar{{Name: "KEEP", Value: "yes"}},
						Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("500m"),
						}},
						ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"true"}}}},
					},
					{Name: "sidecar", Image: "registry.example.com/sidecar@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
				}},
			},
		},
		Status: appsv1.DeploymentStatus{ObservedGeneration: 3, UpdatedReplicas: 2, AvailableReplicas: 2},
	}
	client := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}, deployment)
	var patchBody map[string]any
	client.Fake.PrependReactor("patch", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		patch := action.(ktesting.PatchAction)
		if err := json.Unmarshal(patch.GetPatch(), &patchBody); err != nil {
			t.Fatal(err)
		}
		updated := deployment.DeepCopy()
		updated.Spec.Template.Spec.Containers[0].Image = targetDigestImage()
		return true, updated, nil
	})

	result := newTestK8sExecutor(client).Execute(context.Background(), k8sRequest())
	if result.Status != "success" {
		t.Fatalf("expected success, got %#v", result)
	}
	assertImageOnlyPatch(t, patchBody)
	if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 2 {
		t.Fatalf("executor must not change replicas: %#v", deployment.Spec.Replicas)
	}
	app := deployment.Spec.Template.Spec.Containers[0]
	if len(app.Env) != 1 || app.Resources.Limits.Cpu().String() != "500m" || app.ReadinessProbe == nil {
		t.Fatalf("executor must not change runtime config: %#v", app)
	}
	if deployment.Spec.Template.Spec.Containers[1].Image != "registry.example.com/sidecar@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("executor must not change sidecar image: %#v", deployment.Spec.Template.Spec.Containers[1])
	}
}

func TestK8sExecutorReturnsContainerNotFound(t *testing.T) {
	deployment := readyDeployment("order-api", "other")
	client := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}, deployment)
	result := newTestK8sExecutor(client).Execute(context.Background(), k8sRequest())
	if result.Status != "failed" || result.ErrorCode != "container_not_found" {
		t.Fatalf("expected container_not_found, got %#v", result)
	}
}

func TestK8sExecutorMapsDeploymentNotFound(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}})
	result := newTestK8sExecutor(client).Execute(context.Background(), k8sRequest())
	if result.Status != "failed" || result.ErrorCode != "deployment_not_found" {
		t.Fatalf("expected deployment_not_found, got %#v", result)
	}
}

func TestK8sExecutorMapsPermissionDenied(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}, readyDeployment("order-api", "app"))
	client.Fake.PrependReactor("patch", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "deployments"}, "order-api", errors.New("no patch"))
	})
	result := newTestK8sExecutor(client).Execute(context.Background(), k8sRequest())
	if result.Status != "failed" || result.ErrorCode != "permission_denied" {
		t.Fatalf("expected permission_denied, got %#v", result)
	}
}

func TestK8sExecutorRolloutTimeout(t *testing.T) {
	deployment := readyDeployment("order-api", "app")
	deployment.Status.UpdatedReplicas = 0
	deployment.Status.AvailableReplicas = 0
	client := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}, deployment)
	result := newTestK8sExecutor(client).Execute(context.Background(), Request{
		Target: domain.DeploymentTarget{TimeoutSeconds: 1, K8s: k8sTarget()},
		Version: domain.ServiceVersion{
			ArtifactURL: targetDigestImage(),
		},
	})
	if result.Status != "failed" || result.ErrorCode != "rollout_timeout" {
		t.Fatalf("expected rollout_timeout, got %#v", result)
	}
}

func newTestK8sExecutor(client kubernetes.Interface) K8s {
	return K8s{
		Credentials: credentialResolverByID{
			"cred_kube": {
				Credential: domain.Credential{ID: "cred_kube", Type: "kubeconfig", Enabled: true},
				Secret:     "apiVersion: v1\nkind: Config\n",
			},
		},
		Clusters: fakeK8sClusters{
			"k8s_test": {ID: "k8s_test", Name: "test", CredentialRef: "cred_kube", Enabled: true},
		},
		ClientFactory: func(context.Context, string) (kubernetes.Interface, error) {
			return client, nil
		},
		PollInterval: time.Millisecond,
	}
}

func k8sRequest() Request {
	return Request{
		Target: domain.DeploymentTarget{TimeoutSeconds: 3, K8s: k8sTarget()},
		Version: domain.ServiceVersion{
			ArtifactURL: targetDigestImage(),
		},
	}
}

func k8sTarget() *domain.K8sDeploymentTarget {
	return &domain.K8sDeploymentTarget{
		ClusterID:      "k8s_test",
		Namespace:      "default",
		DeploymentName: "order-api",
		ContainerName:  "app",
	}
}

func readyDeployment(name string, container string) *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Generation: 1},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name:  container,
				Image: "registry.example.com/order-api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			}}}},
		},
		Status: appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: 1, AvailableReplicas: 1},
	}
}

func targetDigestImage() string {
	return "registry.example.com/order-api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}

func assertImageOnlyPatch(t *testing.T, patch map[string]any) {
	t.Helper()
	raw, err := json.Marshal(patch)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, forbidden := range []string{"replicas", "resources", "env", "probe", "volume", "label", "annotation"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("patch must not contain %q: %s", forbidden, text)
		}
	}
	spec := patch["spec"].(map[string]any)
	template := spec["template"].(map[string]any)
	podSpec := template["spec"].(map[string]any)
	containers := podSpec["containers"].([]any)
	if len(containers) != 1 {
		t.Fatalf("expected one patched container, got %s", text)
	}
	container := containers[0].(map[string]any)
	if len(container) != 2 || container["name"] != "app" || container["image"] != targetDigestImage() {
		t.Fatalf("expected name/image-only container patch, got %s", text)
	}
}
