package hubs

import (
	"fmt"

	"../signalr"
)

type Chat struct {
	signalr.Hub // Implements Hub
}

func NewChat() *Chat {
	return &Chat{}
}

func (c *Chat) OnConnected(connectionID string) {
	fmt.Printf("%s connected\n", connectionID)

	c.Groups().AddToGroup("group", connectionID)
}

func (c *Chat) OnDisconnected(connectionID string) {
	fmt.Printf("%s disconnected\n", connectionID)

	c.Groups().RemoveFromGroup("group", connectionID)
}

func (c *Chat) Send(message string) string {
	c.Clients().Group("group").Send("send", message)

	return "Hello World"
}
