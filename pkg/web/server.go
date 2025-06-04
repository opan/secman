package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/opan/secman/pkg/api"
	"github.com/opan/secman/pkg/storage"
)

// Server represents the SecMan server
type Server struct {
	ID          string
	router      *gin.Engine
	agents      map[string]*agentConnection
	agentsMutex sync.RWMutex
	upgrader    websocket.Upgrader
	storage     storage.Storage
	config      *Config
}

// Config holds server configuration
type Config struct {
	Host        string
	Port        int
	UIDir       string
	StoragePath string
}

// agentConnection represents a connected agent
type agentConnection struct {
	Agent      api.Agent
	Connection *websocket.Conn
	LastSeen   time.Time
	SendChan   chan []byte
}

// NewServer creates a new server instance
func NewServer(config *Config) (*Server, error) {
	// Generate a unique server ID
	serverID := uuid.New().String()

	// Initialize storage
	store, err := storage.NewFileStorage(config.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Create server instance
	server := &Server{
		ID:      serverID,
		router:  gin.Default(),
		agents:  make(map[string]*agentConnection),
		storage: store,
		config:  config,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all connections
			},
		},
	}

	// Set up routes
	server.setupRoutes()

	return server, nil
}

// setupRoutes initializes the HTTP routes
func (s *Server) setupRoutes() {
	// API routes
	api := s.router.Group("/api")
	{
		api.GET("/agents", s.listAgents)
		api.GET("/connect", s.handleAgentConnection)

		// Secret management routes that will proxy to agents
		secrets := api.Group("/secrets")
		{
			secrets.GET("/:agent_id/:namespace", s.listSecrets)
			secrets.GET("/:agent_id/:namespace/:name", s.getSecret)
			secrets.POST("/:agent_id/:namespace", s.createSecret)
			secrets.PUT("/:agent_id/:namespace/:name", s.updateSecret)
			secrets.DELETE("/:agent_id/:namespace/:name", s.deleteSecret)
		}
	}

	// Serve static UI files
	s.router.Use(static.Serve("/", static.LocalFile(s.config.UIDir, false)))

	// For any other routes, serve the index.html for SPA
	s.router.NoRoute(func(c *gin.Context) {
		c.File(s.config.UIDir + "/index.html")
	})
}

// Start starts the server
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	log.Printf("Starting SecMan server on %s", addr)

	// Start a goroutine to monitor agent connections
	go s.monitorAgents()

	// Start the HTTP server
	return s.router.Run(addr)
}

// Stop stops the server and closes all agent connections
func (s *Server) Stop(ctx context.Context) error {
	// Close all agent connections
	s.agentsMutex.Lock()
	defer s.agentsMutex.Unlock()

	for _, agent := range s.agents {
		if agent.Connection != nil {
			agent.Connection.Close()
		}
	}

	return nil
}

// monitorAgents periodically checks agent connections and updates status
func (s *Server) monitorAgents() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.agentsMutex.Lock()
		for id, agent := range s.agents {
			// If agent hasn't been seen in 1 minute, mark as offline
			if time.Since(agent.LastSeen) > time.Minute {
				agent.Agent.Status = api.AgentStatusOffline
				log.Printf("Agent %s (%s) is now offline", agent.Agent.Name, id)
			}
		}
		s.agentsMutex.Unlock()
	}
}

// handleAgentConnection handles WebSocket connections from agents
func (s *Server) handleAgentConnection(c *gin.Context) {
	// Upgrade HTTP connection to WebSocket
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upgrade connection"})
		return
	}

	// Read registration request
	var regReq api.RegistrationRequest
	if err := conn.ReadJSON(&regReq); err != nil {
		log.Printf("Error reading registration request: %v", err)
		conn.Close()
		return
	}

	// Generate agent ID
	agentID := uuid.New().String()

	// Create agent
	agent := api.Agent{
		ID:        agentID,
		Name:      regReq.Name,
		ClusterID: agentID, // Use agent ID as cluster ID for now
		Status:    api.AgentStatusOnline,
		LastSeen:  time.Now(),
		Metadata:  regReq.Metadata,
	}

	// Send registration response
	regResp := api.RegistrationResponse{
		Success:  true,
		AgentID:  agentID,
		ServerID: s.ID,
	}

	if err := conn.WriteJSON(regResp); err != nil {
		log.Printf("Error sending registration response: %v", err)
		conn.Close()
		return
	}

	// Create agent connection
	agentConn := &agentConnection{
		Agent:      agent,
		Connection: conn,
		LastSeen:   time.Now(),
		SendChan:   make(chan []byte, 100),
	}

	// Store agent
	s.agentsMutex.Lock()
	s.agents[agentID] = agentConn
	s.agentsMutex.Unlock()

	log.Printf("Agent %s (%s) connected", agent.Name, agentID)

	// Start read/write goroutines
	go s.readPump(agentConn)
	go s.writePump(agentConn)
}

