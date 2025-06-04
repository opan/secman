package api

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// AgentStatus represents the current status of an agent
type AgentStatus string

const (
	// AgentStatusOnline indicates the agent is online and connected
	AgentStatusOnline AgentStatus = "online"
	// AgentStatusOffline indicates the agent is offline
	AgentStatusOffline AgentStatus = "offline"
)

// Agent represents a Kubernetes cluster agent
type Agent struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	ClusterID string        `json:"cluster_id"`
	Status    AgentStatus   `json:"status"`
	LastSeen  time.Time     `json:"last_seen"`
	Metadata  AgentMetadata `json:"metadata"`
}

// AgentMetadata contains additional information about the agent's cluster
type AgentMetadata struct {
	KubernetesVersion string            `json:"kubernetes_version"`
	NodeCount         int               `json:"node_count"`
	Labels            map[string]string `json:"labels"`
}

// SecretRequest represents a request to perform an operation on a secret
type SecretRequest struct {
	Operation string            `json:"operation"` // "create", "get", "update", "delete", "list"
	Namespace string            `json:"namespace"`
	Name      string            `json:"name,omitempty"`
	Data      map[string][]byte `json:"data,omitempty"`
	Key       string            `json:"key,omitempty"`
}

// SecretResponse represents a response from a secret operation
type SecretResponse struct {
	Success bool            `json:"success"`
	Error   string          `json:"error,omitempty"`
	Secret  *corev1.Secret  `json:"secret,omitempty"`
	Secrets []corev1.Secret `json:"secrets,omitempty"`
}

// RegistrationRequest is sent by an agent to register with the server
type RegistrationRequest struct {
	Name     string        `json:"name"`
	Metadata AgentMetadata `json:"metadata"`
}

// RegistrationResponse is sent by the server in response to a registration request
type RegistrationResponse struct {
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
	AgentID  string `json:"agent_id"`
	ServerID string `json:"server_id"`
}

// HeartbeatRequest is sent by an agent to indicate it's still alive
type HeartbeatRequest struct {
	AgentID string `json:"agent_id"`
}

// HeartbeatResponse is sent by the server in response to a heartbeat
type HeartbeatResponse struct {
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
	ServerID string `json:"server_id"`
}

// MessageType defines the type of a message
type MessageType string

const (
	// MessageTypeSecretRequest is a request to perform an operation on a secret
	MessageTypeSecretRequest MessageType = "secret_request"
	// MessageTypeSecretResponse is a response to a secret operation
	MessageTypeSecretResponse MessageType = "secret_response"
	// MessageTypeHeartbeat is a heartbeat message
	MessageTypeHeartbeat MessageType = "heartbeat"
	// MessageTypeRegistration is a registration message
	MessageTypeRegistration MessageType = "registration"
)

// Message is the wrapper for all messages between server and agent
type Message struct {
	ID      string      `json:"id,omitempty"`
	Type    MessageType `json:"type"`
	Payload []byte      `json:"payload"`
}
