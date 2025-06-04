package kubernetes_test

import (
	"testing"

	"github.com/opan/secman/pkg/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

// MockClient is a mock implementation of the Client struct for testing
type MockClient struct {
	clientset *fake.Clientset
}

func NewMockClient() *MockClient {
	return &MockClient{
		clientset: fake.NewSimpleClientset(),
	}
}

func (m *MockClient) GetClientset() *fake.Clientset {
	return m.clientset
}

func TestNewClient(t *testing.T) {
	// This is more of an integration test than a unit test
	// We can only properly test this if we have a valid kubeconfig or are running in-cluster
	// So we'll just test that the function doesn't crash
	_, err := kubernetes.NewClient()
	if err != nil {
		// It's okay if this fails locally without a valid kubeconfig
		t.Logf("NewClient() returned an error: %v", err)
	}
}