// readPump handles incoming messages from an agent
func (s *Server) readPump(agent *agentConnection) {
	defer func() {
		agent.Connection.Close()
		// Remove agent when connection closes
		s.agentsMutex.Lock()
		delete(s.agents, agent.Agent.ID)
		s.agentsMutex.Unlock()
		log.Printf("Agent %s (%s) disconnected", agent.Agent.Name, agent.Agent.ID)
	}()

	agent.Connection.SetReadLimit(4096) // 4KB max message size

	for {
		_, message, err := agent.Connection.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Agent connection error: %v", err)
			}
			break
		}

		// Update last seen time
		agent.LastSeen = time.Now()
		agent.Agent.Status = api.AgentStatusOnline

		// Process message (could be heartbeat, response, etc)
		s.handleAgentMessage(agent, message)
	}
}

// writePump sends messages to the agent
func (s *Server) writePump(agent *agentConnection) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		agent.Connection.Close()
	}()

	for {
		select {
		case message, ok := <-agent.SendChan:
			if !ok {
				// Channel closed
				agent.Connection.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := agent.Connection.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("Error writing to agent: %v", err)
				return
			}

		case <-ticker.C:
			// Send ping to keep connection alive
			if err := agent.Connection.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleAgentMessage processes messages received from agents
func (s *Server) handleAgentMessage(agent *agentConnection, rawMessage []byte) {
	var message api.Message
	if err := json.Unmarshal(rawMessage, &message); err != nil {
		log.Printf("Error unmarshaling message: %v", err)
		return
	}

	switch message.Type {
	case api.MessageTypeHeartbeat:
		var req api.HeartbeatRequest
		if err := json.Unmarshal(message.Payload, &req); err != nil {
			log.Printf("Error unmarshaling heartbeat request: %v", err)
			return
		}

		// Send heartbeat response
		resp := api.HeartbeatResponse{
			Success:  true,
			ServerID: s.ID,
		}

		respData, err := json.Marshal(resp)
		if err != nil {
			log.Printf("Error marshaling heartbeat response: %v", err)
			return
		}

		responseMsg := api.Message{
			Type:    api.MessageTypeHeartbeat,
			Payload: respData,
		}

		msgData, err := json.Marshal(responseMsg)
		if err != nil {
			log.Printf("Error marshaling message: %v", err)
			return
		}

		agent.SendChan <- msgData

	case api.MessageTypeSecretResponse:
		var resp api.SecretResponse
		if err := json.Unmarshal(message.Payload, &resp); err != nil {
			log.Printf("Error unmarshaling secret response: %v", err)
			return
		}

		// Store the response in a cache or forward to the appropriate handler
		// This would be implemented based on the request ID
	}
}

// listAgents returns the list of all agents
func (s *Server) listAgents(c *gin.Context) {
	s.agentsMutex.RLock()
	defer s.agentsMutex.RUnlock()

	agents := make([]api.Agent, 0, len(s.agents))
	for _, agent := range s.agents {
		agents = append(agents, agent.Agent)
	}

	c.JSON(http.StatusOK, gin.H{"agents": agents})
}

// Secret management handlers
// These functions forward requests to the appropriate agent

func (s *Server) listSecrets(c *gin.Context) {
	agentID := c.Param("agent_id")
	namespace := c.Param("namespace")

	agentConn, ok := s.getAgent(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	// Create secret request
	req := api.SecretRequest{
		Operation: "list",
		Namespace: namespace,
	}

	// Convert request to JSON
	reqData, err := json.Marshal(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Create message with unique ID
	msgID := uuid.New().String()
	message := api.Message{
		ID:      msgID,
		Type:    api.MessageTypeSecretRequest,
		Payload: reqData,
	}

	// Convert message to JSON
	msgData, err := json.Marshal(message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Send message to agent
	agentConn.SendChan <- msgData

	// For now, return a placeholder response
	// In a real implementation, we would set up a response channel and wait for the response
	c.JSON(http.StatusOK, gin.H{"message": "Request sent to agent", "request_id": msgID})
}

func (s *Server) getSecret(c *gin.Context) {
	agentID := c.Param("agent_id")
	namespace := c.Param("namespace")
	name := c.Param("name")

	agentConn, ok := s.getAgent(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	// Create secret request
	req := api.SecretRequest{
		Operation: "get",
		Namespace: namespace,
		Name:      name,
	}

	// Convert request to JSON
	reqData, err := json.Marshal(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Create message with unique ID
	msgID := uuid.New().String()
	message := api.Message{
		ID:      msgID,
		Type:    api.MessageTypeSecretRequest,
		Payload: reqData,
	}

	// Convert message to JSON
	msgData, err := json.Marshal(message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Send message to agent
	agentConn.SendChan <- msgData

	// For now, return a placeholder response
	// In a real implementation, we would set up a response channel and wait for the response
	c.JSON(http.StatusOK, gin.H{"message": "Request sent to agent", "request_id": msgID})
}

func (s *Server) createSecret(c *gin.Context) {
	agentID := c.Param("agent_id")
	namespace := c.Param("namespace")

	var reqBody struct {
		Name string            `json:"name"`
		Data map[string]string `json:"data"`
	}

	if err := c.BindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	agentConn, ok := s.getAgent(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	// Create secret request
	req := api.SecretRequest{
		Operation: "create",
		Namespace: namespace,
		Name:      reqBody.Name,
		Data:      make(map[string][]byte),
	}

	// Convert string data to bytes
	for k, v := range reqBody.Data {
		req.Data[k] = []byte(v)
	}

	// Convert request to JSON
	reqData, err := json.Marshal(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Create message with unique ID
	msgID := uuid.New().String()
	message := api.Message{
		ID:      msgID,
		Type:    api.MessageTypeSecretRequest,
		Payload: reqData,
	}

	// Convert message to JSON
	msgData, err := json.Marshal(message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Send message to agent
	agentConn.SendChan <- msgData

	// For now, return a placeholder response
	// In a real implementation, we would set up a response channel and wait for the response
	c.JSON(http.StatusOK, gin.H{"message": "Request sent to agent", "request_id": msgID})
}

func (s *Server) updateSecret(c *gin.Context) {
	agentID := c.Param("agent_id")
	namespace := c.Param("namespace")
	name := c.Param("name")

	var reqBody struct {
		Data map[string]string `json:"data"`
	}

	if err := c.BindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	agentConn, ok := s.getAgent(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	// Create secret request
	req := api.SecretRequest{
		Operation: "update",
		Namespace: namespace,
		Name:      name,
		Data:      make(map[string][]byte),
	}

	// Convert string data to bytes
	for k, v := range reqBody.Data {
		req.Data[k] = []byte(v)
	}

	// Convert request to JSON
	reqData, err := json.Marshal(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Create message with unique ID
	msgID := uuid.New().String()
	message := api.Message{
		ID:      msgID,
		Type:    api.MessageTypeSecretRequest,
		Payload: reqData,
	}

	// Convert message to JSON
	msgData, err := json.Marshal(message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Send message to agent
	agentConn.SendChan <- msgData

	// For now, return a placeholder response
	// In a real implementation, we would set up a response channel and wait for the response
	c.JSON(http.StatusOK, gin.H{"message": "Request sent to agent", "request_id": msgID})
}

func (s *Server) deleteSecret(c *gin.Context) {
	agentID := c.Param("agent_id")
	namespace := c.Param("namespace")
	name := c.Param("name")

	agentConn, ok := s.getAgent(agentID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	// Create secret request
	req := api.SecretRequest{
		Operation: "delete",
		Namespace: namespace,
		Name:      name,
	}

	// Convert request to JSON
	reqData, err := json.Marshal(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Create message with unique ID
	msgID := uuid.New().String()
	message := api.Message{
		ID:      msgID,
		Type:    api.MessageTypeSecretRequest,
		Payload: reqData,
	}

	// Convert message to JSON
	msgData, err := json.Marshal(message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Send message to agent
	agentConn.SendChan <- msgData

	// For now, return a placeholder response
	// In a real implementation, we would set up a response channel and wait for the response
	c.JSON(http.StatusOK, gin.H{"message": "Request sent to agent", "request_id": msgID})
}

// getAgent returns an agent by ID if it exists
func (s *Server) getAgent(agentID string) (*agentConnection, bool) {
	s.agentsMutex.RLock()
	defer s.agentsMutex.RUnlock()

	agent, ok := s.agents[agentID]
	return agent, ok
}
