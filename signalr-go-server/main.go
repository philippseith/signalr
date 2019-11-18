package main

import (
	"log"
	"net/http"

	"./signalr"
)

func main() {
	http.Handle("/", http.FileServer(http.Dir("public")))

	signalr.MapHub("/chat", NewChat())

	if err := http.ListenAndServe("localhost:8086", nil); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
