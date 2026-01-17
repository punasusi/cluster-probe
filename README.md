# cluster-probe

A read-only Kubernetes cluster diagnostic tool that analyzes cluster health and provides actionable remediation suggestions.

## Features

- **Read-only access** - Uses a dedicated service account with no secrets access
- **Comprehensive checks** - 20 diagnostic checks across 5 tiers
- **Scan comparison** - Shows new and resolved issues since last scan
- **Configurable** - Disable checks, ignore namespaces, adjust thresholds
- **Multiple output formats** - Text (default) and JSON
- **Exit codes** - Suitable for CI/CD pipelines and monitoring

## Installation

### Download binary

```bash
# Linux (amd64)
curl -Lo cluster-probe https://github.com/punasusi/cluster-probe/releases/latest/download/cluster-probe-linux-amd64
chmod +x cluster-probe

# Linux (arm64)
curl -Lo cluster-probe https://github.com/punasusi/cluster-probe/releases/latest/download/cluster-probe-linux-arm64
chmod +x cluster-probe

# macOS (Apple Silicon)
curl -Lo cluster-probe https://github.com/punasusi/cluster-probe/releases/latest/download/cluster-probe-darwin-arm64
chmod +x cluster-probe

# macOS (Intel)
curl -Lo cluster-probe https://github.com/punasusi/cluster-probe/releases/latest/download/cluster-probe-darwin-amd64
chmod +x cluster-probe
```

### Build from source

```bash
CGO_ENABLED=0 go build -o cluster-probe ./cmd/cluster-probe
```

### Requirements

- kubectl access to a Kubernetes cluster (for initial setup)

## Quick Start

```bash
# First run - creates read-only service account and credentials
./cluster-probe

# Subsequent runs - performs diagnostics
./cluster-probe

# Verbose output (shows all checks, not just critical)
./cluster-probe -v

# JSON output
./cluster-probe -o json
```

## Usage

```
cluster-probe [flags]

Flags:
      --kubeconfig string   Path to kubeconfig file
      --no-container        Run without container isolation
  -v, --verbose             Enable verbose output
      --setup               Force setup mode to create read-only credentials
  -o, --output string       Output format: text, json (default "text")
      --no-diff             Skip comparison with previous scan
      --init-config         Create example config file at .probe/config.yaml
  -h, --help                Help for cluster-probe
```

## First Run Setup

On first run, cluster-probe creates a read-only service account in your cluster:

1. **ServiceAccount**: `cluster-reader` in `default` namespace
2. **ClusterRole**: `cluster-reader-no-secrets` with read-only access (no secrets)
3. **ClusterRoleBinding**: Binds the service account to the role
4. **Kubeconfig**: Saved to `.kube/probe.yaml`

This requires your current kubeconfig to have permissions to create these resources. After setup, cluster-probe uses only the restricted read-only credentials.

To re-run setup (e.g., after cluster changes):
```bash
./cluster-probe --setup
```

## Diagnostic Checks

### Tier 1: Critical
| Check | Description |
|-------|-------------|
| `node-status` | Verifies all nodes are Ready and checks for conditions |
| `control-plane` | Checks API server, controller-manager, scheduler, etcd, DNS |
| `critical-pods` | Monitors kube-system pods for CrashLoopBackOff or failures |
| `certificates` | Checks certificate expiration and CSR status |

### Tier 2: Workload
| Check | Description |
|-------|-------------|
| `pod-status` | Identifies pending, failed, CrashLoopBackOff, ImagePullBackOff pods |
| `deployment-status` | Checks deployment replica availability and progress |
| `pvc-status` | Finds pending or lost PersistentVolumeClaims |
| `job-failures` | Detects failed jobs and long-running jobs |

### Tier 3: Resource
| Check | Description |
|-------|-------------|
| `resource-requests` | Reports containers without CPU/memory requests |
| `node-capacity` | Monitors node CPU and memory utilization |
| `storage-health` | Checks storage classes, CSI drivers, volume attachments |
| `quota-usage` | Monitors ResourceQuota usage in namespaces |

### Tier 4: Networking
| Check | Description |
|-------|-------------|
| `service-endpoints` | Finds services with no endpoints |
| `ingress-status` | Checks ingress configurations and TLS |
| `network-policies` | Reports namespaces without network policies |
| `dns-resolution` | Verifies CoreDNS is running and healthy |

