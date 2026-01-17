package checks

import (
	"context"
	"fmt"

	"github.com/punasusi/cluster-probe/pkg/probe"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type NodeCapacity struct{}

func NewNodeCapacity() *NodeCapacity {
	return &NodeCapacity{}
}

func (c *NodeCapacity) Name() string {
	return "node-capacity"
}

func (c *NodeCapacity) Tier() int {
	return 3
}

func (c *NodeCapacity) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	pods, err := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	nodeUsage := make(map[string]*resourceUsage)
	for _, node := range nodes.Items {
		nodeUsage[node.Name] = &resourceUsage{
			cpuCapacity:	node.Status.Allocatable.Cpu().MilliValue(),
			memoryCapacity:	node.Status.Allocatable.Memory().Value(),
			podCapacity:	node.Status.Allocatable.Pods().Value(),
		}
	}

	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		usage, ok := nodeUsage[pod.Spec.NodeName]
		if !ok {
			continue
		}

		usage.podCount++
		for _, container := range pod.Spec.Containers {
			if container.Resources.Requests != nil {
				usage.cpuRequested += container.Resources.Requests.Cpu().MilliValue()
				usage.memoryRequested += container.Resources.Requests.Memory().Value()
			}
		}
	}

	for _, node := range nodes.Items {
		usage := nodeUsage[node.Name]

		cpuPercent := float64(0)
		memPercent := float64(0)
		podPercent := float64(0)

		if usage.cpuCapacity > 0 {
			cpuPercent = float64(usage.cpuRequested) / float64(usage.cpuCapacity) * 100
		}
		if usage.memoryCapacity > 0 {
			memPercent = float64(usage.memoryRequested) / float64(usage.memoryCapacity) * 100
		}
		if usage.podCapacity > 0 {
			podPercent = float64(usage.podCount) / float64(usage.podCapacity) * 100
		}

		if cpuPercent > 90 || memPercent > 90 || podPercent > 90 {
			severity := probe.SeverityWarning
			if cpuPercent > 95 || memPercent > 95 || podPercent > 95 {
				severity = probe.SeverityCritical
			}

			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	severity,
				Message:	fmt.Sprintf("Node %s has high resource allocation", node.Name),
				Details: []string{
					fmt.Sprintf("CPU: %.1f%% allocated (%dm/%dm)", cpuPercent, usage.cpuRequested, usage.cpuCapacity),
					fmt.Sprintf("Memory: %.1f%% allocated (%s/%s)", memPercent, formatBytes(usage.memoryRequested), formatBytes(usage.memoryCapacity)),
					fmt.Sprintf("Pods: %.1f%% (%d/%d)", podPercent, usage.podCount, usage.podCapacity),
				},
				Remediation:	"Consider adding more nodes or reducing workload on this node",
			})
		}

		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityOK,
			Message:	fmt.Sprintf("Node %s: CPU %.0f%%, Memory %.0f%%, Pods %d/%d", node.Name, cpuPercent, memPercent, usage.podCount, usage.podCapacity),
		})
	}

	return result, nil
}

type resourceUsage struct {
	cpuCapacity	int64
	cpuRequested	int64
	memoryCapacity	int64
	memoryRequested	int64
	podCapacity	int64
	podCount	int64
}

func formatBytes(bytes int64) string {
	const (
		KB	= 1024
		MB	= KB * 1024
		GB	= MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fGi", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1fMi", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1fKi", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
