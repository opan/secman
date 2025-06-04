package secrets

import (
	"context"
	"fmt"

	"github.com/opan/secman/pkg/kubernetes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Manager handles operations on Kubernetes secrets
type Manager struct {
	client kubernetes.ClientInterface
}

// NewManager creates a new secret manager
func NewManager(client kubernetes.ClientInterface) *Manager {
	return &Manager{
		client: client,
	}
}

// CreateSecret creates a new Kubernetes secret
func (m *Manager) CreateSecret(ctx context.Context, namespace, name string, data map[string][]byte) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	return m.client.GetClientset().CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
}

// GetSecret retrieves a Kubernetes secret
func (m *Manager) GetSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	return m.client.GetClientset().CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
}

// UpdateSecret updates an existing Kubernetes secret
func (m *Manager) UpdateSecret(ctx context.Context, namespace string, secret *corev1.Secret) (*corev1.Secret, error) {
	return m.client.GetClientset().CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
}

// DeleteSecret deletes a Kubernetes secret
func (m *Manager) DeleteSecret(ctx context.Context, namespace, name string) error {
	return m.client.GetClientset().CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ListSecrets lists all secrets in a namespace
func (m *Manager) ListSecrets(ctx context.Context, namespace string) (*corev1.SecretList, error) {
	return m.client.GetClientset().CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
}

// AddSecretData adds or updates a key-value pair in a secret
func (m *Manager) AddSecretData(ctx context.Context, namespace, name, key string, value []byte) (*corev1.Secret, error) {
	secret, err := m.GetSecret(ctx, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data[key] = value

	return m.UpdateSecret(ctx, namespace, secret)
}

// RemoveSecretData removes a key from a secret
func (m *Manager) RemoveSecretData(ctx context.Context, namespace, name, key string) (*corev1.Secret, error) {
	secret, err := m.GetSecret(ctx, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	if secret.Data == nil {
		return secret, nil
	}
	delete(secret.Data, key)

	return m.UpdateSecret(ctx, namespace, secret)
}
