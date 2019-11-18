package main

import (
	"log"
	"net/http"

	"./signalr"
)

type Chat struct {
	Clients signalr.HubClients
}

func (c *Chat) Initialize(clients signalr.HubClients) {
	c.Clients = clients
}

func (c *Chat) Send(message string) {
	c.Clients.All().Send("send", message)
}

func main() {
	http.Handle("/", http.FileServer(http.Dir("public")))

	signalr.MapHub("/chat", &Chat{})

	if err := http.ListenAndServe("localhost:8086", nil); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
