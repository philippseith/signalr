# Per-Connection Hub Factory Example

This example demonstrates how to use the new `PerConnectionHubFactory` option to create a 1:1 mapping between client connections and hub instances.

## Problem

Previously, SignalR servers had two options for hub management:

1. **UseHub**: One hub instance shared across all connections (shared state)
2. **HubFactory/SimpleHubFactory**: New hub instance created for every method call (no persistent state)

Neither option provided a way to have one hub instance per connection that persists for the lifetime of that connection.

## Solution

The new `PerConnectionHubFactory` option creates one hub instance per connection and reuses it for all method invocations on that connection. This provides:

- **1:1 mapping**: One hub instance per client connection
- **Persistent state**: Hub instance maintains state throughout the connection lifetime
- **Connection isolation**: Each connection has its own isolated hub instance
- **Memory efficiency**: Hub instances are cleaned up when connections close

## Usage

```go
package main

import (
    "context"
    "github.com/philippseith/signalr"
)

// Define your hub with connection-specific state
type ChatHub struct {
    signalr.Hub
    connectionID string
    username     string
    messageCount int
}

func (h *ChatHub) OnConnected(connectionID string) {
    h.connectionID = connectionID
    h.messageCount = 0
    // Initialize connection-specific state
}

func (h *ChatHub) SendMessage(message string) {
    h.messageCount++
    // Send message to all clients
    h.Clients().All().Send("ReceiveMessage", h.username, message)
}

func (h *ChatHub) SetUsername(username string) {
    h.username = username
}

func (h *ChatHub) GetStats() map[string]interface{} {
    return map[string]interface{}{
        "connectionID": h.connectionID,
        "username":     h.username,
        "messageCount": h.messageCount,
    }
}

func main() {
    ctx := context.Background()
    
    // Create server with PerConnectionHubFactory
    server, err := signalr.NewServer(ctx,
        signalr.PerConnectionHubFactory(func(connectionID string) signalr.HubInterface {
            return &ChatHub{}
        }),
        signalr.Logger(signalr.StructuredLogger(nil), false),
    )
    if err != nil {
        panic(err)
    }
    
    // Use the server as usual
    // Each connection will get its own ChatHub instance
    // that persists for the lifetime of the connection
}
```

## How It Works

1. **Connection Establishment**: When a client connects, `PerConnectionHubFactory` is called with the connection ID
2. **Hub Creation**: A new hub instance is created and stored in the server's connection map
3. **Method Invocation**: All subsequent method calls on that connection use the same hub instance
4. **Connection Cleanup**: When the connection closes, the hub instance is automatically removed from the map

## Benefits

- **Stateful Hubs**: Hubs can maintain connection-specific state (user sessions, counters, etc.)
- **Connection Isolation**: Each client gets its own isolated hub instance
- **Memory Management**: Automatic cleanup prevents memory leaks
- **Performance**: No need to recreate hub instances for each method call
- **Flexibility**: Factory function can create hubs with connection-specific configuration

## Comparison

| Option | Hub Instances | State Persistence | Use Case |
|--------|---------------|-------------------|----------|
| `UseHub` | 1 (shared) | Global state | Broadcast to all clients |
| `HubFactory` | New per call | No state | Stateless operations |
| `PerConnectionHubFactory` | 1 per connection | Per-connection state | User sessions, personal data |

## Migration

To migrate from existing patterns:

- **From `UseHub`**: If you need per-connection state, replace with `PerConnectionHubFactory`
- **From `HubFactory`**: If you want to maintain state between method calls, use `PerConnectionHubFactory`
- **New implementations**: Start with `PerConnectionHubFactory` for most use cases requiring state

## Example Use Cases

- **Chat applications**: Track user sessions, message counts, typing indicators
- **Gaming**: Maintain player state, game progress, scores
- **Dashboard**: Store user preferences, cached data, real-time updates
- **Collaboration tools**: Track document editing state, user presence
- **IoT applications**: Maintain device state, sensor data history
