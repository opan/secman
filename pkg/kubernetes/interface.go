package kubernetes

import (
	"k8s.io/client-go/kubernetes"
)

// ClientInterface defines the interface for kubernetes client operations
type ClientInterface interface {
	GetClientset() kubernetes.Interface
}

// Ensure Client implements ClientInterface
var _ ClientInterface = (*Client)(nil)
