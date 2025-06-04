// Package main implements the server binary entry point
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	server "github.com/opan/secman/pkg/web"
)

func main() {
	// Parse command line flags
	host := flag.String("host", "0.0.0.0", "Host to bind the server to")
	port := flag.Int("port", 8080, "Port to listen on")
	uiDir := flag.String("ui-dir", "./ui/dist", "Path to UI files")
	storagePath := flag.String("storage-path", "./data", "Path to storage directory")

	flag.Parse()

	// Ensure storage directory exists
	if err := os.MkdirAll(*storagePath, 0755); err != nil {
		log.Fatalf("Failed to create storage directory: %v", err)
	}

	// Resolve UI directory path
	absUIDir, err := filepath.Abs(*uiDir)
	if err != nil {
		log.Fatalf("Failed to resolve UI directory path: %v", err)
	}

	// Check if UI directory exists
	if _, err := os.Stat(absUIDir); os.IsNotExist(err) {
		log.Printf("Warning: UI directory %s does not exist. Web UI will not be available.", absUIDir)
	}

	// Create server config
	config := &server.Config{
		Host:        *host,
		Port:        *port,
		UIDir:       absUIDir,
		StoragePath: *storagePath,
	}

	// Create server
	srv, err := server.NewServer(config)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		log.Printf("Starting server on %s:%d", *host, *port)
		if err := srv.Start(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for termination signal
	sig := <-sigCh
	log.Printf("Received signal: %v", sig)

	// Graceful shutdown
	if err := srv.Stop(ctx); err != nil {
		log.Printf("Error stopping server: %v", err)
	}

	log.Println("Server stopped")
}
