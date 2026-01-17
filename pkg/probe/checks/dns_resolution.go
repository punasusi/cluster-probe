package checks

import (
	"context"
	"fmt"

	"github.com/punasusi/cluster-probe/pkg/probe"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type DNSResolution struct{}

func NewDNSResolution() *DNSResolution {
	return &DNSResolution{}
}

func (c *DNSResolution) Name() string {
	return "dns-resolution"
}

func (c *DNSResolution) Tier() int {
	return 4
}

func (c *DNSResolution) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	dnsService, err := client.CoreV1().Services("kube-system").Get(ctx, "kube-dns", metav1.GetOptions{})
	if err != nil {

		dnsService, err = client.CoreV1().Services("kube-system").Get(ctx, "coredns", metav1.GetOptions{})
	}
	if err != nil {

		dnsService, err = client.CoreV1().Services("kube-system").Get(ctx, "rke2-coredns-rke2-coredns", metav1.GetOptions{})
	}

	if err != nil {
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityCritical,
			Message:	"DNS service not found",
			Details:	[]string{"Could not find kube-dns, coredns, or rke2-coredns service"},
			Remediation:	"Check DNS addon installation",
		})
		return result, nil
	}

	endpoints, err := client.CoreV1().Endpoints("kube-system").Get(ctx, dnsService.Name, metav1.GetOptions{})
	if err != nil {
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityCritical,
			Message:	fmt.Sprintf("DNS endpoints not found for %s", dnsService.Name),
			Remediation:	"Check DNS pod status",
		})
		return result, nil
	}

	readyAddresses := 0
	notReadyAddresses := 0
	for _, subset := range endpoints.Subsets {
		readyAddresses += len(subset.Addresses)
		notReadyAddresses += len(subset.NotReadyAddresses)
	}

	if readyAddresses == 0 {
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityCritical,
			Message:	"No ready DNS endpoints",
			Details:	[]string{fmt.Sprintf("Not ready: %d", notReadyAddresses)},
			Remediation:	"Check DNS pod status: kubectl get pods -n kube-system -l k8s-app=kube-dns",
		})
	} else {
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityOK,
			Message:	fmt.Sprintf("DNS service %s has %d ready endpoints", dnsService.Name, readyAddresses),
			Details: []string{
				fmt.Sprintf("ClusterIP: %s", dnsService.Spec.ClusterIP),
			},
		})
	}

	pods, err := client.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{})
	if err == nil {
		dnsPodsReady := 0
		dnsPodsTotal := 0

		for _, pod := range pods.Items {

			if containsComponent(pod.Name, "coredns") || containsComponent(pod.Name, "kube-dns") {

				if containsComponent(pod.Name, "helm-install") {
					continue
				}

				dnsPodsTotal++
				if pod.Status.Phase == corev1.PodRunning {
					allReady := true
					for _, cs := range pod.Status.ContainerStatuses {
						if !cs.Ready {
							allReady = false
							break
						}
					}
					if allReady {
						dnsPodsReady++
					}
				}
			}
		}

		if dnsPodsTotal > 0 {
			severity := probe.SeverityOK
			if dnsPodsReady < dnsPodsTotal {
				severity = probe.SeverityWarning
			}
			if dnsPodsReady == 0 {
				severity = probe.SeverityCritical
			}

			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	severity,
				Message:	fmt.Sprintf("DNS pods: %d/%d ready", dnsPodsReady, dnsPodsTotal),
			})
		}
	}

	configMaps, err := client.CoreV1().ConfigMaps("kube-system").List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, cm := range configMaps.Items {
			if cm.Name == "coredns" || cm.Name == "kube-dns" {
				result.Results = append(result.Results, probe.Result{
					CheckName:	c.Name(),
					Severity:	probe.SeverityOK,
					Message:	fmt.Sprintf("DNS ConfigMap %s found", cm.Name),
				})
				break
			}
		}
	}

	return result, nil
}
