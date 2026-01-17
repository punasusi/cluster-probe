package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	if len(cfg.Checks) != 20 {
		t.Errorf("expected 20 default checks, got %d", len(cfg.Checks))
	}

	for name, check := range cfg.Checks {
		if check.Enabled == nil || !*check.Enabled {
			t.Errorf("check %s should be enabled by default", name)
		}
	}

	if cfg.Thresholds.DefaultServiceAccountPods != 10 {
		t.Errorf("expected DefaultServiceAccountPods=10, got %d", cfg.Thresholds.DefaultServiceAccountPods)
	}
}

func TestLoadConfigNonExistent(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/path/config.yaml")
	if err != nil {
		t.Errorf("unexpected error for nonexistent file: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected default config for nonexistent file")
	}
}

func TestLoadConfigValid(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	content := `
checks:
  dns-resolution:
    enabled: false
ignore:
  namespaces:
    - kube-system
thresholds:
  default_service_account_pods: 20
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Checks["dns-resolution"].Enabled == nil || *cfg.Checks["dns-resolution"].Enabled {
		t.Error("dns-resolution should be disabled")
	}

	if len(cfg.Ignore.Namespaces) != 1 || cfg.Ignore.Namespaces[0] != "kube-system" {
		t.Errorf("unexpected ignore namespaces: %v", cfg.Ignore.Namespaces)
	}

	if cfg.Thresholds.DefaultServiceAccountPods != 20 {
		t.Errorf("expected threshold 20, got %d", cfg.Thresholds.DefaultServiceAccountPods)
	}
}

func TestLoadConfigInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestIsCheckEnabled(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.IsCheckEnabled("node-status") {
		t.Error("node-status should be enabled by default")
	}

	disabled := false
	cfg.Checks["test-check"] = CheckConfig{Enabled: &disabled}
	if cfg.IsCheckEnabled("test-check") {
		t.Error("test-check should be disabled")
	}

	cfg.Ignore.Checks = []string{"ignored-check"}
	if cfg.IsCheckEnabled("ignored-check") {
		t.Error("ignored-check should be disabled via ignore list")
	}

	if !cfg.IsCheckEnabled("unknown-check") {
		t.Error("unknown check should be enabled by default")
	}
}

func TestIsNamespaceIgnored(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.IsNamespaceIgnored("default") {
		t.Error("default namespace should not be ignored initially")
	}

	cfg.Ignore.Namespaces = []string{"kube-system", "monitoring"}

	if !cfg.IsNamespaceIgnored("kube-system") {
		t.Error("kube-system should be ignored")
	}
	if !cfg.IsNamespaceIgnored("monitoring") {
		t.Error("monitoring should be ignored")
	}
	if cfg.IsNamespaceIgnored("default") {
		t.Error("default should not be ignored")
	}
}

func TestGetThreshold(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		name     string
		expected int
	}{
		{"default_service_account_pods", 10},
		{"pending_pod_age_minutes", 30},
		{"job_running_age_hours", 24},
		{"certificate_expiry_warning_days", 30},
		{"node_cpu_warning_percent", 80},
		{"node_memory_warning_percent", 80},
		{"node_memory_critical_percent", 95},
		{"unknown_threshold", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfg.GetThreshold(tt.name)
			if got != tt.expected {
				t.Errorf("GetThreshold(%s) = %d, want %d", tt.name, got, tt.expected)
			}
		})
	}
}

func TestGetThresholdCustom(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Thresholds.DefaultServiceAccountPods = 50

	got := cfg.GetThreshold("default_service_account_pods")
	if got != 50 {
		t.Errorf("expected custom threshold 50, got %d", got)
	}
}

func TestSaveExample(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := SaveExample(configPath); err != nil {
		t.Fatalf("SaveExample failed: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	if len(data) == 0 {
		t.Error("saved config is empty")
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Errorf("saved config should be valid YAML: %v", err)
	}
	if cfg == nil {
		t.Error("should load config from example")
	}
}
