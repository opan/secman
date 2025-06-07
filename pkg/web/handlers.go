package web

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/opan/secman/pkg/api"
)

// listAgents returns all connected agents
func (s *Server) listAgents(c *gin.Context) {
	s.agentsMu.RLock()
	defer s.agentsMu.RUnlock()

	agents := make([]*api.Agent, 0, len(s.agents))
	for _, conn := range s.agents {
		agents = append(agents, conn.Agent)
	}

	s.sendJSON(c, http.StatusOK, gin.H{
		"agents": agents,
		"count":  len(agents),
	})
}

// getAgent returns a specific agent by ID
func (s *Server) getAgent(c *gin.Context) {
	agentID := c.Param("id")

	s.agentsMu.RLock()
	conn, exists := s.agents[agentID]
	s.agentsMu.RUnlock()

	if !exists {
		s.sendError(c, http.StatusNotFound, "Agent not found")
		return
	}

	s.sendJSON(c, http.StatusOK, conn.Agent)
}

// disconnectAgent forcefully disconnects an agent
func (s *Server) disconnectAgent(c *gin.Context) {
	agentID := c.Param("id")

	s.agentsMu.Lock()
	conn, exists := s.agents[agentID]
	if exists {
		close(conn.Send)
		conn.Conn.Close()
		delete(s.agents, agentID)

		// Remove from storage
		if conn.Agent != nil {
			if err := s.storage.DeleteAgent(conn.Agent.ID); err != nil {
				log.Printf("Failed to delete agent from storage: %v", err)
			}
		}
	}
	s.agentsMu.Unlock()

	if !exists {
		s.sendError(c, http.StatusNotFound, "Agent not found")
		return
	}

	s.sendJSON(c, http.StatusOK, gin.H{"message": "Agent disconnected"})
}

