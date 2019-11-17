package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"golang.org/x/net/websocket"
)

type HandshakeRequest struct {
	Protocol string `json:"protocol"`
	Version  int    `json:"version"`
}

func hubConnectionHandler(ws *websocket.Conn) {
	finished := make(chan bool)

	go handleReads(finished, ws)
	// go handleWrites(ws)
	<-finished
}

type HubMessage struct {
	Type int `json:"type"`
}

type HubInvocationMessage struct {
	Type      int               `json:"type"`
	Target    string            `json:"target"`
	Arguments []json.RawMessage `json:"arguments"`
}

func handleReads(finished chan bool, ws *websocket.Conn) {
	var err error
	var data []byte
	handshake := false

	for {
		if err = websocket.Message.Receive(ws, &data); err != nil {
			fmt.Println("Can't receive")
			break
		}

		if !handshake {
			rawHandshake, remainder := parseTextMessageFormat(data)

			if len(remainder) > 0 {
				fmt.Println("Can't handle partial messages yet...I'm lazy")
				return
			}

			request := HandshakeRequest{}
			json.Unmarshal(rawHandshake, &request)

			var handshakeResponse = []byte{'{', '}', 30}

			// Send the handshake response (it's a string so it sends text back)
			websocket.Message.Send(ws, string(handshakeResponse))

			handshake = true
			continue
		}

		for {
			message, remainder := parseTextMessageFormat(data)

			hubMessage := HubMessage{}
			json.Unmarshal(message, &hubMessage)

			switch hubMessage.Type {
			case 1:
				invocation := HubInvocationMessage{}
				json.Unmarshal(message, &invocation)

				// Dispatch invocation here

				break
			case 6:
				// Ping
				break
			}

			if len(remainder) == 0 {
				break
			}

			data = remainder
		}
	}

	finished <- true
}

func parseTextMessageFormat(data []byte) ([]byte, []byte) {
	i := 0
	for i < len(data) {
		// Record separator
		if data[i] == 30 {
			break
		}
		i++
	}
	return data[0:i], data[i+1:]
}

func handleWrites(ws *websocket.Conn) {
	// TODO: Use channels here
	for {

	}
}

type AvailableTransport struct {
	Transport       string   `json:"transport"`
	TransferFormats []string `json:"transferFormats"`
}

type NegotiateResponse struct {
	ConnectionId        string               `json:"connectionId"`
	AvailableTransports []AvailableTransport `json:"availableTransports"`
}

func negotiateHandler(w http.ResponseWriter, req *http.Request) {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	var connectionId = base64.StdEncoding.EncodeToString(bytes)

	response := NegotiateResponse{
		ConnectionId: connectionId,
		AvailableTransports: []AvailableTransport{
			AvailableTransport{
				Transport:       "WebSockets",
				TransferFormats: []string{"Text", "Binary"},
			},
		},
	}

	json.NewEncoder(w).Encode(response)
}

func main() {
	http.Handle("/", http.FileServer(http.Dir("public")))

	// SignalR protocol
	http.HandleFunc("/chat/negotiate", negotiateHandler)
	http.Handle("/chat", websocket.Handler(hubConnectionHandler))

	if err := http.ListenAndServe("localhost:8086", nil); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
