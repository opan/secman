package main

import (
	"encoding/json"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/opan/secman/pkg/api"
)

func main() {
	// Parse server URL from command line or use default
	serverURL := "ws://localhost:8080/ws/agent"
	if len(os.Args) > 1 {
		serverURL = os.Args[1]
	}

	// Parse URL
	u, err := url.Parse(serverURL)
	if err != nil {
		log.Fatal("Invalid server URL:", err)
	}

	log.Printf("Connecting to %s", u.String())

	// Connect to WebSocket
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer c.Close()

	// Channel for interrupt signal
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// Channel for done signal
	done := make(chan struct{})

	// Start reading messages
	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("Read error:", err)
				return
			}

			var msg api.Message
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("Failed to unmarshal message: %v", err)
				continue
			}

			log.Printf("Received message: Type=%s, ID=%s", msg.Type, msg.ID)

			// Handle different message types
			switch msg.Type {
			case api.MessageTypeRegistration:
				var regResp api.RegistrationResponse
				if err := json.Unmarshal(msg.Payload, &regResp); err == nil {
					if regResp.Success {
						log.Printf("Registration successful! Agent ID: %s", regResp.AgentID)
					} else {
						log.Printf("Registration failed: %s", regResp.Error)
					}
				}
			case api.MessageTypeHeartbeat:
				var hbResp api.HeartbeatResponse
				if err := json.Unmarshal(msg.Payload, &hbResp); err == nil {
					if hbResp.Success {
						log.Printf("Heartbeat acknowledged by server: %s", hbResp.ServerID)
					}
				}
			}
		}
	}()

	// Send registration message
	regReq := api.RegistrationRequest{
		Name: "example-agent",
		Metadata: api.AgentMetadata{
			KubernetesVersion: "v1.28.0",
			NodeCount:         3,
			Labels: map[string]string{
				"environment": "development",
				"region":      "us-west-2",
			},
		},
	}

	regPayload, err := json.Marshal(regReq)
	if err != nil {
		log.Fatal("Failed to marshal registration request:", err)
	}

	regMessage := api.Message{
		ID:      uuid.New().String(),
		Type:    api.MessageTypeRegistration,
		Payload: regPayload,
	}

	regMessageBytes, err := json.Marshal(regMessage)
	if err != nil {
		log.Fatal("Failed to marshal registration message:", err)
	}

	if err := c.WriteMessage(websocket.TextMessage, regMessageBytes); err != nil {
		log.Fatal("Failed to send registration message:", err)
	}

	log.Println("Registration message sent")

	// Send heartbeat every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// Send heartbeat
			hbReq := api.HeartbeatRequest{
				AgentID: "example-agent-id", // In real implementation, use the ID from registration response
			}

			hbPayload, err := json.Marshal(hbReq)
			if err != nil {
				log.Printf("Failed to marshal heartbeat request: %v", err)
				continue
			}

			hbMessage := api.Message{
				ID:      uuid.New().String(),
				Type:    api.MessageTypeHeartbeat,
				Payload: hbPayload,
			}

			hbMessageBytes, err := json.Marshal(hbMessage)
			if err != nil {
				log.Printf("Failed to marshal heartbeat message: %v", err)
				continue
			}

			if err := c.WriteMessage(websocket.TextMessage, hbMessageBytes); err != nil {
				log.Printf("Failed to send heartbeat: %v", err)
				return
			}

			log.Println("Heartbeat sent")

		case <-interrupt:
			log.Println("Interrupt received, closing connection...")

			// Send close message
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("Write close error:", err)
				return
			}

			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}
