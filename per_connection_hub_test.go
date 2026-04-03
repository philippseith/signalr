package signalr

import (
	"context"
	"testing"
)

// testPerConnectionHub is a test hub that tracks connection-specific state
type testPerConnectionHub struct {
	Hub
	connectionID string
	messageCount int
}

func (h *testPerConnectionHub) OnConnected(connectionID string) {
	h.connectionID = connectionID
	h.messageCount = 0
}

func (h *testPerConnectionHub) IncrementCount() int {
	h.messageCount++
	return h.messageCount
}

func (h *testPerConnectionHub) GetCount() int {
	return h.messageCount
}

func (h *testPerConnectionHub) GetConnectionID() string {
	return h.connectionID
}

func TestPerConnectionHubFactory(t *testing.T) {
	// Create a server with PerConnectionHubFactory
	server, err := NewServer(context.TODO(),
		PerConnectionHubFactory(func(connectionID string) HubInterface {
			return &testPerConnectionHub{}
		}),
		testLoggerOption(),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Test that the server was created successfully
	if server == nil {
		t.Fatal("Server should not be nil")
	}

	// Test that we can get hub clients (this tests that the server is working)
	hubClients := server.HubClients()
	if hubClients == nil {
		t.Fatal("HubClients should not be nil")
	}
}

func TestPerConnectionHubFactoryOption(t *testing.T) {
	// Test that PerConnectionHubFactory is a valid option
	option := PerConnectionHubFactory(func(connectionID string) HubInterface {
		return &testPerConnectionHub{}
	})

	if option == nil {
		t.Fatal("PerConnectionHubFactory should return a valid option")
	}
}
