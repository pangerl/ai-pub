package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"ai-pub/internal/domain"
	"ai-pub/internal/repository"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var k8sDigestPattern = regexp.MustCompile(`^[^@]+@sha256:[0-9a-fA-F]{64}$`)

type K8sClusterResolver interface {
	GetK8sCluster(ctx context.Context, id string) (domain.K8sCluster, error)
}

type K8sClientFactory func(ctx context.Context, kubeconfig string) (kubernetes.Interface, error)

type K8s struct {
	Credentials   CredentialResolver
	Clusters      K8sClusterResolver
	ClientFactory K8sClientFactory
	PollInterval  time.Duration
}

func (k K8s) Execute(ctx context.Context, req Request) repository.ServerResult {
	start := time.Now()
	target := req.Target.K8s
	if target == nil {
		return failedResult(start, "executor_error", "k8s deployment target config is required", nil)
	}
	if !k8sDigestPattern.MatchString(req.Version.ArtifactURL) {
		return failedResult(start, "image_invalid", "artifact_url must be an immutable digest image", nil)
	}
	if k.Clusters == nil || k.Credentials == nil {
		return failedResult(start, "executor_error", "k8s executor is not configured", nil)
	}
	client, err := k.clientForTarget(ctx, *target)
	if err != nil {
		return failedResult(start, "cluster_not_available", "k8s cluster is not available", nil)
	}
	timeout := time.Duration(req.Target.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if _, err := client.CoreV1().Namespaces().Get(execCtx, target.Namespace, metav1.GetOptions{}); err != nil {
		return k8sErrorResult(start, err, "namespace_not_found", "namespace is not available")
	}
	deployment, err := client.AppsV1().Deployments(target.Namespace).Get(execCtx, target.DeploymentName, metav1.GetOptions{})
	if err != nil {
		return k8sErrorResult(start, err, "deployment_not_found", "deployment is not available")
	}
	if !deploymentHasContainer(deployment, target.ContainerName) {
		return failedResult(start, "container_not_found", "container is not found in deployment", nil)
	}
	patch, err := imagePatch(target.ContainerName, req.Version.ArtifactURL)
	if err != nil {
		return failedResult(start, "executor_error", "failed to build deployment patch", nil)
	}
	if _, err := client.AppsV1().Deployments(target.Namespace).Patch(execCtx, target.DeploymentName, types.StrategicMergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return k8sErrorResult(start, err, "executor_error", "failed to patch deployment image")
	}
	if err := k.waitRollout(execCtx, client, *target); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			return failedResult(start, "rollout_timeout", "deployment rollout timed out", nil)
		}
		return failedResult(start, "rollout_failed", err.Error(), nil)
	}
	code := 0
	return repository.ServerResult{
		Status:     "success",
		ExitCode:   &code,
		DurationMS: int(time.Since(start).Milliseconds()),
		LogOutput:  fmt.Sprintf("updated deployment/%s container %s image", target.DeploymentName, target.ContainerName),
	}
}

func (k K8s) CheckDeployment(ctx context.Context, target domain.K8sDeploymentTarget) (string, string, error) {
	client, err := k.clientForTarget(ctx, target)
	if err != nil {
		return "cluster_not_available", "K8s 集群不可用", nil
	}
	if _, err := client.CoreV1().Namespaces().Get(ctx, target.Namespace, metav1.GetOptions{}); err != nil {
		result := k8sErrorResult(time.Now(), err, "namespace_not_found", "K8s namespace 不存在")
		return result.ErrorCode, result.ErrorMessage, nil
	}
	deployment, err := client.AppsV1().Deployments(target.Namespace).Get(ctx, target.DeploymentName, metav1.GetOptions{})
	if err != nil {
		result := k8sErrorResult(time.Now(), err, "deployment_not_found", "K8s Deployment 不存在")
		return result.ErrorCode, result.ErrorMessage, nil
	}
	if !deploymentHasContainer(deployment, target.ContainerName) {
		return "container_not_found", "K8s Deployment 中不存在指定容器", nil
	}
	return "", "", nil
}

func (k K8s) clientForTarget(ctx context.Context, target domain.K8sDeploymentTarget) (kubernetes.Interface, error) {
	if k.Clusters == nil || k.Credentials == nil {
		return nil, fmt.Errorf("k8s executor is not configured")
	}
	cluster, err := k.Clusters.GetK8sCluster(ctx, target.ClusterID)
	if err != nil || !cluster.Enabled {
		return nil, fmt.Errorf("k8s cluster is not available")
	}
	secret, err := k.Credentials.Secret(ctx, cluster.CredentialRef)
	if err != nil || secret.Credential.Type != "kubeconfig" || !secret.Credential.Enabled {
		return nil, fmt.Errorf("kubeconfig credential is not available")
	}
	return k.client(ctx, secret.Secret)
}

func (k K8s) client(ctx context.Context, kubeconfig string) (kubernetes.Interface, error) {
	if k.ClientFactory != nil {
		return k.ClientFactory(ctx, kubeconfig)
	}
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

func (k K8s) waitRollout(ctx context.Context, client kubernetes.Interface, target domain.K8sDeploymentTarget) error {
	interval := k.PollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		deployment, err := client.AppsV1().Deployments(target.Namespace).Get(ctx, target.DeploymentName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if rolloutFailed(deployment) {
			return fmt.Errorf("deployment rollout failed")
		}
		if rolloutComplete(deployment) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func deploymentHasContainer(deployment *appsv1.Deployment, containerName string) bool {
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == containerName {
			return true
		}
	}
	return false
}

func imagePatch(containerName string, image string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []map[string]string{{
						"name":  containerName,
						"image": image,
					}},
				},
			},
		},
	})
}

func rolloutComplete(deployment *appsv1.Deployment) bool {
	replicas := int32(1)
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}
	return deployment.Status.ObservedGeneration >= deployment.Generation &&
		deployment.Status.UpdatedReplicas >= replicas &&
		deployment.Status.AvailableReplicas >= replicas &&
		deployment.Status.UnavailableReplicas == 0
}

func rolloutFailed(deployment *appsv1.Deployment) bool {
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing && condition.Status == corev1.ConditionFalse {
			return true
		}
	}
	return false
}

func k8sErrorResult(start time.Time, err error, notFoundCode string, message string) repository.ServerResult {
	if apierrors.IsNotFound(err) {
		return failedResult(start, notFoundCode, message, nil)
	}
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		return failedResult(start, "permission_denied", "kubernetes api permission denied", nil)
	}
	return failedResult(start, "executor_error", message, nil)
}
