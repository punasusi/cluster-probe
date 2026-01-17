# cluster-probe: Kubernetes Cluster Diagnostic Tool

## Overview

cluster-probe is a self-contained Kubernetes cluster diagnostic tool that runs inside a Linux namespace-isolated container. It reuses the containerization pattern from cluster-bloom to provide a clean, reproducible environment for cluster analysis without requiring external container runtimes.

## Goals

1. **Diagnose Kubernetes cluster health** - Identify issues with nodes, pods, networking, storage, and control plane components
2. **Self-contained execution** - Run in an isolated namespace with all dependencies bundled
3. **Host kubeconfig access** - Read kubeconfig from the host filesystem to connect to the cluster
4. **Actionable output** - Provide clear, prioritized findings with remediation suggestions

## Non-Goals

1. Modifying the cluster state (read-only diagnostics)
2. Long-running monitoring (point-in-time analysis)
3. Multi-cluster management
4. Configuration management (that's cluster-bloom's domain)

---

## Architecture

### Reused from cluster-bloom

| Component | Source | Purpose |
|-----------|--------|---------|
| Namespace executor | `pkg/ansible/runtime/executor_linux.go` | Linux namespace isolation (UTS, PID, Mount) |
| Container image pull | `pkg/ansible/runtime/container.go` | Pull and extract base image for rootfs |
| Workspace management | `.bloom/` pattern | Local workspace with rootfs cache |
| Process model | Self-re-execution with `__child__` | Transition into namespaces cleanly |

### New Components

| Component | Purpose |
|-----------|---------|
| `pkg/probe/` | Core diagnostic engine |
| `pkg/probe/checks/` | Individual diagnostic checks |
| `pkg/probe/report/` | Report generation (text, JSON, HTML) |
| `pkg/k8s/` | Kubernetes client wrapper |

### Container Filesystem

```
.probe/
├── rootfs/                    # Extracted base image (minimal Go runtime)
│   ├── usr/
│   ├── etc/
│   ├── host/                  # Bind-mounted host filesystem
│   │   └── home/user/.kube/   # Access to kubeconfig
│   └── probe/                 # Bind-mounted probe binary and config
└── reports/                   # Generated diagnostic reports
```

### Host Interaction

```
┌─ Host ────────────────────────────────────────────────────────┐
│                                                                │
│  kubeconfig (~/.kube/config)                                  │
│       │                                                        │
│       ▼                                                        │
│  ┌─ Namespace Container ──────────────────────────────────┐   │
│  │                                                          │   │
│  │  /host/home/user/.kube/config ──► cluster-probe binary │   │
│  │                                          │               │   │
│  │                                          ▼               │   │
│  │                              Kubernetes API Server       │   │
│  │                              (via host network)          │   │
│  │                                          │               │   │
│  │                                          ▼               │   │
│  │                              Diagnostic Checks           │   │
│  │                                          │               │   │
│  │                                          ▼               │   │
│  │                              Report Generation           │   │
│  │                                          │               │   │
│  └──────────────────────────────────────────│───────────────┘   │
│                                              ▼                   │
│                                    .probe/reports/              │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

---

## Diagnostic Checks

### Tier 1: Critical Health

| Check | Description |
|-------|-------------|
| `node-status` | All nodes Ready, resource pressure conditions |
| `control-plane` | API server, etcd, scheduler, controller-manager health |
| `critical-pods` | kube-system pods running and healthy |
| `certificates` | Certificate expiration warnings |

### Tier 2: Workload Health

| Check | Description |
|-------|-------------|
| `pod-status` | Pending, CrashLooping, Evicted pods |
| `deployment-status` | Deployments with unavailable replicas |
| `pvc-status` | Pending or lost PersistentVolumeClaims |
| `job-failures` | Failed jobs and cronjobs |

### Tier 3: Resource Analysis

| Check | Description |
|-------|-------------|
| `resource-requests` | Over-committed nodes, missing requests/limits |
| `node-capacity` | CPU/memory/disk pressure indicators |
| `storage-health` | StorageClass availability, CSI driver status |
| `quota-usage` | ResourceQuota utilization |

### Tier 4: Networking

| Check | Description |
|-------|-------------|
| `service-endpoints` | Services with no endpoints |
| `ingress-status` | Ingress configuration issues |
| `network-policies` | Potential network isolation issues |
| `dns-resolution` | CoreDNS health and configuration |

### Tier 5: Security (Optional)

| Check | Description |
|-------|-------------|
| `rbac-audit` | Overly permissive roles |
| `pod-security` | Privileged containers, host mounts |
| `secrets-usage` | Unused secrets, secret references |
| `service-accounts` | Default SA usage, token mounts |

---

## CLI Interface

```bash
# Basic usage - diagnose current cluster
cluster-probe

