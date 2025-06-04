// Package agent implements the secman agent functionality
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/opan/secman/pkg/api"
	"github.com/opan/secman/pkg/kubernetes"
	"github.com/opan/secman/pkg/secrets"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

// Agent represents the secman agent
type Agent struct {
	ID            string
	Name          string
	ServerURL     string
	conn          *websocket.Conn
	kubeClient    kubernetes.ClientInterface
	secretManager *secrets.Manager
	connMutex     sync.Mutex
	isConnected   bool
	stopCh        chan struct{}
	reqCh         chan []byte
	respHandlers  map[string]ResponseHandler
	handlersMutex sync.RWMutex
	config        *Config
}

// Config holds agent configuration
type Config struct {
	Name           string
	ServerURL      string
	KubeconfigPath string
}

// ResponseHandler is a function that handles a response from the server
type ResponseHandler func(response []byte) error

// New creates a new agent instance
func New(config *Config) (*Agent, error) {
	// Generate a unique agent ID
	agentID := uuid.New().String()

	// Create Kubernetes client
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", config.KubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes config: %w", err)
	}

	kubeClient, err := kubernetes.NewClient(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Create secret manager
	secretManager := secrets.NewManager(kubeClient)

	agent := &Agent{
		ID:            agentID,
		Name:          config.Name,
		ServerURL:     config.ServerURL,
		kubeClient:    kubeClient,
		secretManager: secretManager,
		stopCh:        make(chan struct{}),
		reqCh:         make(chan []byte, 100),
		respHandlers:  make(map[string]ResponseHandler),
		config:        config,
	}

	return agent, nil
}

// Start starts the agent
func (a *Agent) Start(ctx context.Context) error {
	log.Printf("Starting agent %s", a.Name)

	// Connect to server
	if err := a.connect(); err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}

	// Start goroutines for communication
	go a.readPump(ctx)
	go a.writePump(ctx)
	go a.reconnectLoop(ctx)

	return nil
}

// Stop stops the agent
func (a *Agent) Stop() error {
	log.Printf("Stopping agent %s", a.Name)

	close(a.stopCh)

	a.connMutex.Lock()
	defer a.connMutex.Unlock()

	if a.conn != nil {
		a.conn.Close()
		a.isConnected = false
	}

	return nil
}

// connect establishes a WebSocket connection to the server
func (a *Agent) connect() error {
	a.connMutex.Lock()
	defer a.connMutex.Unlock()

	// Close existing connection
	if a.conn != nil {
		a.conn.Close()
		a.isConnected = false
	}

	// Parse server URL
	u, err := url.Parse(a.ServerURL)
	if err != nil {
		return err
	}

	// Create WebSocket URL
	wsScheme := "ws"
	if u.Scheme == "https" {
		wsScheme = "wss"
	}
	wsURL := fmt.Sprintf("%s://%s/api/connect", wsScheme, u.Host)

	// Connect to WebSocket server
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}

	a.conn = conn
	a.isConnected = true

	// Register with server
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown-host"
	}

	if a.Name == "" {
		a.Name = hostname
	}

	// Collect metadata about the cluster
	clusterInfo, err := a.kubeClient.GetClientset().Discovery().ServerVersion()
	if err != nil {
		log.Printf("Warning: Failed to get Kubernetes version: %v", err)
	}

	// Count nodes in the cluster
	nodeCount := 0
	if nodes, err := a.kubeClient.GetClientset().CoreV1().Nodes().List(context.Background(), metav1.ListOptions{}); err == nil {
		nodeCount = len(nodes.Items)
	}

	// Create registration request
	regReq := api.RegistrationRequest{
		Name: a.Name,
		Metadata: api.AgentMetadata{
			KubernetesVersion: clusterInfo.GitVersion,
			NodeCount:         nodeCount,
			Labels:            map[string]string{"hostname": hostname},
		},
	}

	// Send registration request
	if err := conn.WriteJSON(regReq); err != nil {
		conn.Close()
		a.isConnected = false
		return fmt.Errorf("failed to send registration request: %w", err)
	}

	// Read registration response
	var regResp api.RegistrationResponse
	if err := conn.ReadJSON(&regResp); err != nil {
		conn.Close()
		a.isConnected = false
		return fmt.Errorf("failed to read registration response: %w", err)
	}

	if !regResp.Success {
		conn.Close()
		a.isConnected = false
		return fmt.Errorf("registration failed: %s", regResp.Error)
	}

	// Update agent ID from response
	a.ID = regResp.AgentID
	log.Printf("Agent registered with server (ID: %s)", a.ID)

	return nil
}

// reconnectLoop attempts to reconnect to the server if connection is lost
func (a *Agent) reconnectLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.connMutex.Lock()
			isConnected := a.isConnected
			a.connMutex.Unlock()

			if !isConnected {
				log.Println("Attempting to reconnect to server...")
				if err := a.connect(); err != nil {
					log.Printf("Failed to reconnect: %v", err)
				} else {
					log.Println("Reconnected to server")
				}
			}
		}
	}
}