### Tier 5: Security
| Check | Description |
|-------|-------------|
| `rbac-audit` | Detects overly permissive RBAC roles and bindings |
| `pod-security` | Finds privileged containers, root users, host namespaces |
| `secrets-usage` | Checks secret exposure patterns (env vars vs volumes) |
| `service-accounts` | Audits service account usage and configurations |

## Output Formats

### Text (default)

Shows only critical issues by default:
```
  CLUSTER PROBE REPORT
────────────────────────────────────────────────────────────
  Cluster: my-cluster (v1.28.0)
  Time:    2026-01-17 12:00:00 UTC

  Critical Issues:
  ✗ [deployment-status] Deployment app/api has insufficient replicas
    → Check pods: kubectl get pods -n app -l app=api

  Summary: ✗ 1 critical  ⚠ 3 warning  ✓ 16 passed
```

With `-v` (verbose), shows all checks grouped by tier with full details.

### JSON

```bash
./cluster-probe -o json
```

Returns structured JSON with all checks, results, and diff information:
```json
{
  "timestamp": "2026-01-17T12:00:00Z",
  "cluster": "my-cluster (v1.28.0)",
  "summary": {
    "total": 20,
    "critical": 1,
    "warning": 3,
    "ok": 16
  },
  "checks": [...],
  "diff": {
    "previous_time": "2026-01-17T11:00:00Z",
    "new_issues": [...],
    "resolved_issues": [...],
    "critical_delta": 0,
    "warning_delta": -1
  }
}
```

## Scan Comparison

cluster-probe automatically stores scan results in `.probe/last-scan.json` and shows differences on subsequent runs:

```
  New Issues (since last scan):
    ⚠ [pod-status] Pod app/worker-abc is in CrashLoopBackOff

  Resolved Issues:
    ✓ [deployment-status] Deployment app/api has insufficient replicas

  Summary: ✗ 0 critical  ⚠ 4 warning  ✓ 16 passed (-1 critical since last scan)
```

To skip comparison:
```bash
./cluster-probe --no-diff
```

## Configuration

Create a config file to customize behavior:

```bash
./cluster-probe --init-config
```

This creates `.probe/config.yaml`:

```yaml
# Disable specific checks
checks:
  dns-resolution:
    enabled: false
  network-policies:
    enabled: false

# Ignore namespaces (no issues reported from these)
ignore:
  namespaces:
    - kube-system
    - monitoring

  # Checks to skip entirely
  checks:
    - dns-resolution

# Adjust thresholds
thresholds:
  # Warn if more than N pods use default service account
  default_service_account_pods: 10

  # Warn if pods are pending longer than N minutes
  pending_pod_age_minutes: 30

  # Warn if jobs are running longer than N hours
  job_running_age_hours: 24

  # Warn if certificates expire within N days
  certificate_expiry_warning_days: 30

  # Node resource thresholds (percent)
  node_cpu_warning_percent: 80
  node_memory_warning_percent: 80
  node_memory_critical_percent: 95
```

## Directory Structure

```
.probe/
├── config.yaml      # Custom configuration (optional)
└── last-scan.json   # Previous scan for comparison

.kube/
└── probe.yaml       # Read-only kubeconfig (created by setup)
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All checks passed (OK) |
| 1 | Warnings found |
| 2 | Critical issues found |
| 3 | Could not connect to cluster |
| 4 | Internal error |

Use exit codes in scripts:
```bash
./cluster-probe
if [ $? -eq 2 ]; then
  echo "Critical issues found!"
fi
```

## CI/CD Integration

```yaml
# GitHub Actions example
- name: Run cluster diagnostics
  run: |
    ./cluster-probe -o json > probe-report.json
    if [ $? -ge 2 ]; then
      echo "::error::Critical cluster issues detected"
      exit 1
    fi
```

## Security

- **Read-only access**: The service account cannot modify any resources
- **No secrets access**: Explicitly excluded from RBAC permissions
- **Minimal permissions**: Only list/get/watch verbs on cluster resources
- **Local credentials**: Kubeconfig stored locally, not transmitted

## Troubleshooting

### Setup fails with permission denied
Your current kubeconfig needs cluster-admin or equivalent permissions to create the service account and RBAC resources. After setup, only read-only permissions are used.

### Cannot connect to cluster
Ensure `.kube/probe.yaml` exists and the cluster is reachable. Re-run setup if credentials are stale:
```bash
./cluster-probe --setup
```

### Too many warnings
Use the config file to:
- Ignore noisy namespaces
- Adjust thresholds
- Disable specific checks

## License

MIT
