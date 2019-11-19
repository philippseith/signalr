package main

import (
	"log"
	"net/http"

	"../pkg/hubs"
	"../pkg/signalr"
)

func main() {
	router := http.NewServeMux()
	router.Handle("/", http.FileServer(http.Dir("../public")))

	signalr.MapHub(router, "/chat", hubs.NewChat())

	if err := http.ListenAndServe("localhost:8086", router); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
