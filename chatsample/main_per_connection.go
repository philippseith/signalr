package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	kitlog "github.com/go-kit/log"
	"github.com/philippseith/signalr"
	"github.com/philippseith/signalr/chatsample/middleware"
	"github.com/philippseith/signalr/chatsample/public"
)

// chatPerConnection is a hub that maintains state per connection
type chatPerConnection struct {
	signalr.Hub
	connectionID string
	username     string
	messageCount int
	joinTime     time.Time
}

func (c *chatPerConnection) OnConnected(connectionID string) {
	c.connectionID = connectionID
	c.messageCount = 0
	c.joinTime = time.Now()
	c.username = fmt.Sprintf("User_%s", connectionID[:8]) // Generate username from connection ID

	fmt.Printf("New connection: %s, Username: %s\n", connectionID, c.username)

	// Send welcome message to the new user
	c.Clients().Caller().Send("ReceiveMessage", "System", fmt.Sprintf("Welcome %s! You are connection #%s", c.username, connectionID[:8]))

	// Broadcast to all clients that a new user joined
	c.Clients().All().Send("ReceiveMessage", "System", fmt.Sprintf("%s has joined the chat", c.username))
}

func (c *chatPerConnection) OnDisconnected(connectionID string) {
	fmt.Printf("Connection closed: %s, Username: %s, Messages sent: %d, Duration: %v\n",
		connectionID, c.username, c.messageCount, time.Since(c.joinTime))

	// Broadcast to all clients that a user left
	c.Clients().All().Send("ReceiveMessage", "System", fmt.Sprintf("%s has left the chat", c.username))
}

func (c *chatPerConnection) SendMessage(message string) {
	c.messageCount++

	// Log the message with connection-specific info
	fmt.Printf("[%s] %s: %s (Message #%d)\n", c.connectionID[:8], c.username, message, c.messageCount)

	// Send message to all clients
	c.Clients().All().Send("ReceiveMessage", c.username, message)
}

func (c *chatPerConnection) SetUsername(username string) {
	oldUsername := c.username
	c.username = username

	fmt.Printf("Username changed: %s -> %s (Connection: %s)\n", oldUsername, username, c.connectionID[:8])

	// Broadcast username change
	c.Clients().All().Send("ReceiveMessage", "System", fmt.Sprintf("%s is now known as %s", oldUsername, username))
}

func (c *chatPerConnection) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"connectionID": c.connectionID[:8],
		"username":     c.username,
		"messageCount": c.messageCount,
		"joinTime":     c.joinTime.Format(time.RFC3339),
		"duration":     time.Since(c.joinTime).String(),
	}
}

func (c *chatPerConnection) Echo(message string) {
	c.Clients().Caller().Send("ReceiveMessage", "Echo", message)
}

func (c *chatPerConnection) Abort() {
	fmt.Printf("Aborting connection: %s, Username: %s\n", c.connectionID[:8], c.username)
	c.Hub.Abort()
}

func runHTTPServerPerConnection(address string) {
	// Create server with PerConnectionHubFactory
	server, _ := signalr.NewServer(context.TODO(),
		signalr.PerConnectionHubFactory(func(connectionID string) signalr.HubInterface {
			return &chatPerConnection{}
		}),
		signalr.Logger(kitlog.NewLogfmtLogger(os.Stdout), false),
		signalr.KeepAliveInterval(2*time.Second))

	router := http.NewServeMux()
	server.MapHTTP(signalr.WithHTTPServeMux(router), "/chat")

	fmt.Printf("Serving public content from the embedded filesystem\n")
	router.Handle("/", http.FileServer(http.FS(public.FS)))
	fmt.Printf("Listening for websocket connections on http://%s\n", address)
	fmt.Printf("Using PerConnectionHubFactory - each connection gets its own hub instance!\n")

	if err := http.ListenAndServe(address, middleware.LogRequests(router)); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}

func runHTTPClientPerConnection(address string, receiver interface{}) error {
	c, err := signalr.NewClient(context.Background(), nil,
		signalr.WithReceiver(receiver),
		signalr.WithConnector(func() (signalr.Connection, error) {
			creationCtx, _ := context.WithTimeout(context.Background(), 2*time.Second)
			return signalr.NewHTTPConnection(creationCtx, address)
		}),
		signalr.Logger(kitlog.NewLogfmtLogger(os.Stdout), false))
	if err != nil {
		return err
	}
	c.Start()
	fmt.Println("Client started")
	return nil
}

type receiverPerConnection struct {
	signalr.Receiver
}

func (r *receiverPerConnection) Receive(msg string) {
	fmt.Println(msg)
	// The silly client urges the server to end his connection after 10 seconds
	r.Server().Send("abort")
}

func mainPerConnection() {
	fmt.Println("=== Per-Connection Hub Factory Demo ===")
	fmt.Println("This demo shows how each connection gets its own hub instance")
	fmt.Println("with persistent state throughout the connection lifetime.")
	fmt.Println()

	go runHTTPServerPerConnection("localhost:8087")
	<-time.After(time.Millisecond * 2)

	go func() {
		fmt.Println(runHTTPClientPerConnection("http://localhost:8087/chat", &receiverPerConnection{}))
	}()

	ch := make(chan struct{})
	<-ch
}
