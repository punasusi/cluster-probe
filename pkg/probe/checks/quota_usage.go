package checks

import (
	"context"
	"fmt"

	"github.com/punasusi/cluster-probe/pkg/probe"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type QuotaUsage struct{}

func NewQuotaUsage() *QuotaUsage {
	return &QuotaUsage{}
}

func (c *QuotaUsage) Name() string {
	return "quota-usage"
}

func (c *QuotaUsage) Tier() int {
	return 3
}

func (c *QuotaUsage) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	quotas, err := client.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list resource quotas: %w", err)
	}

	if len(quotas.Items) == 0 {
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityOK,
			Message:	"No resource quotas defined in the cluster",
		})
		return result, nil
	}

	quotasNearLimit := 0
	quotasExceeded := 0

	for _, quota := range quotas.Items {
		for resourceName, hardLimit := range quota.Status.Hard {
			used := quota.Status.Used[resourceName]

			if hardLimit.IsZero() {
				continue
			}

			usedValue := used.Value()
			hardValue := hardLimit.Value()

			if resourceName == "cpu" || resourceName == "requests.cpu" || resourceName == "limits.cpu" {
				usedValue = used.MilliValue()
				hardValue = hardLimit.MilliValue()
			}

			if hardValue == 0 {
				continue
			}

			usagePercent := float64(usedValue) / float64(hardValue) * 100

			if usagePercent >= 100 {
				quotasExceeded++
				result.Results = append(result.Results, probe.Result{
					CheckName:	c.Name(),
					Severity:	probe.SeverityCritical,
					Message:	fmt.Sprintf("Quota %s/%s has reached limit for %s", quota.Namespace, quota.Name, resourceName),
					Details: []string{
						fmt.Sprintf("Used: %s", used.String()),
						fmt.Sprintf("Hard: %s", hardLimit.String()),
						fmt.Sprintf("Usage: %.1f%%", usagePercent),
					},
					Remediation:	"Increase quota limit or reduce resource usage in the namespace",
				})
			} else if usagePercent >= 80 {
				quotasNearLimit++
				result.Results = append(result.Results, probe.Result{
					CheckName:	c.Name(),
					Severity:	probe.SeverityWarning,
					Message:	fmt.Sprintf("Quota %s/%s is at %.0f%% for %s", quota.Namespace, quota.Name, usagePercent, resourceName),
					Details: []string{
						fmt.Sprintf("Used: %s", used.String()),
						fmt.Sprintf("Hard: %s", hardLimit.String()),
					},
					Remediation:	"Consider increasing quota limit before it's reached",
				})
			}
		}
	}

	limitRanges, err := client.CoreV1().LimitRanges("").List(ctx, metav1.ListOptions{})
	if err == nil {
		if len(limitRanges.Items) > 0 {
			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	probe.SeverityOK,
				Message:	fmt.Sprintf("%d limit ranges configured", len(limitRanges.Items)),
			})
		}
	}

	severity := probe.SeverityOK
	if quotasNearLimit > 0 {
		severity = probe.SeverityWarning
	}
	if quotasExceeded > 0 {
		severity = probe.SeverityCritical
	}

	result.Results = append(result.Results, probe.Result{
		CheckName:	c.Name(),
		Severity:	severity,
		Message:	fmt.Sprintf("Resource quotas: %d total, %d near limit, %d exceeded", len(quotas.Items), quotasNearLimit, quotasExceeded),
	})

	return result, nil
}
