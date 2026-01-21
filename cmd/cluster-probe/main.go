package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/punasusi/cluster-probe/pkg/container"
	"github.com/punasusi/cluster-probe/pkg/k8s"
	"github.com/punasusi/cluster-probe/pkg/nettest"
	"github.com/punasusi/cluster-probe/pkg/probe"
	"github.com/punasusi/cluster-probe/pkg/probe/checks"
	"github.com/punasusi/cluster-probe/pkg/probe/config"
	"github.com/punasusi/cluster-probe/pkg/probe/report"
	"github.com/punasusi/cluster-probe/pkg/probe/storage"
	"github.com/punasusi/cluster-probe/pkg/setup"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

const (
	ExitOK		= 0
	ExitWarning	= 1
	ExitCritical	= 2
	ExitNoConnect	= 3
	ExitInternalErr	= 4
)

var (
	kubeconfig	string
	noContainer	bool
	verbose		bool
	forceSetup	bool
	outputFormat	string
	noDiff		bool
	initConfig	bool
	networkTest	bool
)

func init() {

	klog.InitFlags(nil)
	flag.Set("logtostderr", "false")
	flag.Set("stderrthreshold", "FATAL")
}

func main() {
	rootCmd := &cobra.Command{
		Use:	"cluster-probe",
		Short:	"Kubernetes cluster diagnostic tool",
		Long:	"A read-only diagnostic tool that analyzes Kubernetes cluster health and provides actionable remediation suggestions.",
		RunE:	run,
	}

	rootCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file")
	rootCmd.Flags().BoolVar(&noContainer, "no-container", false, "Run without container isolation")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.Flags().BoolVar(&forceSetup, "setup", false, "Force setup mode to create read-only credentials")
	rootCmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json")
	rootCmd.Flags().BoolVar(&noDiff, "no-diff", false, "Skip comparison with previous scan")
	rootCmd.Flags().BoolVar(&initConfig, "init-config", false, "Create example config file at .probe/config.yaml")
	rootCmd.Flags().BoolVar(&networkTest, "network-test", false, "Run network connectivity tests (creates temporary pods on each node)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(ExitInternalErr)
	}
}

func run(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	executor := container.NewExecutor()
	executor.SetVerbose(verbose)

	if container.IsChild() {
		return executor.Run(func() error {
			return runProbe(ctx, true)
		})
	}

	if !noContainer && executor.IsSupported() {
		return executor.Run(func() error {
			return runProbe(ctx, true)
		})
	}

	if verbose && !noContainer && executor.RequiresRoot() {
		fmt.Fprintln(os.Stderr, "[info] Running without namespace isolation (requires root)")
	}

	return runProbe(ctx, false)
}

