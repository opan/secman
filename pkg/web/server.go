package web

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/opan/secman/pkg/api"
	"github.com/opan/secman/pkg/storage"
)

// Config holds the server configuration
type Config struct {
	Host        string
	Port        int
	UIDir       string
	StoragePath string
}

// Server represents the web server
type Server struct {
	config   *Config
	router   *gin.Engine
	server   *http.Server
	agents   map[string]*AgentConnection
	agentsMu sync.RWMutex
	upgrader websocket.Upgrader
	storage  storage.Storage
}

// AgentConnection represents a connected agent
type AgentConnection struct {
	ID       string
	Agent    *api.Agent
	Conn     *websocket.Conn
	Send     chan []byte
	LastSeen time.Time
	mu       sync.RWMutex
}

// NewServer creates a new web server instance
func NewServer(config *Config) (*Server, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Initialize storage
	fileStorage, err := storage.NewFileStorage(config.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Set Gin to release mode for production
	gin.SetMode(gin.ReleaseMode)

	s := &Server{
		config:  config,
		router:  gin.New(),
		agents:  make(map[string]*AgentConnection),
		storage: fileStorage,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Allow all origins for now - in production, you should restrict this
				return true
			},
		},
	}

	s.setupRoutes()
	s.setupServer()

	return s, nil
}

// setupRoutes configures all the HTTP routes
func (s *Server) setupRoutes() {
	// Add middleware
	s.router.Use(gin.Logger())
	s.router.Use(gin.Recovery())
	s.router.Use(s.corsMiddleware())

	// API routes
	api := s.router.Group("/api/v1")
	{
		// Agent management
		api.GET("/agents", s.listAgents)
		api.GET("/agents/:id", s.getAgent)
		api.DELETE("/agents/:id", s.disconnectAgent)

		// Secret operations
		api.POST("/agents/:id/secrets", s.handleSecretOperation)
		api.GET("/agents/:id/secrets", s.listSecrets)
		api.GET("/agents/:id/secrets/:namespace/:name", s.getSecret)

		// Health check
		api.GET("/health", s.healthCheck)
	}

	// WebSocket endpoint for agent connections
	s.router.GET("/ws/agent", s.handleAgentWebSocket)

	// Serve static UI files if directory exists
	s.router.Use(static.Serve("/", static.LocalFile(s.config.UIDir, false)))

	// Fallback for SPA routing
	s.router.NoRoute(func(c *gin.Context) {
		c.File(s.config.UIDir + "/index.html")
	})
}

// setupServer configures the HTTP server
func (s *Server) setupServer() {
	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.config.Host, s.config.Port),
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// Start starts the web server
func (s *Server) Start() error {
	// Start cleanup routine for stale connections
	go s.cleanupStaleConnections()

	log.Printf("Server starting on %s", s.server.Addr)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start server: %w", err)
	}
	return nil
}

// Stop gracefully stops the web server
func (s *Server) Stop(ctx context.Context) error {
	log.Println("Shutting down server...")

	// Close all agent connections
	s.agentsMu.Lock()
	for _, agent := range s.agents {
		close(agent.Send)
		agent.Conn.Close()
	}
	s.agentsMu.Unlock()

	// Shutdown HTTP server
	return s.server.Shutdown(ctx)
}

// corsMiddleware adds CORS headers
func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// cleanupStaleConnections removes agents that haven't sent heartbeats
func (s *Server) cleanupStaleConnections() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.agentsMu.Lock()
		for id, agent := range s.agents {
			if time.Since(agent.LastSeen) > 2*time.Minute {
				log.Printf("Removing stale agent connection: %s", id)
				close(agent.Send)
				agent.Conn.Close()
				delete(s.agents, id)

				// Remove from storage if agent exists
				if agent.Agent != nil {
					if err := s.storage.DeleteAgent(agent.Agent.ID); err != nil {
						log.Printf("Failed to delete agent from storage: %v", err)
					}
				}
			}
		}
		s.agentsMu.Unlock()
	}
}

// Helper function to send JSON response
func (s *Server) sendJSON(c *gin.Context, status int, data interface{}) {
	c.JSON(status, data)
}

// Helper function to send error response
func (s *Server) sendError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}
