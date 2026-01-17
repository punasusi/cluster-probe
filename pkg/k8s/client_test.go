package k8s

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewClient_InvalidPath(t *testing.T) {
	_, err := NewClient("/nonexistent/path/config")
	if err == nil {
		t.Error("expected error for nonexistent kubeconfig")
	}
}

func TestNewClient_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")
	if err := os.WriteFile(configPath, []byte("invalid: yaml: ["), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := NewClient(configPath)
	if err == nil {
		t.Error("expected error for invalid kubeconfig YAML")
	}
}

func TestNewClient_EmptyConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")
	if err := os.WriteFile(configPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := NewClient(configPath)
	if err == nil {
		t.Error("expected error for empty kubeconfig")
	}
}

func TestNewClient_MissingCluster(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")
	content := `apiVersion: v1
kind: Config
current-context: test
contexts:
- name: test
  context:
    cluster: nonexistent
    user: test-user
users:
- name: test-user
  user:
    token: test-token
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := NewClient(configPath)
	if err == nil {
		t.Error("expected error for missing cluster in kubeconfig")
	}
}
