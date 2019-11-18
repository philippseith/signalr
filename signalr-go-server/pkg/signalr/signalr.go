package signalr

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/websocket"
)

// Protocol
type HubMessage struct {
	Type int `json:"type"`
}

type HubInvocationMessage struct {
	Type         int               `json:"type"`
	Target       string            `json:"target"`
	InvocationID string            `json:"invocationId"`
	Arguments    []json.RawMessage `json:"arguments"`
}

type SendOnlyHubInvocationMessage struct {
	Type      int               `json:"type"`
	Target    string            `json:"target"`
	Arguments []json.RawMessage `json:"arguments"`
}

type CompletionMessage struct {
	Type         int         `json:"type"`
	InvocationID string      `json:"invocationId"`
	Result       interface{} `json:"result"`
	Error        string      `json:"error"`
}

type PingMessage struct {
	Type int `json:"type"`
}

// Hub
type Hub interface {
	Initialize(clients HubClients)
}

type HubLifetimeManager interface {
	OnConnected(conn hubConnection)
	OnDisconnected(conn hubConnection)
	InvokeAll(target string, args []interface{})
	InvokeClient(connectionID string, target string, args []interface{})
}

type ClientProxy interface {
	Send(target string, args ...interface{})
}

type HubClients interface {
	All() ClientProxy
	Client(id string) ClientProxy
}

// Implementation

type defaultHubLifetimeManager struct {
	clients sync.Map
}

func (d *defaultHubLifetimeManager) OnConnected(conn hubConnection) {
	d.clients.Store(conn.getConnectionID(), conn)
}

func (d *defaultHubLifetimeManager) OnDisconnected(conn hubConnection) {
	d.clients.Delete(conn.getConnectionID())
}

func (d *defaultHubLifetimeManager) InvokeAll(target string, args []interface{}) {
	d.clients.Range(func(key, value interface{}) bool {
		value.(hubConnection).sendInvocation(target, args)
		return true
	})
}

func (d *defaultHubLifetimeManager) InvokeClient(connectionID string, target string, args []interface{}) {
	client, ok := d.clients.Load(connectionID)

	if !ok {
		return
	}

	client.(hubConnection).sendInvocation(target, args)
}

type hubInfo struct {
	hub             *Hub
	lifetimeManager HubLifetimeManager
	methods         map[string]reflect.Value
}

type allClientProxy struct {
	lifetimeManager HubLifetimeManager
}

func (a *allClientProxy) Send(target string, args ...interface{}) {
	a.lifetimeManager.InvokeAll(target, args)
}

type singleClientProxy struct {
	id              string
	lifetimeManager HubLifetimeManager
}

func (a *singleClientProxy) Send(target string, args ...interface{}) {
	a.lifetimeManager.InvokeClient(a.id, target, args)
}

type defaultHubClients struct {
	lifetimeManager HubLifetimeManager
	allCache        allClientProxy
}

func (c *defaultHubClients) All() ClientProxy {
	return &c.allCache
}

func (c *defaultHubClients) Client(id string) ClientProxy {
	return &singleClientProxy{id: id, lifetimeManager: c.lifetimeManager}
}

type handshakeRequest struct {
	Protocol string `json:"protocol"`
	Version  int    `json:"version"`
}

type hubConnection interface {
	isConnected() bool
	getConnectionID() string
	sendInvocation(target string, args []interface{})
	completion(id string, result interface{}, error string)
	ping()
}

type webSocketHubConnection struct {
	ws           *websocket.Conn
	connectionID string
	connected    int32
}

func (w *webSocketHubConnection) start() {
	atomic.CompareAndSwapInt32(&w.connected, 0, 1)
}

func (w *webSocketHubConnection) isConnected() bool {
	return atomic.LoadInt32(&w.connected) == 1
}

func (w *webSocketHubConnection) getConnectionID() string {
	return w.connectionID
}

