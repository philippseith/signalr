package signalr

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"golang.org/x/net/websocket"
)

// Protocol
type HubMessage struct {
	Type int `json:"type"`
}

type HubInvocationMessage struct {
	Type      int               `json:"type"`
	Target    string            `json:"target"`
	Arguments []json.RawMessage `json:"arguments"`
}

// Hub
type Hub interface {
	Initialize(clients HubClients)
}

type HubInfo struct {
	hub     *Hub
	methods map[string]reflect.Value
}

type AllClientProxy struct {
}

func (a AllClientProxy) Send(method string, args ...interface{}) {

}

type HubClients struct {
	All AllClientProxy
}

type HandshakeRequest struct {
	Protocol string `json:"protocol"`
	Version  int    `json:"version"`
}

func hubConnectionHandler(ws *websocket.Conn, hubInfo *HubInfo) {
	finished := make(chan bool)

	go handleReads(finished, ws, hubInfo)
	// go handleWrites(ws)
	<-finished
}

func handleReads(finished chan bool, ws *websocket.Conn, hubInfo *HubInfo) {
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

			fmt.Println("Handshake received")

			request := HandshakeRequest{}
			json.Unmarshal(rawHandshake, &request)

			var handshakeResponse = []byte{'{', '}', 30}

			// Send the handshake response (it's a string so it sends text back)
			websocket.Message.Send(ws, string(handshakeResponse))

			handshake = true
			continue
		}

		fmt.Println("Message received " + string(data))

		for {
			message, remainder := parseTextMessageFormat(data)

			hubMessage := HubMessage{}
			json.Unmarshal(message, &hubMessage)

			switch hubMessage.Type {
			case 1:
				invocation := HubInvocationMessage{}
				json.Unmarshal(message, &invocation)

				// Dispatch invocation here
				normalized := strings.ToLower(invocation.Target)
				method := hubInfo.methods[normalized]
				in := make([]reflect.Value, method.Type().NumIn())

				for i := 0; i < method.Type().NumIn(); i++ {
					t := method.Type().In(i)
					arg := reflect.New(t)
					json.Unmarshal(invocation.Arguments[i], arg.Interface())
					in[i] = arg.Elem()
				}

				method.Call(in)

				break
			case 6:
				// Ping
				break
			}

			// TODO: Fix partial messages
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

func MapHub(path string, hub Hub) {
	hubInfo := HubInfo{
		hub:     &hub,
		methods: make(map[string]reflect.Value),
	}

	hubType := reflect.TypeOf(hub)
	hubValue := reflect.ValueOf(hub)

	for i := 0; i < hubType.NumMethod(); i++ {
		m := hubType.Method(i)
		hubInfo.methods[strings.ToLower(m.Name)] = hubValue.Method(i)
	}

	http.HandleFunc(fmt.Sprintf("%s/negotiate", path), negotiateHandler)
	http.Handle(path, websocket.Handler(func(ws *websocket.Conn) {
		hubConnectionHandler(ws, &hubInfo)
	}))
}
