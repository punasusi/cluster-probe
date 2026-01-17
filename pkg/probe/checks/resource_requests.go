package checks

import (
	"context"
	"fmt"

	"github.com/punasusi/cluster-probe/pkg/probe"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ResourceRequests struct{}

func NewResourceRequests() *ResourceRequests {
	return &ResourceRequests{}
}

func (c *ResourceRequests) Name() string {
	return "resource-requests"
}

func (c *ResourceRequests) Tier() int {
	return 3
}

func (c *ResourceRequests) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	pods, err := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	noRequests := 0
	noLimits := 0
	total := 0

	nsWithIssues := make(map[string]int)

	for _, pod := range pods.Items {

		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		isSystemNS := pod.Namespace == "kube-system" || pod.Namespace == "kube-public" || pod.Namespace == "kube-node-lease"

		total++

		for _, container := range pod.Spec.Containers {
			hasRequests := container.Resources.Requests != nil &&
				(container.Resources.Requests.Cpu() != nil || container.Resources.Requests.Memory() != nil)
			hasLimits := container.Resources.Limits != nil &&
				(container.Resources.Limits.Cpu() != nil || container.Resources.Limits.Memory() != nil)

			if !hasRequests {
				noRequests++
				if !isSystemNS {
					nsWithIssues[pod.Namespace]++
				}
			}

			if !hasLimits {
				noLimits++
			}
		}
	}

	for ns, count := range nsWithIssues {
		if count > 3 {
			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	probe.SeverityWarning,
				Message:	fmt.Sprintf("Namespace %s has %d containers without resource requests", ns, count),
				Remediation: "Set resource requests for better scheduling: " +
					"resources: { requests: { cpu: '100m', memory: '128Mi' } }",
			})
		}
	}

	severity := probe.SeverityOK
	if noRequests > total/2 {
		severity = probe.SeverityWarning
	}

	result.Results = append(result.Results, probe.Result{
		CheckName:	c.Name(),
		Severity:	severity,
		Message:	fmt.Sprintf("Resource requests: %d/%d containers have requests defined", total-noRequests, total),
		Details: []string{
			fmt.Sprintf("Without requests: %d", noRequests),
			fmt.Sprintf("Without limits: %d", noLimits),
		},
	})

	return result, nil
}
