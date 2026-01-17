package checks

import (
	"context"
	"fmt"

	"github.com/punasusi/cluster-probe/pkg/probe"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type NodeStatus struct{}

func NewNodeStatus() *NodeStatus {
	return &NodeStatus{}
}

func (c *NodeStatus) Name() string {
	return "node-status"
}

func (c *NodeStatus) Tier() int {
	return 1
}

func (c *NodeStatus) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	if len(nodes.Items) == 0 {
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityCritical,
			Message:	"No nodes found in cluster",
			Remediation:	"Check cluster connectivity and node registration",
		})
		return result, nil
	}

	readyCount := 0
	notReadyNodes := []string{}

	for _, node := range nodes.Items {
		ready := false
		var lastCondition *corev1.NodeCondition

		for i := range node.Status.Conditions {
			cond := &node.Status.Conditions[i]
			if cond.Type == corev1.NodeReady {
				lastCondition = cond
				if cond.Status == corev1.ConditionTrue {
					ready = true
					readyCount++
				}
				break
			}
		}

		if !ready {
			notReadyNodes = append(notReadyNodes, node.Name)
			details := []string{}
			if lastCondition != nil {
				details = append(details, fmt.Sprintf("Reason: %s", lastCondition.Reason))
				details = append(details, fmt.Sprintf("Message: %s", lastCondition.Message))
			}

			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	probe.SeverityCritical,
				Message:	fmt.Sprintf("Node %s is not Ready", node.Name),
				Details:	details,
				Remediation:	"Check node kubelet status with 'systemctl status kubelet' and review node logs",
			})
		}

		for _, cond := range node.Status.Conditions {
			switch cond.Type {
			case corev1.NodeMemoryPressure:
				if cond.Status == corev1.ConditionTrue {
					result.Results = append(result.Results, probe.Result{
						CheckName:	c.Name(),
						Severity:	probe.SeverityWarning,
						Message:	fmt.Sprintf("Node %s has MemoryPressure", node.Name),
						Details:	[]string{cond.Message},
						Remediation:	"Consider adding more memory or reducing workload on this node",
					})
				}
			case corev1.NodeDiskPressure:
				if cond.Status == corev1.ConditionTrue {
					result.Results = append(result.Results, probe.Result{
						CheckName:	c.Name(),
						Severity:	probe.SeverityWarning,
						Message:	fmt.Sprintf("Node %s has DiskPressure", node.Name),
						Details:	[]string{cond.Message},
						Remediation:	"Free up disk space or add more storage capacity",
					})
				}
			case corev1.NodePIDPressure:
				if cond.Status == corev1.ConditionTrue {
					result.Results = append(result.Results, probe.Result{
						CheckName:	c.Name(),
						Severity:	probe.SeverityWarning,
						Message:	fmt.Sprintf("Node %s has PIDPressure", node.Name),
						Details:	[]string{cond.Message},
						Remediation:	"Check for runaway processes and consider increasing PID limits",
					})
				}
			case corev1.NodeNetworkUnavailable:
				if cond.Status == corev1.ConditionTrue {
					result.Results = append(result.Results, probe.Result{
						CheckName:	c.Name(),
						Severity:	probe.SeverityCritical,
						Message:	fmt.Sprintf("Node %s has NetworkUnavailable", node.Name),
						Details:	[]string{cond.Message},
						Remediation:	"Check CNI plugin status and network configuration",
					})
				}
			}
		}
	}

	if len(notReadyNodes) == 0 {
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityOK,
			Message:	fmt.Sprintf("All %d nodes are Ready", readyCount),
		})
	}

	return result, nil
}
