# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Code Style

**No comments in code.** Code should be self-documenting through clear naming and structure. Do not add comments to Go files (except for build tags like `//go:build`).

## Project Overview

cluster-probe is a Kubernetes cluster diagnostic tool that runs inside Linux namespace isolation. It provides read-only, point-in-time cluster health analysis with actionable remediation suggestions.

## Module

`github.com/punasusi/cluster-probe`

## Build Commands

```bash
# Build static binary (no CGO)
CGO_ENABLED=0 go build -o cluster-probe ./cmd/cluster-probe

# Run tests
go test ./...

# Run single test
go test -run TestName ./pkg/probe/...
```

## Architecture

The tool uses containerization patterns with namespace isolation:
- **Namespace executor**: Linux namespace isolation (UTS, PID, Mount) via self-re-execution with `__CLUSTER_PROBE_CHILD__` env marker
- **Read-only credentials**: Auto-creates `cluster-reader` ServiceAccount with read-only RBAC (no secrets access)
- **Scan comparison**: Stores last scan in `.probe/last-scan.json` and shows diffs
- **Configuration**: Optional `.probe/config.yaml` for customization

Key packages:
- `pkg/probe/` - Core diagnostic engine with Check interface
- `pkg/probe/checks/` - 20 diagnostic checks organized in 5 tiers
- `pkg/probe/report/` - Text and JSON report generation with diff display
- `pkg/probe/storage/` - Scan storage and comparison
- `pkg/probe/config/` - YAML configuration loading
- `pkg/setup/` - Read-only user setup and credential generation
- `pkg/k8s/` - Kubernetes client wrapper using `k8s.io/client-go`
- `pkg/container/` - Linux namespace isolation (requires root)

### Network Model

The container shares host network (no `CLONE_NEWNET`) for direct Kubernetes API access. Host filesystem is bind-mounted at `/host/` for kubeconfig access.

### kubeconfig Discovery Order

1. `--kubeconfig` flag
2. `KUBECONFIG` environment variable
3. `/host/home/{user}/.kube/config`
4. `/host/root/.kube/config`

## Diagnostic Check Tiers

- **Tier 1 (Critical)**: node-status, control-plane, critical-pods, certificates
- **Tier 2 (Workload)**: pod-status, deployment-status, pvc-status, job-failures
- **Tier 3 (Resource)**: resource-requests, node-capacity, storage-health, quota-usage
- **Tier 4 (Networking)**: service-endpoints, ingress-status, network-policies, dns-resolution
- **Tier 5 (Security)**: rbac-audit, pod-security, secrets-usage, service-accounts

## Exit Codes

- 0: All checks passed
- 1: Warnings found
- 2: Critical issues found
- 3: Could not connect to cluster
- 4: Internal error

## CLI Flags

```
--kubeconfig string   Path to kubeconfig file
--no-container        Run without container isolation
--setup               Force setup mode to create read-only credentials
-v, --verbose         Enable verbose output (shows all checks, not just critical)
-o, --output string   Output format: text, json (default "text")
--no-diff             Skip comparison with previous scan
--init-config         Create example config file at .probe/config.yaml
```

## Project Structure

```
cmd/cluster-probe/main.go           # Cobra CLI entry point
pkg/
├── k8s/
│   ├── kubeconfig.go               # kubeconfig discovery
│   └── client.go                   # client-go wrapper
├── probe/
│   ├── result.go                   # Severity, Result, CheckResult types
│   ├── engine.go                   # Check interface, concurrent execution, config support
│   ├── checks/                     # 20 diagnostic check implementations
│   │   ├── node_status.go          # Tier 1
│   │   ├── control_plane.go        # Tier 1
│   │   ├── critical_pods.go        # Tier 1
│   │   ├── certificates.go         # Tier 1
│   │   ├── pod_status.go           # Tier 2
│   │   ├── deployment_status.go    # Tier 2
│   │   ├── pvc_status.go           # Tier 2
│   │   ├── job_failures.go         # Tier 2
│   │   ├── resource_requests.go    # Tier 3
│   │   ├── node_capacity.go        # Tier 3
│   │   ├── storage_health.go       # Tier 3
│   │   ├── quota_usage.go          # Tier 3
│   │   ├── service_endpoints.go    # Tier 4
│   │   ├── ingress_status.go       # Tier 4
│   │   ├── network_policies.go     # Tier 4
│   │   ├── dns_resolution.go       # Tier 4
│   │   ├── rbac_audit.go           # Tier 5
│   │   ├── pod_security.go         # Tier 5
│   │   ├── secrets_usage.go        # Tier 5
│   │   └── service_accounts.go     # Tier 5
│   ├── report/
│   │   └── report.go               # Text/JSON report with diff support
│   ├── storage/
│   │   └── storage.go              # Scan storage and comparison
│   └── config/
│       └── config.go               # YAML config loading
├── container/
│   ├── executor_stub.go            # Non-Linux stub
│   └── executor_linux.go           # Linux namespace isolation
└── setup/
    └── setup.go                    # Read-only user setup with CRD discovery
```

## .probe Directory

```
.probe/
├── config.yaml      # Custom configuration (created with --init-config)
└── last-scan.json   # Previous scan for comparison
```

## Read-Only Setup

On first run, cluster-probe creates:
- ServiceAccount: `cluster-reader` in `default` namespace
- ClusterRole: `cluster-reader-no-secrets` (read-only, excludes secrets)
- ClusterRoleBinding: `cluster-reader-binding`
- Token Secret: `cluster-reader-token`
- Kubeconfig: `.kube/probe.yaml`

The ClusterRole dynamically includes read permissions for all CRD API groups.

## Configuration File

`.probe/config.yaml` supports:
- Enabling/disabling specific checks
- Ignoring namespaces
- Adjusting thresholds (pod age, job age, resource percentages, etc.)

## Adding New Checks

1. Create a new file in `pkg/probe/checks/`
2. Implement the `Check` interface:
   ```go
   type Check interface {
       Name() string
       Tier() int
       Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error)
   }
   ```
3. Optionally implement `ConfigurableCheck` for config support
4. Register in `cmd/cluster-probe/main.go`
