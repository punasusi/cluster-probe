package checks

import (
	"context"
	"fmt"

	"github.com/punasusi/cluster-probe/pkg/probe"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ServiceEndpoints struct{}

func NewServiceEndpoints() *ServiceEndpoints {
	return &ServiceEndpoints{}
}

func (c *ServiceEndpoints) Name() string {
	return "service-endpoints"
}

func (c *ServiceEndpoints) Tier() int {
	return 4
}

func (c *ServiceEndpoints) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	services, err := client.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	endpoints, err := client.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list endpoints: %w", err)
	}

	endpointMap := make(map[string]*corev1.Endpoints)
	for i := range endpoints.Items {
		ep := &endpoints.Items[i]
		key := fmt.Sprintf("%s/%s", ep.Namespace, ep.Name)
		endpointMap[key] = ep
	}

	withEndpoints := 0
	withoutEndpoints := 0
	externalName := 0
	headless := 0

	for _, svc := range services.Items {

		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			externalName++
			continue
		}

		if svc.Spec.ClusterIP == "None" {
			headless++
			continue
		}

		if svc.Namespace == "default" && svc.Name == "kubernetes" {
			continue
		}

		key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
		ep := endpointMap[key]

		hasEndpoints := false
		if ep != nil {
			for _, subset := range ep.Subsets {
				if len(subset.Addresses) > 0 {
					hasEndpoints = true
					break
				}
			}
		}

		if hasEndpoints {
			withEndpoints++
		} else {
			withoutEndpoints++

			severity := probe.SeverityWarning
			if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
				severity = probe.SeverityCritical
			}

			details := []string{
				fmt.Sprintf("Type: %s", svc.Spec.Type),
			}
			if len(svc.Spec.Selector) > 0 {
				details = append(details, fmt.Sprintf("Selector: %v", svc.Spec.Selector))
			} else {
				details = append(details, "No selector defined (manual endpoints required)")
			}

			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	severity,
				Message:	fmt.Sprintf("Service %s/%s has no endpoints", svc.Namespace, svc.Name),
				Details:	details,
				Remediation:	"Check that pods matching the service selector exist and are ready",
			})
		}
	}

	severity := probe.SeverityOK
	if withoutEndpoints > 0 {
		severity = probe.SeverityWarning
	}

	result.Results = append(result.Results, probe.Result{
		CheckName:	c.Name(),
		Severity:	severity,
		Message:	fmt.Sprintf("Services: %d with endpoints, %d without", withEndpoints, withoutEndpoints),
		Details: []string{
			fmt.Sprintf("Total services: %d", len(services.Items)),
			fmt.Sprintf("ExternalName: %d", externalName),
			fmt.Sprintf("Headless: %d", headless),
		},
	})

	return result, nil
}
