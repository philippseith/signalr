package hubs

import (
	"fmt"

	"../signalr"
)

type Chat interface {
	signalr.Hub // Implements Hub
	OnConnected(connectionID string)
	OnDisconnected(connectionID string)
	Send(message string) string
}

type chat struct {
	context signalr.HubContext
}

func NewChat() Chat {
	return &chat{}
}

func (c *chat) Initialize(ctx signalr.HubContext) {
	c.context = ctx
}

func (c *chat) OnConnected(connectionID string) {
	fmt.Printf("%s connected\n", connectionID)

	c.context.Groups().AddToGroup("group", connectionID)
}

func (c *chat) OnDisconnected(connectionID string) {
	fmt.Printf("%s disconnected\n", connectionID)

	c.context.Groups().RemoveFromGroup("group", connectionID)
}

func (c *chat) Send(message string) string {
	c.context.Clients().Group("group").Send("send", message)

	return "Hello World"
}
