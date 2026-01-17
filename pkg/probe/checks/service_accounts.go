package checks

import (
	"context"
	"fmt"

	"github.com/punasusi/cluster-probe/pkg/probe"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ServiceAccounts struct{}

func NewServiceAccounts() *ServiceAccounts {
	return &ServiceAccounts{}
}

func (c *ServiceAccounts) Name() string {
	return "service-accounts"
}

func (c *ServiceAccounts) Tier() int {
	return 5
}

func (c *ServiceAccounts) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	serviceAccounts, err := client.CoreV1().ServiceAccounts("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list service accounts: %w", err)
	}

	pods, err := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	saUsage := make(map[string]int)
	defaultSAUsage := 0

	for _, pod := range pods.Items {
		key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Spec.ServiceAccountName)
		saUsage[key]++

		if pod.Spec.ServiceAccountName == "default" && pod.Namespace != "kube-system" {
			defaultSAUsage++
		}
	}

	stats := struct {
		total			int
		autoMountEnabled	int
		autoMountDisabled	int
		unusedSAs		int
		defaultSAPods		int
	}{}

	stats.total = len(serviceAccounts.Items)
	stats.defaultSAPods = defaultSAUsage

	for _, sa := range serviceAccounts.Items {

		if sa.AutomountServiceAccountToken == nil || *sa.AutomountServiceAccountToken {
			stats.autoMountEnabled++
		} else {
			stats.autoMountDisabled++
		}

		if sa.Name != "default" && sa.Namespace != "kube-system" {
			key := fmt.Sprintf("%s/%s", sa.Namespace, sa.Name)
			if saUsage[key] == 0 {
				stats.unusedSAs++
			}
		}
	}

	if stats.defaultSAPods > 10 {
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityWarning,
			Message:	fmt.Sprintf("%d pods are using the default service account", stats.defaultSAPods),
			Details: []string{
				"Default service accounts should typically have minimal permissions",
				"Using dedicated service accounts provides better audit trails and RBAC control",
			},
			Remediation:	"Create dedicated service accounts for workloads",
		})
	}

	for _, sa := range serviceAccounts.Items {
		if sa.Namespace == "kube-system" {
			continue
		}

		if len(sa.Secrets) > 2 {
			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	probe.SeverityWarning,
				Message:	fmt.Sprintf("ServiceAccount %s/%s has %d secrets attached", sa.Namespace, sa.Name, len(sa.Secrets)),
				Details: []string{
					"Multiple secrets may indicate unused tokens or complex configurations",
				},
				Remediation:	"Review attached secrets and remove unused ones",
			})
		}
	}

	severity := probe.SeverityOK
	if stats.defaultSAPods > 10 {
		severity = probe.SeverityWarning
	}

	result.Results = append(result.Results, probe.Result{
		CheckName:	c.Name(),
		Severity:	severity,
		Message:	"Service account summary",
		Details: []string{
			fmt.Sprintf("Total service accounts: %d", stats.total),
			fmt.Sprintf("Auto-mount token enabled: %d", stats.autoMountEnabled),
			fmt.Sprintf("Auto-mount token disabled: %d", stats.autoMountDisabled),
			fmt.Sprintf("Pods using default SA: %d", stats.defaultSAPods),
			fmt.Sprintf("Unused service accounts: %d", stats.unusedSAs),
		},
	})

	return result, nil
}
