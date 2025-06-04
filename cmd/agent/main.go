// Package main implements the agent binary entry point
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/opan/secman/pkg/agent"
)

func main() {
	// Parse command line flags
	serverURL := flag.String("server-url", "http://localhost:8080", "URL of the SecMan server")
	name := flag.String("name", "", "Name of this agent (defaults to hostname)")
	kubeconfigPath := flag.String("kubeconfig", "", "Path to kubeconfig file (defaults to in-cluster config when empty)")

	flag.Parse()

	// Create agent config
	config := &agent.Config{
		Name:           *name,
		ServerURL:      *serverURL,
		KubeconfigPath: *kubeconfigPath,
	}

	// Use in-cluster config if kubeconfig not provided
	if config.KubeconfigPath == "" {
		// Check if we're running in a Kubernetes cluster
		if _, exists := os.LookupEnv("KUBERNETES_SERVICE_HOST"); exists {
			log.Println("Using in-cluster Kubernetes configuration")
		} else {
			// If not in cluster, try to use default kubeconfig location
			homeDir, err := os.UserHomeDir()
			if err == nil {
				config.KubeconfigPath = homeDir + "/.kube/config"
				log.Printf("Using default kubeconfig: %s", config.KubeconfigPath)
			} else {
				log.Printf("Failed to determine home directory: %v", err)
			}
		}
	}

	// Create agent
	agent, err := agent.New(config)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start agent
	if err := agent.Start(ctx); err != nil {
		log.Fatalf("Failed to start agent: %v", err)
	}

	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Wait for termination signal
	sig := <-sigCh
	log.Printf("Received signal: %v", sig)

	// Stop agent
	if err := agent.Stop(); err != nil {
		log.Printf("Error stopping agent: %v", err)
	}

	log.Println("Agent stopped")
}