// readPump processes incoming messages from the server
func (a *Agent) readPump(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		default:
			a.connMutex.Lock()
			conn := a.conn
			isConnected := a.isConnected
			a.connMutex.Unlock()

			if !isConnected || conn == nil {
				time.Sleep(1 * time.Second)
				continue
			}

			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("Error reading from server: %v", err)

				a.connMutex.Lock()
				a.conn.Close()
				a.isConnected = false
				a.connMutex.Unlock()

				break
			}

			go a.handleServerMessage(message)
		}
	}
}

// writePump sends messages to the server
func (a *Agent) writePump(ctx context.Context) {
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case req := <-a.reqCh:
			a.connMutex.Lock()
			conn := a.conn
			isConnected := a.isConnected
			a.connMutex.Unlock()

			if !isConnected || conn == nil {
				log.Printf("Cannot send message: not connected to server")
				continue
			}

			if err := conn.WriteMessage(websocket.TextMessage, req); err != nil {
				log.Printf("Error sending message to server: %v", err)

				a.connMutex.Lock()
				a.conn.Close()
				a.isConnected = false
				a.connMutex.Unlock()
			}
		case <-heartbeatTicker.C:
			a.sendHeartbeat()
		}
	}
}

// sendHeartbeat sends a heartbeat to the server
func (a *Agent) sendHeartbeat() {
	a.connMutex.Lock()
	conn := a.conn
	isConnected := a.isConnected
	a.connMutex.Unlock()

	if !isConnected || conn == nil {
		return
	}

	heartbeat := api.HeartbeatRequest{
		AgentID: a.ID,
	}

	data, err := json.Marshal(heartbeat)
	if err != nil {
		log.Printf("Error marshaling heartbeat: %v", err)
		return
	}

	message := api.Message{
		Type:    api.MessageTypeHeartbeat,
		Payload: data,
	}

	msgData, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}

	if err := conn.WriteMessage(websocket.TextMessage, msgData); err != nil {
		log.Printf("Error sending heartbeat: %v", err)

		a.connMutex.Lock()
		a.conn.Close()
		a.isConnected = false
		a.connMutex.Unlock()
	}
}

// handleServerMessage processes messages received from the server
func (a *Agent) handleServerMessage(rawMessage []byte) {
	var message api.Message
	if err := json.Unmarshal(rawMessage, &message); err != nil {
		log.Printf("Error unmarshaling message: %v", err)
		return
	}

	switch message.Type {
	case api.MessageTypeSecretRequest:
		a.handleSecretRequest(message.Payload)
	case api.MessageTypeHeartbeat:
		// Process heartbeat response
		var resp api.HeartbeatResponse
		if err := json.Unmarshal(message.Payload, &resp); err != nil {
			log.Printf("Error unmarshaling heartbeat response: %v", err)
			return
		}

		// Heartbeat handling
		if !resp.Success {
			log.Printf("Server reported error in heartbeat: %s", resp.Error)
		}

	default:
		// Check if there's a response handler for this message ID
		if message.ID != "" {
			a.handlersMutex.RLock()
			handler, exists := a.respHandlers[message.ID]
			a.handlersMutex.RUnlock()

			if exists {
				if err := handler(message.Payload); err != nil {
					log.Printf("Error handling response (ID: %s): %v", message.ID, err)
				}

				// Remove the handler after use
				a.handlersMutex.Lock()
				delete(a.respHandlers, message.ID)
				a.handlersMutex.Unlock()
			}
		}
	}
}

// handleSecretRequest processes a secret request from the server
func (a *Agent) handleSecretRequest(rawRequest []byte) {
	var req api.SecretRequest
	if err := json.Unmarshal(rawRequest, &req); err != nil {
		log.Printf("Error unmarshaling secret request: %v", err)
		return
	}

	var resp api.SecretResponse
	var err error

	switch req.Operation {
	case "get":
		var secret *corev1.Secret
		secret, err = a.secretManager.GetSecret(context.Background(), req.Namespace, req.Name)
		if err == nil {
			resp.Secret = secret
			resp.Success = true
		}
	case "list":
		var secrets []corev1.Secret
		secrets, err = a.secretManager.ListSecrets(context.Background(), req.Namespace)
		if err == nil {
			resp.Secrets = secrets
			resp.Success = true
		}
	case "create":
		err = a.secretManager.CreateSecret(context.Background(), req.Namespace, req.Name, req.Data)
		if err == nil {
			resp.Success = true
		}
	case "update":
		err = a.secretManager.UpdateSecret(context.Background(), req.Namespace, req.Name, req.Data)
		if err == nil {
			resp.Success = true
		}
	case "delete":
		err = a.secretManager.DeleteSecret(context.Background(), req.Namespace, req.Name)
		if err == nil {
			resp.Success = true
		}
	default:
		err = fmt.Errorf("unsupported operation: %s", req.Operation)
	}

	if err != nil {
		resp.Error = err.Error()
		resp.Success = false
	}

	// Send response back to server
	respData, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Error marshaling response: %v", err)
		return
	}

	message := api.Message{
		Type:    api.MessageTypeSecretResponse,
		Payload: respData,
	}

	msgData, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}

	a.reqCh <- msgData
}
