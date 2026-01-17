package checks

import (
	"context"
	"fmt"

	"github.com/punasusi/cluster-probe/pkg/probe"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type NetworkPolicies struct{}

func NewNetworkPolicies() *NetworkPolicies {
	return &NetworkPolicies{}
}

func (c *NetworkPolicies) Name() string {
	return "network-policies"
}

func (c *NetworkPolicies) Tier() int {
	return 4
}

func (c *NetworkPolicies) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	policies, err := client.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list network policies: %w", err)
	}

	namespaces, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	policyPerNS := make(map[string]int)
	for _, policy := range policies.Items {
		policyPerNS[policy.Namespace]++
	}

	nsWithPolicies := 0
	nsWithoutPolicies := 0
	systemNS := 0

	for _, ns := range namespaces.Items {

		if ns.Name == "kube-system" || ns.Name == "kube-public" || ns.Name == "kube-node-lease" {
			systemNS++
			continue
		}

		if policyPerNS[ns.Name] > 0 {
			nsWithPolicies++
		} else {
			nsWithoutPolicies++
		}
	}

	if len(policies.Items) == 0 {
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityWarning,
			Message:	"No network policies defined in the cluster",
			Details: []string{
				"Network policies provide pod-level firewall rules",
				"Consider implementing network policies for security isolation",
			},
			Remediation:	"Define network policies to restrict pod-to-pod traffic",
		})
	} else {

		hasDefaultDeny := false
		for _, policy := range policies.Items {

			if len(policy.Spec.PodSelector.MatchLabels) == 0 {
				if len(policy.Spec.Ingress) == 0 || len(policy.Spec.Egress) == 0 {
					hasDefaultDeny = true
					break
				}
			}
		}

		if !hasDefaultDeny && nsWithPolicies > 0 {
			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	probe.SeverityOK,
				Message:	fmt.Sprintf("%d network policies found across %d namespaces", len(policies.Items), nsWithPolicies),
				Details: []string{
					fmt.Sprintf("Namespaces with policies: %d", nsWithPolicies),
					fmt.Sprintf("Namespaces without policies: %d", nsWithoutPolicies),
				},
			})
		}
	}

	severity := probe.SeverityOK
	if len(policies.Items) == 0 {
		severity = probe.SeverityWarning
	}

	result.Results = append(result.Results, probe.Result{
		CheckName:	c.Name(),
		Severity:	severity,
		Message:	fmt.Sprintf("Network policies: %d total", len(policies.Items)),
		Details: []string{
			fmt.Sprintf("User namespaces: %d", len(namespaces.Items)-systemNS),
		},
	})

	return result, nil
}
