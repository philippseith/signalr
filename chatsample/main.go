package main

import (
	"context"
	"fmt"
	"github.com/philippseith/signalr"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
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
	c.Clients().Group("group").Send("Send", message)
}

func (c *chat) Echo(message string) {
	c.Clients().Caller().Send("send", message)
}

func (c *chat) Panic() {
	panic("Don't panic!")
}

func (c *chat) RequestAsync(message string) <-chan map[string]string {
	r := make(chan map[string]string)
	go func() {
		defer close(r)
		time.Sleep(5 * time.Second)
		m := make(map[string]string)
		m["ToUpper"] = strings.ToUpper(message)
		m["ToLower"] = strings.ToLower(message)
		m["len"] = fmt.Sprint(len(message))
		r <- m
	}()
	return r
}

func (c *chat) RequestTuple(message string) (string, string, int) {
	return strings.ToUpper(message), strings.ToLower(message), len(message)
}

func (c *chat) DateStream() <-chan string {
	r := make(chan string)
	go func() {
		defer close(r)
		for i := 0; i < 50; i++ {
			r <- fmt.Sprint(time.Now().Clock())
			time.Sleep(time.Second)
		}
	}()
	return r
}

func (c *chat) UploadStream(upload1 <-chan int, factor float64, upload2 <-chan float64) {
	ok1 := true
	ok2 := true
	u1 := 0
	u2 := 0.0
	c.Echo(fmt.Sprintf("f: %v", factor))
	for {
		select {
		case u1, ok1 = <-upload1:
			if ok1 {
				c.Echo(fmt.Sprintf("u1: %v", u1))
			} else if !ok2 {
				c.Echo("Finished")
				return
			}
		case u2, ok2 = <-upload2:
			if ok2 {
				c.Echo(fmt.Sprintf("u2: %v", u2))
			} else if !ok1 {
				c.Echo("Finished")
				return
			}
		}
	}
}

func runTCP(address string, hub signalr.HubInterface) {
	listener, err := net.Listen("tcp", address)

	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("Listening for TCP connection on %s\n", listener.Addr())

	server, _ := signalr.NewServer(signalr.UseHub(hub))

	for {
		conn, err := listener.Accept()

		if err != nil {
			fmt.Println(err)
			break
		}

		go server.Run(context.TODO(), newNetConnection(conn))
	}
}

func runHTTP(address string, hub signalr.HubInterface) {
	router := http.NewServeMux()
	router.Handle("/", http.FileServer(http.Dir("./public")))

	signalr.MapHub(router, "/chat", hub)

	fmt.Printf("Listening for websocket connections on %s\n", address)

	if err := http.ListenAndServe(address, router); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}

func main() {
	hub := &chat{}

	go runTCP("127.0.0.1:8007", hub)

	// Block on the HTTP server
	runHTTP("localhost:8086", hub)
}