# Specify kubeconfig location
cluster-probe --kubeconfig /path/to/kubeconfig

# Run specific check categories
cluster-probe --checks nodes,pods,networking

# Output formats
cluster-probe --output json
cluster-probe --output html --output-file report.html

# Verbosity
cluster-probe -v      # Show passing checks too
cluster-probe -vv     # Show raw API responses

# Skip containerization (for debugging)
cluster-probe --no-container
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All checks passed |
| 1 | Warnings found (non-critical issues) |
| 2 | Critical issues found |
| 3 | Could not connect to cluster |
| 4 | Internal error |

---

## Report Output

### Text Output (Default)

```
cluster-probe v0.1.0 - Kubernetes Cluster Diagnostics
======================================================

Cluster: kubernetes (https://192.168.1.100:6443)
Nodes: 3 | Pods: 47 | Namespaces: 12

CRITICAL
--------
✗ [node-status] Node worker-2 is NotReady (last transition: 15m ago)
  → Check kubelet logs: journalctl -u kubelet -n 50

✗ [pvc-status] PVC data-postgres-0 in namespace database is Pending
  → No StorageClass 'local-path' found. Available: longhorn

WARNINGS
--------
⚠ [certificates] API server certificate expires in 28 days
  → Renew with: kubeadm certs renew apiserver

⚠ [resource-requests] 12 pods have no resource requests defined
  → Namespaces affected: default (8), monitoring (4)

OK
--
✓ [control-plane] All control plane components healthy
✓ [critical-pods] All kube-system pods running
✓ [dns-resolution] CoreDNS responding correctly

Summary: 2 critical, 2 warnings, 3 passed
```

---

## Base Image Options

### Option A: Minimal Scratch + Go Binary (Recommended)

- Build static Go binary with `CGO_ENABLED=0`
- Use `scratch` or `gcr.io/distroless/static` as base
- Smallest footprint (~15MB)
- No shell access inside container

### Option B: Alpine Base

- Use `alpine:latest` as base
- Includes shell for debugging
- Can install additional tools (curl, dig) for network checks
- Larger footprint (~50MB)

### Recommendation

Start with Option A (distroless) for production, with a `--debug` flag that uses Option B for troubleshooting.

---

## Implementation Phases

### Phase 1: Foundation

- [ ] Project structure and build system
- [ ] Namespace executor (port from cluster-bloom)
- [ ] Image pull and rootfs extraction
- [ ] Basic Kubernetes client setup
- [ ] kubeconfig discovery from host mount

### Phase 2: Core Diagnostics

- [ ] Tier 1 checks (nodes, control-plane, critical pods)
- [ ] Tier 2 checks (pods, deployments, PVCs)
- [ ] Text report output
- [ ] Basic CLI with flags

### Phase 3: Extended Diagnostics

- [ ] Tier 3 checks (resources, storage, quotas)
- [ ] Tier 4 checks (networking, DNS)
- [ ] JSON output format
- [ ] Verbose modes

### Phase 4: Polish

- [ ] Tier 5 checks (security, optional)
- [ ] HTML report output
- [ ] Performance optimization
- [ ] Documentation and examples

---

## Technical Decisions

### Kubernetes Client

Use `k8s.io/client-go` for API access:
- Well-maintained, official client
- Supports all authentication methods via kubeconfig
- InCluster config not needed (we're outside the cluster)

### kubeconfig Discovery

Priority order:
1. `--kubeconfig` flag
2. `KUBECONFIG` environment variable (from host)
3. `/host/home/{user}/.kube/config`
4. `/host/root/.kube/config`

### Network Access

The container does NOT use network namespaces (`CLONE_NEWNET` is not set), so it shares the host network. This allows direct access to the Kubernetes API server without port forwarding.

### Concurrency

Run diagnostic checks concurrently where possible (grouped by API resource type to avoid rate limiting). Use a worker pool pattern.

---

## Open Questions

1. **Should we support in-cluster execution?** (Running as a pod for remote diagnostics)
2. **Should we cache check results?** (For comparing cluster state over time)
3. **Should we support custom check plugins?** (User-defined diagnostic scripts)
4. **Should we integrate with cluster-bloom?** (Run diagnostics after provisioning)

---

## Success Metrics

- Diagnose a typical cluster in <30 seconds
- Zero false positives for critical issues
- Actionable remediation for every finding
- Works on RKE2, k3s, kubeadm, and managed Kubernetes (EKS, GKE, AKS)
