package nettest

import (
	"context"
	"fmt"
	"os"
)

func (n *NetworkTest) TestCoreDNSConnectivity(ctx context.Context, pod TestPod, dnsIPs []string) []TestResult {
	var results []TestResult

	for _, ip := range dnsIPs {
		cmd := []string{"nc", "-z", "-w", "3", ip, "53"}
		_, _, err := n.ExecInPod(ctx, pod.Name, testNamespace, cmd)

		result := TestResult{
			SourceNode: pod.NodeName,
			SourcePod:  pod.Name,
			TestType:   "coredns",
			Target:     fmt.Sprintf("%s:53", ip),
			Success:    err == nil,
		}

		if err != nil {
			result.Error = err.Error()
		}

		if n.verbose {
			status := "OK"
			if !result.Success {
				status = "FAILED"
			}
			fmt.Fprintf(os.Stderr, "[network-test]   %s: CoreDNS %s - %s\n", pod.NodeName, result.Target, status)
		}

		results = append(results, result)
	}

	return results
}

func (n *NetworkTest) TestDNSResolution(ctx context.Context, pod TestPod) TestResult {
	cmd := []string{"nslookup", "github.com"}
	_, _, err := n.ExecInPod(ctx, pod.Name, testNamespace, cmd)

	result := TestResult{
		SourceNode: pod.NodeName,
		SourcePod:  pod.Name,
		TestType:   "dns",
		Target:     "github.com",
		Success:    err == nil,
	}

	if err != nil {
		result.Error = err.Error()
	}

	if n.verbose {
		status := "OK"
		if !result.Success {
			status = "FAILED"
		}
		fmt.Fprintf(os.Stderr, "[network-test]   %s: DNS %s - %s\n", pod.NodeName, result.Target, status)
	}

	return result
}

func (n *NetworkTest) TestExternalTCP(ctx context.Context, pod TestPod) TestResult {
	cmd := []string{"nc", "-z", "-w", "5", "github.com", "443"}
	_, _, err := n.ExecInPod(ctx, pod.Name, testNamespace, cmd)

	result := TestResult{
		SourceNode: pod.NodeName,
		SourcePod:  pod.Name,
		TestType:   "external-tcp",
		Target:     "github.com:443",
		Success:    err == nil,
	}

	if err != nil {
		result.Error = err.Error()
	}

	if n.verbose {
		status := "OK"
		if !result.Success {
			status = "FAILED"
		}
		fmt.Fprintf(os.Stderr, "[network-test]   %s: TCP %s - %s\n", pod.NodeName, result.Target, status)
	}

	return result
}

func (n *NetworkTest) TestKubeletConnectivity(ctx context.Context, pod TestPod, nodeIPs map[string]string) []TestResult {
	var results []TestResult

	for nodeName, nodeIP := range nodeIPs {
		if nodeName == pod.NodeName {
			continue
		}

		cmd := []string{"nc", "-z", "-w", "3", nodeIP, "10250"}
		_, _, err := n.ExecInPod(ctx, pod.Name, testNamespace, cmd)

		result := TestResult{
			SourceNode: pod.NodeName,
			SourcePod:  pod.Name,
			TestType:   "kubelet",
			Target:     fmt.Sprintf("%s:10250 (%s)", nodeIP, nodeName),
			Success:    err == nil,
		}

		if err != nil {
			result.Error = err.Error()
		}

		if n.verbose {
			status := "OK"
			if !result.Success {
				status = "FAILED"
			}
			fmt.Fprintf(os.Stderr, "[network-test]   %s: Kubelet %s - %s\n", pod.NodeName, result.Target, status)
		}

		results = append(results, result)
	}

	return results
}

func (n *NetworkTest) TestPodToPod(ctx context.Context, sourcePod TestPod, allPods []TestPod) []TestResult {
	var results []TestResult

	for _, targetPod := range allPods {
		if targetPod.Name == sourcePod.Name {
			continue
		}

		cmd := []string{"nc", "-z", "-w", "3", targetPod.PodIP, fmt.Sprintf("%d", testListenPort)}
		_, _, err := n.ExecInPod(ctx, sourcePod.Name, testNamespace, cmd)

		result := TestResult{
			SourceNode: sourcePod.NodeName,
			SourcePod:  sourcePod.Name,
			TestType:   "pod-to-pod",
			Target:     fmt.Sprintf("%s:%d (%s)", targetPod.PodIP, testListenPort, targetPod.NodeName),
			Success:    err == nil,
		}

		if err != nil {
			result.Error = err.Error()
		}

		if n.verbose {
			status := "OK"
			if !result.Success {
				status = "FAILED"
			}
			fmt.Fprintf(os.Stderr, "[network-test]   %s: Pod %s - %s\n", sourcePod.NodeName, result.Target, status)
		}

		results = append(results, result)
	}

	return results
}
