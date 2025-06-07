# Web Server Implementation

This package implements a comprehensive web server for the Kubernetes secrets management system with real-time agent communication capabilities.

## Features

### Core Functionality
- **RESTful API** for agent and secret management
- **WebSocket Support** for real-time agent communication
- **File-based Storage** for persistent agent data
- **CORS Support** for cross-origin requests
- **Static File Serving** for web UI
- **Graceful Shutdown** with proper cleanup

### API Endpoints

#### Agent Management
- `GET /api/v1/agents` - List all connected agents
- `GET /api/v1/agents/:id` - Get specific agent details
- `DELETE /api/v1/agents/:id` - Disconnect an agent

#### Secret Operations
- `POST /api/v1/agents/:id/secrets` - Perform secret operations
- `GET /api/v1/agents/:id/secrets` - List secrets in a namespace
- `GET /api/v1/agents/:id/secrets/:namespace/:name` - Get specific secret

#### Health Check
- `GET /api/v1/health` - Server health status

#### WebSocket
- `GET /ws/agent` - Agent WebSocket connection endpoint

## Architecture

### Server Components
- **HTTP Server**: Gin-based web server with middleware
- **WebSocket Handler**: Real-time communication with agents
- **Storage Layer**: File-based persistence for agent data
- **Connection Manager**: Tracks and manages agent connections

### Message Types
The server handles the following message types via WebSocket:
- `registration` - Agent registration requests
- `heartbeat` - Agent heartbeat messages
- `secret_request` - Secret operation requests
- `secret_response` - Secret operation responses

### Agent Lifecycle
1. **Connection**: Agent connects via WebSocket
2. **Registration**: Agent sends registration message with metadata
3. **Heartbeat**: Regular heartbeat messages to maintain connection
4. **Operations**: Secret operations via request/response pattern
5. **Cleanup**: Automatic cleanup of stale connections

## Configuration

The server accepts the following configuration options:
- `Host`: Server bind address (default: "0.0.0.0")
- `Port`: Server port (default: 8080)
- `UIDir`: Path to UI files (default: "./ui/dist")
- `StoragePath`: Path to storage directory (default: "./data")

## Usage

```go
import "github.com/opan/secman/pkg/web"

config := &web.Config{
    Host:        "0.0.0.0",
    Port:        8080,
    UIDir:       "./ui/dist",
    StoragePath: "./data",
}

server, err := web.NewServer(config)
if err != nil {
    log.Fatal(err)
}

// Start server
if err := server.Start(); err != nil {
    log.Fatal(err)
}
```

## Security Considerations

- **CORS**: Currently allows all origins - should be restricted in production
- **Authentication**: No authentication implemented - should be added for production
- **TLS**: No TLS support - should be added for production deployments
- **Input Validation**: Basic validation implemented - should be enhanced

## Dependencies

- **Gin**: HTTP web framework
- **Gorilla WebSocket**: WebSocket implementation
- **UUID**: Unique identifier generation
- **Kubernetes API**: For secret type definitions