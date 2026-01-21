package nettest

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type NetworkTest struct {
	client     kubernetes.Interface
	restConfig *rest.Config
	verbose    bool
}

type TestResult struct {
	SourceNode string
	SourcePod  string
	TestType   string
	Target     string
	Success    bool
	Error      string
}

type TestSummary struct {
	Total  int
	Passed int
	Failed int
}

type NetworkTestReport struct {
	Timestamp   time.Time
	NodeCount   int
	PodCount    int
	TestResults []TestResult
	Summary     TestSummary
}

func New(client kubernetes.Interface, restConfig *rest.Config, verbose bool) *NetworkTest {
	return &NetworkTest{
		client:     client,
		restConfig: restConfig,
		verbose:    verbose,
	}
}

func (n *NetworkTest) Run(ctx context.Context) (*NetworkTestReport, error) {
	report := &NetworkTestReport{
		Timestamp:   time.Now().UTC(),
		TestResults: []TestResult{},
	}

	if n.verbose {
		fmt.Fprintln(os.Stderr, "[network-test] Creating namespace...")
	}
	if err := n.EnsureNamespace(ctx); err != nil {
		return nil, fmt.Errorf("failed to create namespace: %w", err)
	}

	_ = n.CleanupTestPods(ctx)

	if n.verbose {
		fmt.Fprintln(os.Stderr, "[network-test] Listing nodes...")
	}
	nodes, err := n.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	readyNodes := filterReadyNodes(nodes.Items)
	report.NodeCount = len(readyNodes)

	if len(readyNodes) == 0 {
		return nil, fmt.Errorf("no ready nodes found in cluster")
	}

	if n.verbose {
		fmt.Fprintf(os.Stderr, "[network-test] Found %d ready nodes\n", len(readyNodes))
	}

	testPods, err := n.CreateTestPods(ctx, readyNodes)
	if err != nil {
		n.CleanupTestPods(ctx)
		return nil, fmt.Errorf("failed to create test pods: %w", err)
	}

	defer func() {
		if n.verbose {
			fmt.Fprintln(os.Stderr, "[network-test] Cleaning up...")
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		n.CleanupTestPods(cleanupCtx)
	}()

	if n.verbose {
		fmt.Fprintln(os.Stderr, "[network-test] Waiting for pods to be ready...")
	}
	if err := n.WaitForPodsReady(ctx, testPods, 90*time.Second); err != nil {
		return nil, fmt.Errorf("pods failed to become ready: %w", err)
	}

	report.PodCount = len(testPods)

	coreDNSIPs, err := n.DiscoverCoreDNSPods(ctx)
	if err != nil && n.verbose {
		fmt.Fprintf(os.Stderr, "[network-test] Warning: could not discover CoreDNS pods: %v\n", err)
	}

	nodeIPs := n.GetNodeInternalIPs(readyNodes)

	if n.verbose {
		fmt.Fprintln(os.Stderr, "[network-test] Starting listeners on test pods...")
	}
	n.StartListeners(ctx, testPods)

	if n.verbose {
		fmt.Fprintln(os.Stderr, "[network-test] Running tests...")
	}
	results := n.RunAllTests(ctx, testPods, coreDNSIPs, nodeIPs)
	report.TestResults = results

	for _, r := range results {
		report.Summary.Total++
		if r.Success {
			report.Summary.Passed++
		} else {
			report.Summary.Failed++
		}
	}

	return report, nil
}

func (n *NetworkTest) DiscoverCoreDNSPods(ctx context.Context) ([]string, error) {
	var dnsIPs []string

	serviceNames := []string{"kube-dns", "coredns", "rke2-coredns-rke2-coredns"}

	for _, name := range serviceNames {
		endpoints, err := n.client.CoreV1().Endpoints("kube-system").Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			continue
		}

		for _, subset := range endpoints.Subsets {
			for _, addr := range subset.Addresses {
				dnsIPs = append(dnsIPs, addr.IP)
			}
		}

		if len(dnsIPs) > 0 {
			if n.verbose {
				fmt.Fprintf(os.Stderr, "[network-test] Found %d CoreDNS endpoints via %s\n", len(dnsIPs), name)
			}
			return dnsIPs, nil
		}
	}

	pods, err := n.client.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list kube-system pods: %w", err)
	}

	for _, pod := range pods.Items {
		if containsDNSComponent(pod.Name) && pod.Status.Phase == corev1.PodRunning && pod.Status.PodIP != "" {
			dnsIPs = append(dnsIPs, pod.Status.PodIP)
		}
	}

	if len(dnsIPs) == 0 {
		return nil, fmt.Errorf("no CoreDNS pods found")
	}

	if n.verbose {
		fmt.Fprintf(os.Stderr, "[network-test] Found %d CoreDNS pods by name pattern\n", len(dnsIPs))
	}
	return dnsIPs, nil
}

func (n *NetworkTest) GetNodeInternalIPs(nodes []corev1.Node) map[string]string {
	nodeIPs := make(map[string]string)
	for _, node := range nodes {
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				nodeIPs[node.Name] = addr.Address
				break
			}
		}
	}
	return nodeIPs
}

func (n *NetworkTest) RunAllTests(ctx context.Context, pods []TestPod, coreDNSIPs []string, nodeIPs map[string]string) []TestResult {
	var results []TestResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, pod := range pods {
		wg.Add(1)
		go func(p TestPod) {
			defer wg.Done()

			if n.verbose {
				fmt.Fprintf(os.Stderr, "[network-test] Running tests from %s...\n", p.NodeName)
			}

			podResults := n.RunPodTests(ctx, p, coreDNSIPs, nodeIPs, pods)

			mu.Lock()
			results = append(results, podResults...)
			mu.Unlock()
		}(pod)
	}

	wg.Wait()
	return results
}

func (n *NetworkTest) RunPodTests(ctx context.Context, pod TestPod, coreDNSIPs []string, nodeIPs map[string]string, allPods []TestPod) []TestResult {
	var results []TestResult

	results = append(results, n.TestCoreDNSConnectivity(ctx, pod, coreDNSIPs)...)
	results = append(results, n.TestDNSResolution(ctx, pod))
	results = append(results, n.TestExternalTCP(ctx, pod))
	results = append(results, n.TestKubeletConnectivity(ctx, pod, nodeIPs)...)
	results = append(results, n.TestPodToPod(ctx, pod, allPods)...)

	return results
}

func filterReadyNodes(nodes []corev1.Node) []corev1.Node {
	var ready []corev1.Node
	for _, node := range nodes {
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready = append(ready, node)
				break
			}
		}
	}
	return ready
}

func containsDNSComponent(name string) bool {
	return contains(name, "coredns") || contains(name, "kube-dns")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
