package checks

import (
	"context"
	"fmt"

	"github.com/punasusi/cluster-probe/pkg/probe"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type CriticalPods struct{}

func NewCriticalPods() *CriticalPods {
	return &CriticalPods{}
}

func (c *CriticalPods) Name() string {
	return "critical-pods"
}

func (c *CriticalPods) Tier() int {
	return 1
}

func (c *CriticalPods) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	pods, err := client.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list kube-system pods: %w", err)
	}

	criticalPrefixes := []string{
		"kube-apiserver",
		"kube-controller-manager",
		"kube-scheduler",
		"etcd",
		"kube-proxy",
	}

	criticalIssues := 0
	warnings := 0

	for _, pod := range pods.Items {

		if pod.Status.Phase == corev1.PodSucceeded {
			continue
		}

		isCritical := false
		for _, prefix := range criticalPrefixes {
			if len(pod.Name) >= len(prefix) && pod.Name[:len(prefix)] == prefix {
				isCritical = true
				break
			}
		}

		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil {
				reason := cs.State.Waiting.Reason
				if reason == "CrashLoopBackOff" || reason == "ImagePullBackOff" || reason == "ErrImagePull" {
					severity := probe.SeverityWarning
					if isCritical {
						severity = probe.SeverityCritical
						criticalIssues++
					} else {
						warnings++
					}

					result.Results = append(result.Results, probe.Result{
						CheckName:	c.Name(),
						Severity:	severity,
						Message:	fmt.Sprintf("Pod %s container %s is in %s", pod.Name, cs.Name, reason),
						Details: []string{
							fmt.Sprintf("Namespace: %s", pod.Namespace),
							fmt.Sprintf("Restarts: %d", cs.RestartCount),
						},
						Remediation:	c.getRemediation(reason),
					})
				}
			}

			if cs.RestartCount > 5 {
				severity := probe.SeverityWarning
				if isCritical {
					severity = probe.SeverityCritical
					criticalIssues++
				} else {
					warnings++
				}

				result.Results = append(result.Results, probe.Result{
					CheckName:	c.Name(),
					Severity:	severity,
					Message:	fmt.Sprintf("Pod %s container %s has high restart count", pod.Name, cs.Name),
					Details: []string{
						fmt.Sprintf("Restart count: %d", cs.RestartCount),
					},
					Remediation:	"Check container logs for crash reasons: kubectl logs -n kube-system " + pod.Name,
				})
			}
		}

		if pod.Status.Phase == corev1.PodFailed {
			severity := probe.SeverityWarning
			if isCritical {
				severity = probe.SeverityCritical
				criticalIssues++
			} else {
				warnings++
			}

			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	severity,
				Message:	fmt.Sprintf("Pod %s is in Failed state", pod.Name),
				Details: []string{
					fmt.Sprintf("Reason: %s", pod.Status.Reason),
					fmt.Sprintf("Message: %s", pod.Status.Message),
				},
				Remediation:	"Check pod events and logs for failure reason",
			})
		}

		if pod.Status.Phase == corev1.PodPending {

			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
					severity := probe.SeverityWarning
					if isCritical {
						severity = probe.SeverityCritical
						criticalIssues++
					} else {
						warnings++
					}

					result.Results = append(result.Results, probe.Result{
						CheckName:	c.Name(),
						Severity:	severity,
						Message:	fmt.Sprintf("Pod %s is pending and unschedulable", pod.Name),
						Details: []string{
							fmt.Sprintf("Reason: %s", cond.Reason),
							fmt.Sprintf("Message: %s", cond.Message),
						},
						Remediation:	"Check node resources and pod resource requests",
					})
				}
			}
		}
	}

	if criticalIssues == 0 && warnings == 0 {
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityOK,
			Message:	"All critical system pods are healthy",
		})
	}

	return result, nil
}

func (c *CriticalPods) getRemediation(reason string) string {
	switch reason {
	case "CrashLoopBackOff":
		return "Container is crashing repeatedly. Check logs: kubectl logs -n kube-system <pod-name> --previous"
	case "ImagePullBackOff", "ErrImagePull":
		return "Cannot pull container image. Check image name, registry access, and network connectivity"
	default:
		return "Check pod events: kubectl describe pod -n kube-system <pod-name>"
	}
}
