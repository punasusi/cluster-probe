package checks

import (
	"context"
	"fmt"

	"github.com/punasusi/cluster-probe/pkg/probe"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type IngressStatus struct{}

func NewIngressStatus() *IngressStatus {
	return &IngressStatus{}
}

func (c *IngressStatus) Name() string {
	return "ingress-status"
}

func (c *IngressStatus) Tier() int {
	return 4
}

func (c *IngressStatus) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	ingresses, err := client.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list ingresses: %w", err)
	}

	if len(ingresses.Items) == 0 {
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityOK,
			Message:	"No ingresses defined in the cluster",
		})
		return result, nil
	}

	withAddress := 0
	withoutAddress := 0
	withTLS := 0

	for _, ing := range ingresses.Items {

		hasAddress := len(ing.Status.LoadBalancer.Ingress) > 0

		if hasAddress {
			withAddress++
		} else {
			withoutAddress++
			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	probe.SeverityWarning,
				Message:	fmt.Sprintf("Ingress %s/%s has no address assigned", ing.Namespace, ing.Name),
				Details: []string{
					fmt.Sprintf("Class: %s", c.getIngressClass(&ing)),
					fmt.Sprintf("Hosts: %v", c.getHosts(&ing)),
				},
				Remediation:	"Check ingress controller status and logs",
			})
		}

		if len(ing.Spec.TLS) > 0 {
			withTLS++
			for _, tls := range ing.Spec.TLS {
				if tls.SecretName == "" {
					result.Results = append(result.Results, probe.Result{
						CheckName:	c.Name(),
						Severity:	probe.SeverityWarning,
						Message:	fmt.Sprintf("Ingress %s/%s has TLS without secret", ing.Namespace, ing.Name),
						Details: []string{
							fmt.Sprintf("Hosts: %v", tls.Hosts),
						},
						Remediation:	"Configure TLS secret for the ingress",
					})
				}
			}
		}

		if ing.Spec.DefaultBackend == nil && len(ing.Spec.Rules) == 0 {
			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	probe.SeverityWarning,
				Message:	fmt.Sprintf("Ingress %s/%s has no rules or default backend", ing.Namespace, ing.Name),
				Remediation:	"Configure ingress rules or default backend",
			})
		}
	}

	ingressClasses, err := client.NetworkingV1().IngressClasses().List(ctx, metav1.ListOptions{})
	if err == nil && len(ingressClasses.Items) > 0 {
		classes := make([]string, 0, len(ingressClasses.Items))
		for _, ic := range ingressClasses.Items {
			classes = append(classes, ic.Name)
		}
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityOK,
			Message:	fmt.Sprintf("%d ingress classes available", len(classes)),
			Details:	classes,
		})
	}

	severity := probe.SeverityOK
	if withoutAddress > 0 {
		severity = probe.SeverityWarning
	}

	result.Results = append(result.Results, probe.Result{
		CheckName:	c.Name(),
		Severity:	severity,
		Message:	fmt.Sprintf("Ingresses: %d with address, %d without, %d with TLS", withAddress, withoutAddress, withTLS),
		Details: []string{
			fmt.Sprintf("Total ingresses: %d", len(ingresses.Items)),
		},
	})

	return result, nil
}

func (c *IngressStatus) getIngressClass(ing *networkingv1.Ingress) string {
	if ing.Spec.IngressClassName != nil {
		return *ing.Spec.IngressClassName
	}
	if class, ok := ing.Annotations["kubernetes.io/ingress.class"]; ok {
		return class
	}
	return "default"
}

func (c *IngressStatus) getHosts(ing *networkingv1.Ingress) []string {
	hosts := make([]string, 0)
	for _, rule := range ing.Spec.Rules {
		if rule.Host != "" {
			hosts = append(hosts, rule.Host)
		}
	}
	return hosts
}
