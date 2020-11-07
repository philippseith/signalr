package main

import (
	"context"
	"fmt"
	kitlog "github.com/go-kit/kit/log"
	"github.com/philippseith/signalr"
	"log"
	"net/http"
	"os"
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
	c.Clients().Group("group").Send("receive", message)
}

func (c *chat) Echo(message string) {
	c.Clients().Caller().Send("receive", message)
}

func (c *chat) Panic() {
	panic("Don't panic!")
}

func (c *chat) RequestAsync(message string) <-chan map[string]string {
	r := make(chan map[string]string)
	go func() {
		defer close(r)
		time.Sleep(4 * time.Second)
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

//func runTCPServer(address string, hub signalr.HubInterface) {
//	listener, err := net.Listen("tcp", address)
//
//	if err != nil {
//		fmt.Println(err)
//		return
//	}
//
//	fmt.Printf("Listening for TCP connection on %s\n", listener.Addr())
//
//	server, _ := signalr.NewServer(context.TODO(), signalr.UseHub(hub))
//
//	for {
//		conn, err := listener.Accept()
//
//		if err != nil {
//			fmt.Println(err)
//			break
//		}
//
//		go server.ServeConnection(context.TODO(), newNetConnection(conn))
//	}
//}

func runHTTPServer(address string, hub signalr.HubInterface) {
	server, _ := signalr.NewServer(context.TODO(), signalr.SimpleHubFactory(hub),
		signalr.KeepAliveInterval(2*time.Second),
		signalr.Logger(kitlog.NewLogfmtLogger(os.Stderr), true))
	router := server.ServeHTTP("/chat")
	router.Handle("/", http.FileServer(http.Dir("./public")))

	fmt.Printf("Listening for websocket connections on %s\n", address)
	if err := http.ListenAndServe(address, router); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}

//func runHTTPClient(address string, client interface{}) {
//	c, _ := signalr.NewHTTPClient(context.TODO(), address) // HubProtocol is determined inside
//	c.SetReceiver(client)
//	c.Start()
//}

type client struct {
	signalr.Hub
}

func (c *client) Receive(msg string) {
	fmt.Println(msg)
}

func main() {
	hub := &chat{}

	//go runTCPServer("127.0.0.1:8007", hub)
	go runHTTPServer("localhost:8086", hub)
	//<-time.After(time.Millisecond * 2)
	//go runHTTPClient("http://localhost:8086/chat", &client{})
	ch := make(chan struct{})
	<-ch
}