func runProbe(ctx context.Context, inContainer bool) error {

	store := storage.NewStorage("")

	if initConfig {
		configPath := store.ConfigPath()
		if err := store.EnsureProbeDir(); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating .probe directory: %v\n", err)
			os.Exit(ExitInternalErr)
		}
		if err := config.SaveExample(configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating config file: %v\n", err)
			os.Exit(ExitInternalErr)
		}
		fmt.Printf("Created example config at: %s\n", configPath)
		os.Exit(ExitOK)
	}

	probeKubeconfigPath := setup.ProbeKubeconfigPath()

	needsSetup := forceSetup || !setup.ProbeKubeconfigExists()

	if needsSetup && !networkTest {
		return runSetup(ctx, inContainer, probeKubeconfigPath)
	}

	if networkTest {
		return runNetworkTest(ctx, inContainer)
	}

	cfg, err := config.LoadConfig(store.ConfigPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load config: %v\n", err)
		cfg = config.DefaultConfig()
	}

	var previousScan *storage.ScanRecord
	if !noDiff {
		previousScan, err = store.LoadLastScan()
		if err != nil && verbose {
			fmt.Fprintf(os.Stderr, "Warning: failed to load previous scan: %v\n", err)
		}
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Using probe kubeconfig: %s\n", probeKubeconfigPath)
	}

	client, err := k8s.NewClient(probeKubeconfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitNoConnect)
	}

	if err := client.TestConnection(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitNoConnect)
	}

	clusterInfo, err := client.ClusterInfo(ctx)
	if err != nil {
		clusterInfo = "unknown"
	}

	engine := probe.NewEngine(verbose)
	engine.SetConfig(cfg)
	engine.SetDynamicClients(client.DynamicClient(), client.DiscoveryClient())

	engine.Register(checks.NewNodeStatus())
	engine.Register(checks.NewControlPlane())
	engine.Register(checks.NewCriticalPods())
	engine.Register(checks.NewCertificates())

	engine.Register(checks.NewPodStatus())
	engine.Register(checks.NewDeploymentStatus())
	engine.Register(checks.NewPVCStatus())
	engine.Register(checks.NewJobFailures())
	engine.Register(checks.NewStalledResources())

	engine.Register(checks.NewResourceRequests())
	engine.Register(checks.NewNodeCapacity())
	engine.Register(checks.NewStorageHealth())
	engine.Register(checks.NewQuotaUsage())

	engine.Register(checks.NewServiceEndpoints())
	engine.Register(checks.NewIngressStatus())
	engine.Register(checks.NewNetworkPolicies())
	engine.Register(checks.NewDNSResolution())

	engine.Register(checks.NewRBACAudit())
	engine.Register(checks.NewPodSecurity())
	engine.Register(checks.NewSecretsUsage())
	engine.Register(checks.NewServiceAccounts())

	results, err := engine.Run(ctx, client.Clientset())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running checks: %v\n", err)
		os.Exit(ExitInternalErr)
	}

	currentScan := buildScanRecord(results, clusterInfo)

	var diff *storage.ScanDiff
	if previousScan != nil {
		diff = storage.ComputeDiff(currentScan, previousScan)
	}

	if !noDiff {
		if err := store.SaveScan(currentScan); err != nil && verbose {
			fmt.Fprintf(os.Stderr, "Warning: failed to save scan: %v\n", err)
		}
	}

	format := report.FormatText
	if outputFormat == "json" {
		format = report.FormatJSON
	}

	writer := report.NewWriter(os.Stdout, format, verbose)
	writer.SetDiff(diff)
	if err := writer.Write(results, clusterInfo); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		os.Exit(ExitInternalErr)
	}

	switch engine.MaxSeverity(results) {
	case probe.SeverityCritical:
		os.Exit(ExitCritical)
	case probe.SeverityWarning:
		os.Exit(ExitWarning)
	default:
		os.Exit(ExitOK)
	}

	return nil
}

func buildScanRecord(results []probe.CheckResult, clusterInfo string) *storage.ScanRecord {
	record := &storage.ScanRecord{
		Timestamp:	time.Now().UTC(),
		Cluster:	clusterInfo,
		Issues:		make([]storage.StoredIssue, 0),
	}

	for _, cr := range results {
		severity := cr.MaxSeverity()
		switch severity {
		case probe.SeverityCritical:
			record.Summary.Critical++
		case probe.SeverityWarning:
			record.Summary.Warning++
		case probe.SeverityOK:
			record.Summary.OK++
		}
		record.Summary.Total++

		for _, r := range cr.Results {
			if r.Severity == probe.SeverityOK {
				continue
			}
			issue := storage.StoredIssue{
				CheckName:	cr.Name,
				Severity:	r.Severity.String(),
				Message:	r.Message,
				Fingerprint:	storage.GenerateFingerprint(cr.Name, r.Severity.String(), r.Message),
			}
			record.Issues = append(record.Issues, issue)
		}
	}

	return record
}

func runSetup(ctx context.Context, inContainer bool, outputPath string) error {
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("                    CLUSTER PROBE SETUP")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("No read-only credentials found. Setting up cluster-probe...")
	fmt.Println()

	kubeconfigPath := k8s.DiscoverKubeconfig(kubeconfig, inContainer)
	if kubeconfigPath == "" {
		fmt.Fprintln(os.Stderr, "Error: could not find kubeconfig for setup")
		os.Exit(ExitNoConnect)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Using host kubeconfig for setup: %s\n", kubeconfigPath)
	}

	client, err := k8s.NewClient(kubeconfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitNoConnect)
	}

	if err := client.TestConnection(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitNoConnect)
	}

	s := setup.NewSetup(client.Clientset(), kubeconfigPath, verbose)

	var setupErr error
	for i := 0; i < 5; i++ {
		setupErr = s.Run(ctx, outputPath)
		if setupErr == nil {
			break
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "[setup] Attempt %d failed: %v, retrying...\n", i+1, setupErr)
		}
		time.Sleep(time.Second)
	}

	if setupErr != nil {
		fmt.Fprintf(os.Stderr, "Error during setup: %v\n", setupErr)
		os.Exit(ExitInternalErr)
	}

	fmt.Println()
	fmt.Printf("Setup complete! Read-only credentials saved to: %s\n", outputPath)
	fmt.Println()
	fmt.Println("Run cluster-probe again to perform diagnostics.")
	fmt.Println()

	os.Exit(ExitOK)
	return nil
}

