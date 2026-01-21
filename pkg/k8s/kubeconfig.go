package k8s

import (
	"os"
	"path/filepath"
)

func DiscoverKubeconfig(flagValue string, inContainer bool) string {
	if flagValue != "" {
		if inContainer && !hasPrefix(flagValue, "/host/") {
			return "/host" + flagValue
		}
		return flagValue
	}

	if env := os.Getenv("KUBECONFIG"); env != "" {
		return env
	}

	if inContainer {
		if user := os.Getenv("HOST_USER"); user != "" {
			hostPath := filepath.Join("/host/home", user, ".kube/config")
			if _, err := os.Stat(hostPath); err == nil {
				return hostPath
			}
		}
		if _, err := os.Stat("/host/root/.kube/config"); err == nil {
			return "/host/root/.kube/config"
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		defaultPath := filepath.Join(home, ".kube", "config")
		if _, err := os.Stat(defaultPath); err == nil {
			return defaultPath
		}
	}

	return ""
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
