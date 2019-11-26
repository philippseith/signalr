package main

import (
	"fmt"
	"log"
	"net/http"

	"../pkg/signalr"
)

type chat struct {
	signalr.Hub
}

func (c *chat) OnConnected(connectionID string) {
	fmt.Printf("%s connected\n", connectionID)

	c.Groups().AddToGroup("group", connectionID)
}

func (c *chat) OnDisconnected(connectionID string) {
	fmt.Printf("%s disconnected\n", connectionID)

	c.Groups().RemoveFromGroup("group", connectionID)
}

func (c *chat) Send(message string) {
	c.Clients().Group("group").Send("send", message)
}

func main() {
	router := http.NewServeMux()
	router.Handle("/", http.FileServer(http.Dir("../public")))

	signalr.MapHub(router, "/chat", &chat{})

	if err := http.ListenAndServe("localhost:8086", router); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
