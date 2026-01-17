package checks

import (
	"context"
	"fmt"

	"github.com/punasusi/cluster-probe/pkg/probe"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type PodStatus struct{}

func NewPodStatus() *PodStatus {
	return &PodStatus{}
}

func (c *PodStatus) Name() string {
	return "pod-status"
}

func (c *PodStatus) Tier() int {
	return 2
}

func (c *PodStatus) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
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
		total		int
		running		int
		pending		int
		failed		int
		succeeded	int
		crashLoop	int
		imagePull	int
		unknown		int
	}{}

	stats.total = len(pods.Items)

	for _, pod := range pods.Items {
		switch pod.Status.Phase {
		case corev1.PodRunning:
			stats.running++
		case corev1.PodPending:
			stats.pending++
			c.checkPendingPod(&pod, result)
		case corev1.PodFailed:
			stats.failed++
			c.checkFailedPod(&pod, result)
		case corev1.PodSucceeded:
			stats.succeeded++
		default:
			stats.unknown++
		}

		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil {
				switch cs.State.Waiting.Reason {
				case "CrashLoopBackOff":
					stats.crashLoop++
					if pod.Namespace != "kube-system" {
						result.Results = append(result.Results, probe.Result{
							CheckName:	c.Name(),
							Severity:	probe.SeverityWarning,
							Message:	fmt.Sprintf("Pod %s/%s is in CrashLoopBackOff", pod.Namespace, pod.Name),
							Details: []string{
								fmt.Sprintf("Container: %s", cs.Name),
								fmt.Sprintf("Restarts: %d", cs.RestartCount),
							},
							Remediation:	fmt.Sprintf("Check logs: kubectl logs -n %s %s -c %s --previous", pod.Namespace, pod.Name, cs.Name),
						})
					}
				case "ImagePullBackOff", "ErrImagePull":
					stats.imagePull++
					result.Results = append(result.Results, probe.Result{
						CheckName:	c.Name(),
						Severity:	probe.SeverityWarning,
						Message:	fmt.Sprintf("Pod %s/%s cannot pull image", pod.Namespace, pod.Name),
						Details: []string{
							fmt.Sprintf("Container: %s", cs.Name),
							fmt.Sprintf("Image: %s", cs.Image),
							fmt.Sprintf("Reason: %s", cs.State.Waiting.Reason),
						},
						Remediation:	"Verify image name, registry credentials, and network access",
					})
				}
			}
		}
	}

	severity := probe.SeverityOK
	if stats.crashLoop > 0 || stats.imagePull > 0 || stats.failed > 0 {
		severity = probe.SeverityWarning
	}

	result.Results = append(result.Results, probe.Result{
		CheckName:	c.Name(),
		Severity:	severity,
		Message:	fmt.Sprintf("Pod status: %d running, %d pending, %d failed, %d succeeded", stats.running, stats.pending, stats.failed, stats.succeeded),
		Details: []string{
			fmt.Sprintf("Total pods: %d", stats.total),
			fmt.Sprintf("CrashLoopBackOff: %d", stats.crashLoop),
			fmt.Sprintf("ImagePullBackOff: %d", stats.imagePull),
		},
	})

	return result, nil
}

func (c *PodStatus) checkPendingPod(pod *corev1.Pod, result *probe.CheckResult) {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	probe.SeverityWarning,
				Message:	fmt.Sprintf("Pod %s/%s is unschedulable", pod.Namespace, pod.Name),
				Details: []string{
					fmt.Sprintf("Reason: %s", cond.Reason),
					fmt.Sprintf("Message: %s", cond.Message),
				},
				Remediation:	"Check node resources, taints, and pod resource requests/tolerations",
			})
		}
	}
}

func (c *PodStatus) checkFailedPod(pod *corev1.Pod, result *probe.CheckResult) {

	if pod.Status.Reason == "Evicted" {
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityWarning,
			Message:	fmt.Sprintf("Pod %s/%s was evicted", pod.Namespace, pod.Name),
			Details: []string{
				fmt.Sprintf("Message: %s", pod.Status.Message),
			},
			Remediation:	"Check node resources and pod priority. Consider cleanup of evicted pods",
		})
	}
}
