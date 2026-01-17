package k8s

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Client struct {
	clientset	*kubernetes.Clientset
	config		clientcmd.ClientConfig
}

func NewClient(kubeconfigPath string) (*Client, error) {
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	configOverrides := &clientcmd.ConfigOverrides{}
	config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	restConfig, err := config.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &Client{
		clientset:	clientset,
		config:		config,
	}, nil
}

func (c *Client) Clientset() kubernetes.Interface {
	return c.clientset
}

func (c *Client) TestConnection(ctx context.Context) error {
	_, err := c.clientset.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("failed to connect to cluster: %w", err)
	}
	return nil
}

func (c *Client) ClusterInfo(ctx context.Context) (string, error) {
	rawConfig, err := c.config.RawConfig()
	if err != nil {
		return "", err
	}

	currentContext := rawConfig.CurrentContext
	if currentContext == "" {
		return "unknown", nil
	}

	contextConfig, exists := rawConfig.Contexts[currentContext]
	if !exists {
		return currentContext, nil
	}

	version, err := c.clientset.Discovery().ServerVersion()
	if err != nil {
		return contextConfig.Cluster, nil
	}

	return fmt.Sprintf("%s (v%s)", contextConfig.Cluster, version.GitVersion), nil
}
