package checks

import (
	"context"
	"fmt"

	"github.com/punasusi/cluster-probe/pkg/probe"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type DeploymentStatus struct{}

func NewDeploymentStatus() *DeploymentStatus {
	return &DeploymentStatus{}
}

func (c *DeploymentStatus) Name() string {
	return "deployment-status"
}

func (c *DeploymentStatus) Tier() int {
	return 2
}

func (c *DeploymentStatus) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	deployments, err := client.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	healthy := 0
	unhealthy := 0
	progressing := 0

	for _, deploy := range deployments.Items {
		desired := int32(1)
		if deploy.Spec.Replicas != nil {
			desired = *deploy.Spec.Replicas
		}

		available := deploy.Status.AvailableReplicas
		ready := deploy.Status.ReadyReplicas
		updated := deploy.Status.UpdatedReplicas

		var progressingCond, availableCond *appsv1.DeploymentCondition
		for i := range deploy.Status.Conditions {
			cond := &deploy.Status.Conditions[i]
			switch cond.Type {
			case appsv1.DeploymentProgressing:
				progressingCond = cond
			case appsv1.DeploymentAvailable:
				availableCond = cond
			}
		}

		if desired == 0 {

			healthy++
			continue
		}

		if available < desired {
			unhealthy++

			severity := probe.SeverityWarning
			details := []string{
				fmt.Sprintf("Desired: %d, Available: %d, Ready: %d", desired, available, ready),
			}

			if progressingCond != nil {
				details = append(details, fmt.Sprintf("Progressing: %s - %s", progressingCond.Reason, progressingCond.Message))
				if progressingCond.Reason == "ProgressDeadlineExceeded" {
					severity = probe.SeverityCritical
				}
			}

			if availableCond != nil && availableCond.Status == "False" {
				details = append(details, fmt.Sprintf("Available: %s - %s", availableCond.Reason, availableCond.Message))
			}

			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	severity,
				Message:	fmt.Sprintf("Deployment %s/%s has insufficient replicas", deploy.Namespace, deploy.Name),
				Details:	details,
				Remediation:	fmt.Sprintf("Check pods: kubectl get pods -n %s -l app=%s", deploy.Namespace, deploy.Name),
			})
		} else if updated < desired {

			progressing++
			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	probe.SeverityWarning,
				Message:	fmt.Sprintf("Deployment %s/%s rollout in progress", deploy.Namespace, deploy.Name),
				Details: []string{
					fmt.Sprintf("Updated: %d/%d", updated, desired),
				},
				Remediation:	fmt.Sprintf("Monitor rollout: kubectl rollout status -n %s deployment/%s", deploy.Namespace, deploy.Name),
			})
		} else {
			healthy++
		}
	}

	severity := probe.SeverityOK
	if unhealthy > 0 {
		severity = probe.SeverityWarning
	}

	result.Results = append(result.Results, probe.Result{
		CheckName:	c.Name(),
		Severity:	severity,
		Message:	fmt.Sprintf("Deployments: %d healthy, %d unhealthy, %d progressing", healthy, unhealthy, progressing),
		Details: []string{
			fmt.Sprintf("Total deployments: %d", len(deployments.Items)),
		},
	})

	return result, nil
}
