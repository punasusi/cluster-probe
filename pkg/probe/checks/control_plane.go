package checks

import (
	"context"
	"fmt"

	"github.com/punasusi/cluster-probe/pkg/probe"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ControlPlane struct{}

func NewControlPlane() *ControlPlane {
	return &ControlPlane{}
}

func (c *ControlPlane) Name() string {
	return "control-plane"
}

func (c *ControlPlane) Tier() int {
	return 1
}

func (c *ControlPlane) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	pods, err := client.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list kube-system pods: %w", err)
	}

	components := map[string]struct {
		found	bool
		running	bool
		podName	string
		message	string
	}{
		"kube-apiserver":		{},
		"kube-controller-manager":	{},
		"kube-scheduler":		{},
		"etcd":				{},
	}

	for _, pod := range pods.Items {
		for component := range components {

			if containsComponent(pod.Name, component) {
				info := components[component]
				info.found = true
				info.podName = pod.Name

				if pod.Status.Phase == corev1.PodRunning {
					allReady := true
					for _, cs := range pod.Status.ContainerStatuses {
						if !cs.Ready {
							allReady = false
							info.message = fmt.Sprintf("Container %s not ready", cs.Name)
							break
						}
					}
					info.running = allReady
				} else {
					info.message = fmt.Sprintf("Pod phase: %s", pod.Status.Phase)
				}

				components[component] = info
			}
		}
	}

	allHealthy := true
	for component, info := range components {
		if !info.found {

			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	probe.SeverityWarning,
				Message:	fmt.Sprintf("%s not found in kube-system", component),
				Details:	[]string{"This may be normal for managed Kubernetes clusters"},
				Remediation:	"Verify control plane components are running if using self-managed Kubernetes",
			})
		} else if !info.running {
			allHealthy = false
			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	probe.SeverityCritical,
				Message:	fmt.Sprintf("%s is not healthy", component),
				Details:	[]string{fmt.Sprintf("Pod: %s", info.podName), info.message},
				Remediation:	fmt.Sprintf("Check %s logs with 'kubectl logs -n kube-system %s'", component, info.podName),
			})
		}
	}

	dnsHealthy := c.checkDNS(pods.Items, result)

	if allHealthy && dnsHealthy {
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityOK,
			Message:	"Control plane components are healthy",
		})
	}

	return result, nil
}

func (c *ControlPlane) checkDNS(pods []corev1.Pod, result *probe.CheckResult) bool {
	dnsFound := false
	dnsHealthy := true

	for _, pod := range pods {

		if containsComponent(pod.Name, "helm-install") {
			continue
		}

		if containsComponent(pod.Name, "coredns") || containsComponent(pod.Name, "kube-dns") {
			dnsFound = true

			if pod.Status.Phase != corev1.PodRunning {
				dnsHealthy = false
				result.Results = append(result.Results, probe.Result{
					CheckName:	c.Name(),
					Severity:	probe.SeverityCritical,
					Message:	fmt.Sprintf("DNS pod %s is not running", pod.Name),
					Details:	[]string{fmt.Sprintf("Phase: %s", pod.Status.Phase)},
					Remediation:	"Check DNS pod logs and events",
				})
			} else {
				for _, cs := range pod.Status.ContainerStatuses {
					if !cs.Ready {
						dnsHealthy = false
						result.Results = append(result.Results, probe.Result{
							CheckName:	c.Name(),
							Severity:	probe.SeverityWarning,
							Message:	fmt.Sprintf("DNS container %s in pod %s not ready", cs.Name, pod.Name),
							Remediation:	"Check DNS pod logs for errors",
						})
					}
				}
			}
		}
	}

	if !dnsFound {
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityWarning,
			Message:	"No CoreDNS or kube-dns pods found",
			Remediation:	"Verify cluster DNS is configured correctly",
		})
		return false
	}

	return dnsHealthy
}

func containsComponent(podName, component string) bool {

	if len(podName) >= len(component) && podName[:len(component)] == component {
		return true
	}

	for i := 0; i <= len(podName)-len(component); i++ {
		if podName[i:i+len(component)] == component {
			return true
		}
	}
	return false
}
