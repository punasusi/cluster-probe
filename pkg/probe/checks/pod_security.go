package checks

import (
	"context"
	"fmt"

	"github.com/punasusi/cluster-probe/pkg/probe"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type PodSecurity struct{}

func NewPodSecurity() *PodSecurity {
	return &PodSecurity{}
}

func (c *PodSecurity) Name() string {
	return "pod-security"
}

func (c *PodSecurity) Tier() int {
	return 5
}

func (c *PodSecurity) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
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
		privileged		int
		runAsRoot		int
		hostNetwork		int
		hostPID			int
		hostIPC			int
		noSecurityContext	int
		addedCapabilities	int
	}{}

	stats.total = len(pods.Items)

	for _, pod := range pods.Items {

		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		isSystem := pod.Namespace == "kube-system"

		podSecurityContext := pod.Spec.SecurityContext

		if pod.Spec.HostNetwork {
			stats.hostNetwork++
			if !isSystem {
				result.Results = append(result.Results, probe.Result{
					CheckName:	c.Name(),
					Severity:	probe.SeverityWarning,
					Message:	fmt.Sprintf("Pod %s/%s uses host network", pod.Namespace, pod.Name),
					Remediation:	"Review if host network access is necessary",
				})
			}
		}

		if pod.Spec.HostPID {
			stats.hostPID++
			if !isSystem {
				result.Results = append(result.Results, probe.Result{
					CheckName:	c.Name(),
					Severity:	probe.SeverityWarning,
					Message:	fmt.Sprintf("Pod %s/%s uses host PID namespace", pod.Namespace, pod.Name),
					Remediation:	"Review if host PID access is necessary",
				})
			}
		}

		if pod.Spec.HostIPC {
			stats.hostIPC++
			if !isSystem {
				result.Results = append(result.Results, probe.Result{
					CheckName:	c.Name(),
					Severity:	probe.SeverityWarning,
					Message:	fmt.Sprintf("Pod %s/%s uses host IPC namespace", pod.Namespace, pod.Name),
					Remediation:	"Review if host IPC access is necessary",
				})
			}
		}

		allContainers := append(pod.Spec.Containers, pod.Spec.InitContainers...)
		for _, container := range allContainers {
			secCtx := container.SecurityContext

			if secCtx != nil && secCtx.Privileged != nil && *secCtx.Privileged {
				stats.privileged++
				if !isSystem {
					result.Results = append(result.Results, probe.Result{
						CheckName:	c.Name(),
						Severity:	probe.SeverityWarning,
						Message:	fmt.Sprintf("Container %s in pod %s/%s is privileged", container.Name, pod.Namespace, pod.Name),
						Remediation:	"Review if privileged mode is necessary; consider specific capabilities instead",
					})
				}
			}

			runsAsRoot := false
			if secCtx != nil && secCtx.RunAsUser != nil && *secCtx.RunAsUser == 0 {
				runsAsRoot = true
			} else if secCtx == nil || secCtx.RunAsUser == nil {

				if podSecurityContext != nil && podSecurityContext.RunAsUser != nil && *podSecurityContext.RunAsUser == 0 {
					runsAsRoot = true
				} else if (secCtx == nil || secCtx.RunAsNonRoot == nil || !*secCtx.RunAsNonRoot) &&
					(podSecurityContext == nil || podSecurityContext.RunAsNonRoot == nil || !*podSecurityContext.RunAsNonRoot) {

				}
			}

			if runsAsRoot {
				stats.runAsRoot++
				if !isSystem {
					result.Results = append(result.Results, probe.Result{
						CheckName:	c.Name(),
						Severity:	probe.SeverityWarning,
						Message:	fmt.Sprintf("Container %s in pod %s/%s runs as root", container.Name, pod.Namespace, pod.Name),
						Remediation:	"Consider running as non-root user with runAsNonRoot: true",
					})
				}
			}

			if secCtx == nil && podSecurityContext == nil {
				stats.noSecurityContext++
			}

			if secCtx != nil && secCtx.Capabilities != nil && len(secCtx.Capabilities.Add) > 0 {
				stats.addedCapabilities++

				if !isSystem {
					for _, cap := range secCtx.Capabilities.Add {
						if isDangerousCapability(string(cap)) {
							result.Results = append(result.Results, probe.Result{
								CheckName:	c.Name(),
								Severity:	probe.SeverityWarning,
								Message:	fmt.Sprintf("Container %s in pod %s/%s has dangerous capability %s", container.Name, pod.Namespace, pod.Name, cap),
								Remediation:	"Review if this capability is necessary",
							})
						}
					}
				}
			}
		}
	}

	severity := probe.SeverityOK
	if stats.privileged > 0 || stats.runAsRoot > 0 {
		severity = probe.SeverityWarning
	}

	result.Results = append(result.Results, probe.Result{
		CheckName:	c.Name(),
		Severity:	severity,
		Message:	"Pod security summary",
		Details: []string{
			fmt.Sprintf("Total pods: %d", stats.total),
			fmt.Sprintf("Privileged containers: %d", stats.privileged),
			fmt.Sprintf("Running as root: %d", stats.runAsRoot),
			fmt.Sprintf("Host network: %d", stats.hostNetwork),
			fmt.Sprintf("Host PID: %d", stats.hostPID),
			fmt.Sprintf("No security context: %d", stats.noSecurityContext),
		},
	})

	return result, nil
}

func isDangerousCapability(cap string) bool {
	dangerous := map[string]bool{
		"SYS_ADMIN":	true,
		"NET_ADMIN":	true,
		"SYS_PTRACE":	true,
		"SYS_RAWIO":	true,
		"SYS_MODULE":	true,
		"DAC_OVERRIDE":	true,
		"ALL":		true,
	}
	return dangerous[cap]
}