func runNetworkTest(ctx context.Context, inContainer bool) error {
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("                  CLUSTER PROBE NETWORK TEST")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()

	kubeconfigPath := k8s.DiscoverKubeconfig(kubeconfig, inContainer)
	if kubeconfigPath == "" {
		fmt.Fprintln(os.Stderr, "Error: could not find kubeconfig for network test")
		os.Exit(ExitNoConnect)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "[network-test] Using kubeconfig: %s\n", kubeconfigPath)
	}

	client, err := k8s.NewClient(kubeconfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitNoConnect)
	}

	if err := client.TestConnection(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitNoConnect)
	}

	clusterInfo, err := client.ClusterInfo(ctx)
	if err != nil {
		clusterInfo = "unknown"
	}

	nt := nettest.New(client.Clientset(), client.RESTConfig(), verbose)

	testReport, err := nt.Run(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Network test failed: %v\n", err)
		os.Exit(ExitInternalErr)
	}

	results := convertNetworkReport(testReport)

	format := report.FormatText
	if outputFormat == "json" {
		format = report.FormatJSON
	}

	writer := report.NewWriter(os.Stdout, format, verbose)
	if err := writer.Write(results, clusterInfo); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		os.Exit(ExitInternalErr)
	}

	if testReport.Summary.Failed > 0 {
		os.Exit(ExitCritical)
	}
	os.Exit(ExitOK)
	return nil
}

func convertNetworkReport(r *nettest.NetworkTestReport) []probe.CheckResult {
	typeNames := map[string]string{
		"coredns":      "CoreDNS Connectivity",
		"dns":          "DNS Resolution",
		"external-tcp": "External TCP Connectivity",
		"kubelet":      "Kubelet Connectivity",
		"pod-to-pod":   "Pod-to-Pod Connectivity",
	}

	byType := make(map[string][]nettest.TestResult)
	for _, tr := range r.TestResults {
		byType[tr.TestType] = append(byType[tr.TestType], tr)
	}

	var results []probe.CheckResult

	typeOrder := []string{"coredns", "dns", "external-tcp", "kubelet", "pod-to-pod"}
	for _, testType := range typeOrder {
		typeResults, ok := byType[testType]
		if !ok {
			continue
		}

		checkResult := probe.CheckResult{
			Name:    fmt.Sprintf("network-test-%s", testType),
			Tier:    4,
			Results: []probe.Result{},
		}

		passed := 0
		failed := 0
		for _, tr := range typeResults {
			if tr.Success {
				passed++
			} else {
				failed++
				checkResult.Results = append(checkResult.Results, probe.Result{
					CheckName:   checkResult.Name,
					Severity:    probe.SeverityCritical,
					Message:     fmt.Sprintf("%s: %s -> %s failed", typeNames[testType], tr.SourceNode, tr.Target),
					Details:     []string{tr.Error},
					Remediation: getNetworkRemediation(testType),
				})
			}
		}

		if failed == 0 && passed > 0 {
			checkResult.Results = append(checkResult.Results, probe.Result{
				CheckName: checkResult.Name,
				Severity:  probe.SeverityOK,
				Message:   fmt.Sprintf("%s: %d/%d tests passed", typeNames[testType], passed, passed),
			})
		}

		results = append(results, checkResult)
	}

	return results
}

func getNetworkRemediation(testType string) string {
	switch testType {
	case "coredns":
		return "Check CoreDNS pod status and network policies: kubectl get pods -n kube-system -l k8s-app=kube-dns"
	case "dns":
		return "Verify DNS resolution works and external DNS is reachable"
	case "external-tcp":
		return "Check firewall rules and network egress policies for external connectivity"
	case "kubelet":
		return "Verify node-to-node connectivity and firewall rules allow port 10250"
	case "pod-to-pod":
		return "Check CNI plugin status and network policies between namespaces"
	default:
		return "Check network configuration and policies"
	}
}
