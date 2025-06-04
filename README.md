# SecMan - Kubernetes Secret Manager

SecMan is a web-based system for managing Kubernetes secrets across multiple clusters. It uses a server-agent architecture to provide centralized control through a web interface.

## Architecture

SecMan consists of three main components:

1. **Server**: The central component that provides a web UI and API for managing secrets across multiple Kubernetes clusters.
2. **Agent**: Runs in each Kubernetes cluster and communicates with the server to perform secret operations.
3. **Web UI**: A browser-based interface for interacting with the SecMan server.

The architecture is similar to Consul, with a central server and agents running in each cluster.

## Features

- Centralized management of Kubernetes secrets across multiple clusters
- Web-based UI for managing secrets
- REST API for programmatic access
- Real-time agent status monitoring
- Secure WebSocket communication between server and agents

## Installation

### Building from Source

```bash
git clone https://github.com/opan/secman.git
cd secman

# Build server
go build -o secman-server ./cmd/server

# Build agent
go build -o secman-agent ./cmd/agent
```

## Usage

### Running the Server

```bash
./secman-server --host 0.0.0.0 --port 8080 --storage-path ./data
```

Server options:
- `--host`: Host to bind the server to (default: "0.0.0.0")
- `--port`: Port to listen on (default: 8080)
- `--ui-dir`: Path to UI files (default: "./ui/dist")
- `--storage-path`: Path to storage directory (default: "./data")

### Running the Agent

```bash
./secman-agent --server-url http://server-address:8080 --name agent-1
```

Agent options:
- `--server-url`: URL of the SecMan server (default: "http://localhost:8080")
- `--name`: Name of this agent (defaults to hostname)
- `--kubeconfig`: Path to kubeconfig file (defaults to in-cluster config when empty)

```bash
secman list -n default
# or
secman list --namespace default
```

### Get a Secret

```bash
secman get -n default -s mysecret
# or
secman get --namespace default --name mysecret
```

### Create a Secret

```bash
secman create -n default -s mysecret -k username -v admin
# or
secman create --namespace default --name mysecret --key username --value admin
```

### Update a Secret

```bash
secman update -n default -s mysecret -k password -v secretpassword
# or
secman update --namespace default --name mysecret --key password --value secretpassword
```

### Delete a Secret

Delete the entire secret:

```bash
secman delete --namespace=default --name=mysecret
```

Delete a specific key from a secret:

```bash
secman delete --namespace=default --name=mysecret --key=password
```

## Environment Variables

- `KUBECONFIG`: Path to the kubeconfig file (defaults to `~/.kube/config`)

## Development

### Prerequisites

- Go 1.16 or higher
- Access to a Kubernetes cluster for testing

### Building

```bash
go build -o secman ./cmd/secman
```

### Running Tests

```bash
go test ./...
```

## Using the Web UI

Once the server is running, you can access the web UI by navigating to `http://server-address:8080` in your web browser. The UI allows you to:

1. View connected agents and their status
2. Browse and manage secrets across all connected clusters
3. Create, view, edit, and delete secrets

## API Reference

The SecMan server provides a REST API for managing secrets:

### Agent Management

- `GET /api/agents`: List all registered agents and their status

### Secret Management

- `GET /api/secrets/:agent_id/:namespace`: List all secrets in a namespace
- `GET /api/secrets/:agent_id/:namespace/:name`: Get a specific secret
- `POST /api/secrets/:agent_id/:namespace`: Create a new secret
- `PUT /api/secrets/:agent_id/:namespace/:name`: Update an existing secret
- `DELETE /api/secrets/:agent_id/:namespace/:name`: Delete a secret

## Security Considerations

- SecMan currently does not implement authentication or authorization. It should only be deployed within secure networks.
- Future versions will include proper authentication and authorization mechanisms.

## Development Status

This project is currently in active development. Some features may not be fully implemented yet.

## License

MIT
