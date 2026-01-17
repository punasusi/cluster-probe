package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Checks     map[string]CheckConfig `yaml:"checks,omitempty"`
	Ignore     IgnoreConfig           `yaml:"ignore,omitempty"`
	Thresholds ThresholdConfig        `yaml:"thresholds,omitempty"`
}

type CheckConfig struct {
	Enabled  *bool  `yaml:"enabled,omitempty"`
	Severity string `yaml:"severity,omitempty"`
}

type IgnoreConfig struct {
	Namespaces []string `yaml:"namespaces,omitempty"`
	Checks     []string `yaml:"checks,omitempty"`
	Patterns   []string `yaml:"patterns,omitempty"`
}

type ThresholdConfig struct {
	DefaultServiceAccountPods int `yaml:"default_service_account_pods,omitempty"`
	PendingPodAge             int `yaml:"pending_pod_age_minutes,omitempty"`
	JobRunningAge             int `yaml:"job_running_age_hours,omitempty"`
	CertificateExpiryWarning  int `yaml:"certificate_expiry_warning_days,omitempty"`
	NodeCPUWarning            int `yaml:"node_cpu_warning_percent,omitempty"`
	NodeMemoryWarning         int `yaml:"node_memory_warning_percent,omitempty"`
	NodeMemoryCritical        int `yaml:"node_memory_critical_percent,omitempty"`
}

func DefaultConfig() *Config {
	enabled := true
	return &Config{
		Checks: map[string]CheckConfig{
			"node-status":		{Enabled: &enabled},
			"control-plane":	{Enabled: &enabled},
			"critical-pods":	{Enabled: &enabled},
			"certificates":		{Enabled: &enabled},
			"pod-status":		{Enabled: &enabled},
			"deployment-status":	{Enabled: &enabled},
			"pvc-status":		{Enabled: &enabled},
			"job-failures":		{Enabled: &enabled},
			"resource-requests":	{Enabled: &enabled},
			"node-capacity":	{Enabled: &enabled},
			"storage-health":	{Enabled: &enabled},
			"quota-usage":		{Enabled: &enabled},
			"service-endpoints":	{Enabled: &enabled},
			"ingress-status":	{Enabled: &enabled},
			"network-policies":	{Enabled: &enabled},
			"dns-resolution":	{Enabled: &enabled},
			"rbac-audit":		{Enabled: &enabled},
			"pod-security":		{Enabled: &enabled},
			"secrets-usage":	{Enabled: &enabled},
			"service-accounts":	{Enabled: &enabled},
		},
		Thresholds: ThresholdConfig{
			DefaultServiceAccountPods:	10,
			PendingPodAge:			30,
			JobRunningAge:			24,
			CertificateExpiryWarning:	30,
			NodeCPUWarning:			80,
			NodeMemoryWarning:		80,
			NodeMemoryCritical:		95,
		},
	}
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return cfg, nil
}

func (c *Config) IsCheckEnabled(name string) bool {

	if checkCfg, ok := c.Checks[name]; ok {
		if checkCfg.Enabled != nil {
			return *checkCfg.Enabled
		}
	}

	for _, ignored := range c.Ignore.Checks {
		if ignored == name {
			return false
		}
	}

	return true
}

func (c *Config) IsNamespaceIgnored(namespace string) bool {
	for _, ns := range c.Ignore.Namespaces {
		if ns == namespace {
			return true
		}
	}
	return false
}

func (c *Config) GetThreshold(name string) int {
	switch name {
	case "default_service_account_pods":
		if c.Thresholds.DefaultServiceAccountPods > 0 {
			return c.Thresholds.DefaultServiceAccountPods
		}
		return 10
	case "pending_pod_age_minutes":
		if c.Thresholds.PendingPodAge > 0 {
			return c.Thresholds.PendingPodAge
		}
		return 30
	case "job_running_age_hours":
		if c.Thresholds.JobRunningAge > 0 {
			return c.Thresholds.JobRunningAge
		}
		return 24
	case "certificate_expiry_warning_days":
		if c.Thresholds.CertificateExpiryWarning > 0 {
			return c.Thresholds.CertificateExpiryWarning
		}
		return 30
	case "node_cpu_warning_percent":
		if c.Thresholds.NodeCPUWarning > 0 {
			return c.Thresholds.NodeCPUWarning
		}
		return 80
	case "node_memory_warning_percent":
		if c.Thresholds.NodeMemoryWarning > 0 {
			return c.Thresholds.NodeMemoryWarning
		}
		return 80
	case "node_memory_critical_percent":
		if c.Thresholds.NodeMemoryCritical > 0 {
			return c.Thresholds.NodeMemoryCritical
		}
		return 95
	default:
		return 0
	}
}

func SaveExample(path string) error {
	example := `# Cluster Probe Configuration
# Place this file at .probe/config.yaml

# Disable specific checks
checks:
  # dns-resolution:
  #   enabled: false
  # network-policies:
  #   enabled: false

# Ignore patterns
ignore:
  # Namespaces to ignore (no issues reported from these)
  namespaces: []
    # - kube-system
    # - monitoring

  # Checks to skip entirely
  checks: []
    # - dns-resolution

# Thresholds (adjust sensitivity)
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
`

	return os.WriteFile(path, []byte(example), 0644)
}
