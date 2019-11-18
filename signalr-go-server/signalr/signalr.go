package signalr

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"

	"golang.org/x/net/websocket"
)

// Protocol
type HubMessage struct {
	Type int `json:"type"`
}

type HubInvocationMessage struct {
	Type         int               `json:"type"`
	Target       string            `json:"target"`
	InvocationId string            `json:"invocationId"`
	Arguments    []json.RawMessage `json:"arguments"`
}

type SendOnlyHubInvocationMessage struct {
	Type      int               `json:"type"`
	Target    string            `json:"target"`
	Arguments []json.RawMessage `json:"arguments"`
}

type CompletionMessage struct {
	Type         int         `json:"type"`
	InvocationId string      `json:"invocationId"`
	Result       interface{} `json:"result"`
	Error        string      `json:"error"`
}

// Hub
type Hub interface {
	Initialize(clients HubClients)
}

type HubLifetimeManager interface {
	OnConnected(conn HubConnection)
	OnDisconnected(conn HubConnection)
	InvokeAll(target string, args []interface{})
}

// Implementation

type DefaultHubLifetimeManager struct {
	mu      sync.Mutex
	clients map[string]HubConnection
}

func (self *DefaultHubLifetimeManager) OnConnected(conn HubConnection) {
	self.mu.Lock()
	self.clients[conn.getConnectionId()] = conn
	self.mu.Unlock()
}

func (self *DefaultHubLifetimeManager) OnDisconnected(conn HubConnection) {
	self.mu.Lock()
	delete(self.clients, conn.getConnectionId())
	self.mu.Unlock()
}

func (self *DefaultHubLifetimeManager) InvokeAll(target string, args []interface{}) {
	self.mu.Lock()
	for _, v := range self.clients {
		v.sendInvocation(target, args)
	}
	self.mu.Unlock()
}

type HubInfo struct {
	hub             *Hub
	lifetimeManager HubLifetimeManager
	methods         map[string]reflect.Value
}

type AllClientProxy struct {
	lifetimeManager HubLifetimeManager
}

func (a *AllClientProxy) Send(target string, args ...interface{}) {
	a.lifetimeManager.InvokeAll(target, args)
}

type HubClients struct {
	All AllClientProxy
}

func newHubClients(lifetimeManager HubLifetimeManager) HubClients {
	return HubClients{All: AllClientProxy{lifetimeManager: lifetimeManager}}
}

type HandshakeRequest struct {
	Protocol string `json:"protocol"`
	Version  int    `json:"version"`
}

type HubConnection interface {
	getConnectionId() string
	sendInvocation(target string, args []interface{})
	completion(id string, result interface{}, error string)
}

type WebSocketHubConnection struct {
	ws           *websocket.Conn
	connectionId string
}

func (w *WebSocketHubConnection) getConnectionId() string {
	return w.connectionId
}

func (w *WebSocketHubConnection) sendInvocation(target string, args []interface{}) {
	var values = make([]json.RawMessage, len(args))
	for i := 0; i < len(args); i++ {
		values[i], _ = json.Marshal(args[i])
	}
	var invocationMessage = SendOnlyHubInvocationMessage{
		Type:      1,
		Target:    target,
		Arguments: values,
	}

	var payload, _ = json.Marshal(&invocationMessage)
	websocket.Message.Send(w.ws, string(payload)+"\u001e")
}

func (w *WebSocketHubConnection) completion(id string, result interface{}, error string) {
	var completionMessage = CompletionMessage{
		InvocationId: id,
		Result:       result,
		Error:        error,
	}

	var payload, _ = json.Marshal(&completionMessage)
	websocket.Message.Send(w.ws, string(payload)+"\u001e")
}

func hubConnectionHandler(ws *websocket.Conn, hubInfo *HubInfo) {
	var err error
	var data []byte
	handshake := false

	id := ws.Request().URL.Query().Get("id")
	conn := WebSocketHubConnection{connectionId: id, ws: ws}

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

			var handshakeResponse = "{}\u001e"

			// Send the handshake response (it's a string so it sends text back)
			websocket.Message.Send(ws, handshakeResponse)

			hubInfo.lifetimeManager.OnConnected(&conn)

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

				// TODO: Handle unknown methods
				method := hubInfo.methods[normalized]
				in := make([]reflect.Value, method.Type().NumIn())

				for i := 0; i < method.Type().NumIn(); i++ {
					t := method.Type().In(i)
					arg := reflect.New(t)
					json.Unmarshal(invocation.Arguments[i], arg.Interface())
					in[i] = arg.Elem()
				}

				// TODO: Handle return values
				method.Call(in)

				conn.completion(invocation.Target, nil, "")

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

	hubInfo.lifetimeManager.OnDisconnected(&conn)
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
	lifetimeManager := DefaultHubLifetimeManager{
		clients: make(map[string]HubConnection),
	}
	hubClients := newHubClients(&lifetimeManager)

	hubInfo := HubInfo{
		hub:             &hub,
		lifetimeManager: &lifetimeManager,
		methods:         make(map[string]reflect.Value),
	}

	hub.Initialize(hubClients)

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
