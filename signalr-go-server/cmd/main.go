package main

import (
	"log"
	"net/http"

	"../pkg/hubs"
	"../pkg/signalr"
)

func main() {
	http.Handle("/", http.FileServer(http.Dir("../public")))

	signalr.MapHub("/chat", hubs.NewChat())

	if err := http.ListenAndServe("localhost:8086", nil); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