// handleSecretOperation handles secret operations for a specific agent
func (s *Server) handleSecretOperation(c *gin.Context) {
	agentID := c.Param("id")

	s.agentsMu.RLock()
	conn, exists := s.agents[agentID]
	s.agentsMu.RUnlock()

	if !exists {
		s.sendError(c, http.StatusNotFound, "Agent not found")
		return
	}

	var secretReq api.SecretRequest
	if err := c.ShouldBindJSON(&secretReq); err != nil {
		s.sendError(c, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Create message to send to agent
	message := api.Message{
		ID:   uuid.New().String(),
		Type: api.MessageTypeSecretRequest,
	}

	payload, err := json.Marshal(secretReq)
	if err != nil {
		s.sendError(c, http.StatusInternalServerError, "Failed to marshal request")
		return
	}
	message.Payload = payload

	messageBytes, err := json.Marshal(message)
	if err != nil {
		s.sendError(c, http.StatusInternalServerError, "Failed to marshal message")
		return
	}

	// Send message to agent
	select {
	case conn.Send <- messageBytes:
		s.sendJSON(c, http.StatusAccepted, gin.H{
			"message":    "Request sent to agent",
			"message_id": message.ID,
		})
	default:
		s.sendError(c, http.StatusServiceUnavailable, "Agent connection is busy")
	}
}

// listSecrets lists all secrets in a namespace for a specific agent
func (s *Server) listSecrets(c *gin.Context) {
	agentID := c.Param("id")
	namespace := c.Query("namespace")

	if namespace == "" {
		namespace = "default"
	}

	s.agentsMu.RLock()
	conn, exists := s.agents[agentID]
	s.agentsMu.RUnlock()

	if !exists {
		s.sendError(c, http.StatusNotFound, "Agent not found")
		return
	}

	secretReq := api.SecretRequest{
		Operation: "list",
		Namespace: namespace,
	}

	// Create message to send to agent
	message := api.Message{
		ID:   uuid.New().String(),
		Type: api.MessageTypeSecretRequest,
	}

	payload, err := json.Marshal(secretReq)
	if err != nil {
		s.sendError(c, http.StatusInternalServerError, "Failed to marshal request")
		return
	}
	message.Payload = payload

	messageBytes, err := json.Marshal(message)
	if err != nil {
		s.sendError(c, http.StatusInternalServerError, "Failed to marshal message")
		return
	}

	// Send message to agent
	select {
	case conn.Send <- messageBytes:
		s.sendJSON(c, http.StatusAccepted, gin.H{
			"message":    "List request sent to agent",
			"message_id": message.ID,
		})
	default:
		s.sendError(c, http.StatusServiceUnavailable, "Agent connection is busy")
	}
}

// getSecret gets a specific secret from a namespace for a specific agent
func (s *Server) getSecret(c *gin.Context) {
	agentID := c.Param("id")
	namespace := c.Param("namespace")
	name := c.Param("name")

	s.agentsMu.RLock()
	conn, exists := s.agents[agentID]
	s.agentsMu.RUnlock()

	if !exists {
		s.sendError(c, http.StatusNotFound, "Agent not found")
		return
	}

	secretReq := api.SecretRequest{
		Operation: "get",
		Namespace: namespace,
		Name:      name,
	}

	// Create message to send to agent
	message := api.Message{
		ID:   uuid.New().String(),
		Type: api.MessageTypeSecretRequest,
	}

	payload, err := json.Marshal(secretReq)
	if err != nil {
		s.sendError(c, http.StatusInternalServerError, "Failed to marshal request")
		return
	}
	message.Payload = payload

	messageBytes, err := json.Marshal(message)
	if err != nil {
		s.sendError(c, http.StatusInternalServerError, "Failed to marshal message")
		return
	}

	// Send message to agent
	select {
	case conn.Send <- messageBytes:
		s.sendJSON(c, http.StatusAccepted, gin.H{
			"message":    "Get request sent to agent",
			"message_id": message.ID,
		})
	default:
		s.sendError(c, http.StatusServiceUnavailable, "Agent connection is busy")
	}
}

// healthCheck returns the server health status
func (s *Server) healthCheck(c *gin.Context) {
	s.agentsMu.RLock()
	agentCount := len(s.agents)
	s.agentsMu.RUnlock()

	s.sendJSON(c, http.StatusOK, gin.H{
		"status":      "healthy",
		"timestamp":   time.Now().UTC(),
		"agent_count": agentCount,
	})
}

// handleAgentWebSocket handles WebSocket connections from agents
func (s *Server) handleAgentWebSocket(c *gin.Context) {
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	// Create agent connection
	agentConn := &AgentConnection{
		ID:       uuid.New().String(),
		Conn:     conn,
		Send:     make(chan []byte, 256),
		LastSeen: time.Now(),
	}

	log.Printf("New agent connection: %s", agentConn.ID)

	// Start goroutines for handling the connection
	go s.handleAgentReader(agentConn)
	go s.handleAgentWriter(agentConn)

	// Wait for registration message
	// The agent should send a registration message first
}

// handleAgentReader reads messages from the agent WebSocket connection
func (s *Server) handleAgentReader(agentConn *AgentConnection) {
	defer func() {
		agentConn.Conn.Close()
		s.agentsMu.Lock()
		if agentConn.Agent != nil {
			delete(s.agents, agentConn.Agent.ID)
			log.Printf("Agent disconnected: %s", agentConn.Agent.ID)
		}
		s.agentsMu.Unlock()
	}()

	agentConn.Conn.SetReadLimit(512)
	agentConn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	agentConn.Conn.SetPongHandler(func(string) error {
		agentConn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, messageBytes, err := agentConn.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		agentConn.mu.Lock()
		agentConn.LastSeen = time.Now()
		agentConn.mu.Unlock()

		var message api.Message
		if err := json.Unmarshal(messageBytes, &message); err != nil {
			log.Printf("Failed to unmarshal message: %v", err)
			continue
		}

		s.handleAgentMessage(agentConn, &message)
	}
}

// handleAgentWriter writes messages to the agent WebSocket connection
func (s *Server) handleAgentWriter(agentConn *AgentConnection) {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		agentConn.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-agentConn.Send:
			agentConn.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				agentConn.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := agentConn.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("Failed to write message: %v", err)
				return
			}

		case <-ticker.C:
			agentConn.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := agentConn.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleAgentMessage processes messages received from agents
func (s *Server) handleAgentMessage(agentConn *AgentConnection, message *api.Message) {
	switch message.Type {
	case api.MessageTypeRegistration:
		s.handleAgentRegistration(agentConn, message)
	case api.MessageTypeHeartbeat:
		s.handleAgentHeartbeat(agentConn, message)
	case api.MessageTypeSecretResponse:
		s.handleSecretResponse(agentConn, message)
	default:
		log.Printf("Unknown message type: %s", message.Type)
	}
}

// handleAgentRegistration processes agent registration
func (s *Server) handleAgentRegistration(agentConn *AgentConnection, message *api.Message) {
	var regReq api.RegistrationRequest
	if err := json.Unmarshal(message.Payload, &regReq); err != nil {
		log.Printf("Failed to unmarshal registration request: %v", err)
		return
	}

	// Create agent
	agent := &api.Agent{
		ID:        uuid.New().String(),
		Name:      regReq.Name,
		ClusterID: uuid.New().String(), // Generate cluster ID
		Status:    api.AgentStatusOnline,
		LastSeen:  time.Now(),
		Metadata:  regReq.Metadata,
	}

	agentConn.Agent = agent

	// Add to agents map
	s.agentsMu.Lock()
	s.agents[agent.ID] = agentConn
	s.agentsMu.Unlock()

	// Save agent to storage
	if err := s.storage.SaveAgent(agent); err != nil {
		log.Printf("Failed to save agent to storage: %v", err)
	}

	log.Printf("Agent registered: %s (%s)", agent.Name, agent.ID)

	// Send registration response
	regResp := api.RegistrationResponse{
		Success:  true,
		AgentID:  agent.ID,
		ServerID: "server-" + uuid.New().String(),
	}

	respMessage := api.Message{
		ID:   message.ID,
		Type: api.MessageTypeRegistration,
	}

	payload, err := json.Marshal(regResp)
	if err != nil {
		log.Printf("Failed to marshal registration response: %v", err)
		return
	}
	respMessage.Payload = payload

	messageBytes, err := json.Marshal(respMessage)
	if err != nil {
		log.Printf("Failed to marshal response message: %v", err)
		return
	}

	select {
	case agentConn.Send <- messageBytes:
	default:
		log.Printf("Failed to send registration response to agent %s", agent.ID)
	}
}

// handleAgentHeartbeat processes agent heartbeat
func (s *Server) handleAgentHeartbeat(agentConn *AgentConnection, message *api.Message) {
	var heartbeatReq api.HeartbeatRequest
	if err := json.Unmarshal(message.Payload, &heartbeatReq); err != nil {
		log.Printf("Failed to unmarshal heartbeat request: %v", err)
		return
	}

	if agentConn.Agent != nil {
		agentConn.Agent.LastSeen = time.Now()
		agentConn.Agent.Status = api.AgentStatusOnline
	}

	// Send heartbeat response
	heartbeatResp := api.HeartbeatResponse{
		Success:  true,
		ServerID: "server-" + uuid.New().String(),
	}

	respMessage := api.Message{
		ID:   message.ID,
		Type: api.MessageTypeHeartbeat,
	}

	payload, err := json.Marshal(heartbeatResp)
	if err != nil {
		log.Printf("Failed to marshal heartbeat response: %v", err)
		return
	}
	respMessage.Payload = payload

	messageBytes, err := json.Marshal(respMessage)
	if err != nil {
		log.Printf("Failed to marshal response message: %v", err)
		return
	}

	select {
	case agentConn.Send <- messageBytes:
	default:
		log.Printf("Failed to send heartbeat response to agent %s", heartbeatReq.AgentID)
	}
}

// handleSecretResponse processes secret operation responses from agents
func (s *Server) handleSecretResponse(agentConn *AgentConnection, message *api.Message) {
	var secretResp api.SecretResponse
	if err := json.Unmarshal(message.Payload, &secretResp); err != nil {
		log.Printf("Failed to unmarshal secret response: %v", err)
		return
	}

	// Log the response for now - in a real implementation, you might want to
	// store this in a database or send it back to a waiting HTTP request
	if secretResp.Success {
		log.Printf("Secret operation successful for agent %s", agentConn.Agent.ID)
	} else {
		log.Printf("Secret operation failed for agent %s: %s", agentConn.Agent.ID, secretResp.Error)
	}
}
