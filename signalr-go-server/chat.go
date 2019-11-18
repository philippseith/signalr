package main

import "./signalr"

type Chat interface {
	Send(message string)
	Initialize(clients signalr.HubClients)
}

type chat struct {
	clients signalr.HubClients
}

func NewChat() Chat {
	return &chat{}
}

func (c *chat) Initialize(clients signalr.HubClients) {
	c.clients = clients
}

func (c *chat) Send(message string) {
	c.clients.All().Send("send", message)
}
