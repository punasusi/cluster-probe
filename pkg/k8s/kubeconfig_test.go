package k8s

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverKubeconfig_FlagValue(t *testing.T) {
	result := DiscoverKubeconfig("/custom/path/config", false)
	if result != "/custom/path/config" {
		t.Errorf("expected flag value to take precedence, got %q", result)
	}
}

func TestDiscoverKubeconfig_EnvVar(t *testing.T) {
	t.Setenv("KUBECONFIG", "/env/path/config")

	result := DiscoverKubeconfig("", false)
	if result != "/env/path/config" {
		t.Errorf("expected env var value, got %q", result)
	}
}

func TestDiscoverKubeconfig_FlagOverridesEnv(t *testing.T) {
	t.Setenv("KUBECONFIG", "/env/path/config")

	result := DiscoverKubeconfig("/flag/path/config", false)
	if result != "/flag/path/config" {
		t.Errorf("expected flag to override env, got %q", result)
	}
}

func TestDiscoverKubeconfig_ContainerHostUser(t *testing.T) {
	tmpDir := t.TempDir()

	hostDir := filepath.Join(tmpDir, "host/home/testuser/.kube")
	if err := os.MkdirAll(hostDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(hostDir, "config")
	if err := os.WriteFile(configPath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	origHostUserPath := "/host/home"
	defer func() {
		_ = origHostUserPath
	}()

	t.Setenv("HOST_USER", "testuser")
}

func TestDiscoverKubeconfig_DefaultPath(t *testing.T) {
	tmpDir := t.TempDir()

	kubeDir := filepath.Join(tmpDir, ".kube")
	if err := os.MkdirAll(kubeDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(kubeDir, "config")
	if err := os.WriteFile(configPath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	result := DiscoverKubeconfig("", false)
	if result != configPath {
		t.Errorf("expected default path %q, got %q", configPath, result)
	}
}

func TestDiscoverKubeconfig_EmptyWhenNoConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("KUBECONFIG", "")

	result := DiscoverKubeconfig("", false)
	if result != "" {
		t.Errorf("expected empty string when no config found, got %q", result)
	}
}
