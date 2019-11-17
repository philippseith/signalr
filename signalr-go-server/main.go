package main

import (
	"log"
	"net/http"

	"./signalr"
)

// User code
type Chat struct {
	Clients signalr.HubClients
}

// Hub interface impl (the framework will use this to setup the hub)
func (c Chat) Initialize(clients signalr.HubClients) {
	c.Clients = clients
}

func (c Chat) Send(message string) {
	c.Clients.All.Send("send", message)
}

// Framework

func main() {
	http.Handle("/", http.FileServer(http.Dir("public")))

	signalr.MapHub("/chat", &Chat{})

	if err := http.ListenAndServe("localhost:8086", nil); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
