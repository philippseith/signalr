package hubs

import (
	"fmt"

	"../signalr"
)

type Chat interface {
	OnConnected(id string)
	OnDisconnected(id string)
	Send(message string)
	Initialize(clients signalr.HubContext)
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

func (c *chat) OnConnected(id string) {
	fmt.Printf("%s connected\n", id)

	c.context.Groups().AddToGroup("group", id)
}

func (c *chat) OnDisconnected(id string) {
	fmt.Printf("%s disconnected\n", id)

	c.context.Groups().RemoveFromGroup("group", id)
}

func (c *chat) Send(message string) {
	c.context.Clients().Group("group").Send("send", message)
}
