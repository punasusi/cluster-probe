package checks

import (
	"context"
	"fmt"

	"github.com/punasusi/cluster-probe/pkg/probe"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type SecretsUsage struct{}

func NewSecretsUsage() *SecretsUsage {
	return &SecretsUsage{}
}

func (c *SecretsUsage) Name() string {
	return "secrets-usage"
}

func (c *SecretsUsage) Tier() int {
	return 5
}

func (c *SecretsUsage) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	pods, err := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	stats := struct {
		total			int
		secretEnvVars		int
		secretVolumes		int
		autoMountToken		int
		noAutoMountToken	int
		envFromSecrets		int
	}{}

	stats.total = len(pods.Items)

	for _, pod := range pods.Items {

		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		if pod.Namespace == "kube-system" {
			continue
		}

		if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
			stats.autoMountToken++
		} else {
			stats.noAutoMountToken++
		}

		hasSecretVolume := false
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil {
				hasSecretVolume = true
				stats.secretVolumes++
				break
			}
		}

		allContainers := append(pod.Spec.Containers, pod.Spec.InitContainers...)
		podHasSecretEnv := false

		for _, container := range allContainers {

			for _, envFrom := range container.EnvFrom {
				if envFrom.SecretRef != nil {
					stats.envFromSecrets++
					result.Results = append(result.Results, probe.Result{
						CheckName:	c.Name(),
						Severity:	probe.SeverityWarning,
						Message:	fmt.Sprintf("Container %s in pod %s/%s uses envFrom with secret", container.Name, pod.Namespace, pod.Name),
						Details: []string{
							fmt.Sprintf("Secret: %s", envFrom.SecretRef.Name),
							"Exposing entire secrets as environment variables may leak sensitive data",
						},
						Remediation:	"Consider using volume mounts or specific env vars instead of envFrom",
					})
				}
			}

			for _, env := range container.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					podHasSecretEnv = true

					if !hasSecretVolume {
						stats.secretEnvVars++
					}
				}
			}
		}

		if podHasSecretEnv && !hasSecretVolume && stats.secretEnvVars <= 5 {

			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	probe.SeverityWarning,
				Message:	fmt.Sprintf("Pod %s/%s exposes secrets via environment variables", pod.Namespace, pod.Name),
				Details: []string{
					"Secrets in env vars may appear in logs, process listings, or crash dumps",
				},
				Remediation:	"Consider using secret volume mounts instead for sensitive data",
			})
		}
	}

	severity := probe.SeverityOK
	if stats.envFromSecrets > 0 {
		severity = probe.SeverityWarning
	}

	result.Results = append(result.Results, probe.Result{
		CheckName:	c.Name(),
		Severity:	severity,
		Message:	"Secrets usage summary",
		Details: []string{
			fmt.Sprintf("Pods with auto-mounted SA token: %d", stats.autoMountToken),
			fmt.Sprintf("Pods with disabled SA token mount: %d", stats.noAutoMountToken),
			fmt.Sprintf("Pods with secret volumes: %d", stats.secretVolumes),
			fmt.Sprintf("Pods with secrets in env vars only: %d", stats.secretEnvVars),
			fmt.Sprintf("EnvFrom with secrets: %d", stats.envFromSecrets),
		},
	})

	return result, nil
}