func (w *webSocketHubConnection) sendInvocation(target string, args []interface{}) {
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

func (w *webSocketHubConnection) completion(id string, result interface{}, error string) {
	var completionMessage = CompletionMessage{
		Type:         3,
		InvocationID: id,
		Result:       result,
		Error:        error,
	}

	var payload, _ = json.Marshal(&completionMessage)
	websocket.Message.Send(w.ws, string(payload)+"\u001e")
}

func (w *webSocketHubConnection) ping() {
	var pingMessage = PingMessage{
		Type: 6,
	}
	var payload, _ = json.Marshal(&pingMessage)
	websocket.Message.Send(w.ws, string(payload)+"\u001e")
}

func (w *webSocketHubConnection) close() {
	atomic.StoreInt32(&w.connected, 0)
}

func hubConnectionHandler(ws *websocket.Conn, hubInfo *hubInfo) {
	var err error
	var data []byte
	handshake := false
	var waitgroup sync.WaitGroup

	id := ws.Request().URL.Query().Get("id")
	conn := webSocketHubConnection{connectionID: id, ws: ws}

	for {
		if err = websocket.Message.Receive(ws, &data); err != nil {
			fmt.Println(err)
			break
		}

		if !handshake {
			rawHandshake, remainder := parseTextMessageFormat(data)

			if len(remainder) > 0 {
				fmt.Println("Can't handle partial messages yet...I'm lazy")
				return
			}

			fmt.Println("Handshake received")

			request := handshakeRequest{}
			json.Unmarshal(rawHandshake, &request)

			var handshakeResponse = "{}\u001e"

			// Send the handshake response (it's a string so it sends text back)
			websocket.Message.Send(ws, handshakeResponse)

			conn.start()
			hubInfo.lifetimeManager.OnConnected(&conn)

			// Start sending pings to the client
			waitgroup.Add(1)
			go pingLoop(&waitgroup, &conn)

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

				method, ok := hubInfo.methods[normalized]

				if !ok {
					// Unable to find the method
					conn.completion(invocation.InvocationID, nil, fmt.Sprintf("Unknown method %s", invocation.Target))
					break
				}

				in := make([]reflect.Value, method.Type().NumIn())

				for i := 0; i < method.Type().NumIn(); i++ {
					t := method.Type().In(i)
					arg := reflect.New(t)
					json.Unmarshal(invocation.Arguments[i], arg.Interface())
					in[i] = arg.Elem()
				}

				// TODO: Handle return values
				method.Call(in)

				conn.completion(invocation.InvocationID, nil, "")

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
	conn.close()

	// Wait for pings to complete
	waitgroup.Wait()
}

func pingLoop(waitGroup *sync.WaitGroup, conn hubConnection) {
	for conn.isConnected() {
		conn.ping()
		time.Sleep(5 * time.Second)
	}

	waitGroup.Done()
}

func parseTextMessageFormat(data []byte) ([]byte, []byte) {
	index := bytes.Index(data, []byte{30})

	// TODO: Handle -1

	return data[0:index], data[index+1:]
}

type availableTransport struct {
	Transport       string   `json:"transport"`
	TransferFormats []string `json:"transferFormats"`
}

type negotiateResponse struct {
	ConnectionID        string               `json:"connectionId"`
	AvailableTransports []availableTransport `json:"availableTransports"`
}

func negotiateHandler(w http.ResponseWriter, req *http.Request) {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	connectionID := base64.StdEncoding.EncodeToString(bytes)

	response := negotiateResponse{
		ConnectionID: connectionID,
		AvailableTransports: []availableTransport{
			availableTransport{
				Transport:       "WebSockets",
				TransferFormats: []string{"Text", "Binary"},
			},
		},
	}

	json.NewEncoder(w).Encode(response)
}

func MapHub(path string, hub Hub) {
	lifetimeManager := defaultHubLifetimeManager{}
	hubClients := defaultHubClients{
		lifetimeManager: &lifetimeManager,
		allCache:        allClientProxy{lifetimeManager: &lifetimeManager},
	}

	hubInfo := hubInfo{
		hub:             &hub,
		lifetimeManager: &lifetimeManager,
		methods:         make(map[string]reflect.Value),
	}

	hub.Initialize(&hubClients)

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
