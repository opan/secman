package kubernetes

import (
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client represents a Kubernetes client
type Client struct {
	clientset *kubernetes.Clientset
}

// NewClient creates a new Kubernetes client
func NewClient() (*Client, error) {
	config, err := getConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &Client{
		clientset: clientset,
	}, nil
}

// getConfig returns a Kubernetes client config
func getConfig() (*rest.Config, error) {
	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	// Fall back to kubeconfig
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	if envvar := os.Getenv("KUBECONFIG"); envvar != "" {
		kubeconfig = envvar
	}

	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

// GetClientset returns the underlying Kubernetes clientset
func (c *Client) GetClientset() kubernetes.Interface {
	return c.clientset
}
