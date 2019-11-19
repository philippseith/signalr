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

// Hub
type Hub interface {
	Initialize(hubContext HubContext)
}

type HubLifetimeManager interface {
	OnConnected(conn hubConnection)
	OnDisconnected(conn hubConnection)
	InvokeAll(target string, args []interface{})
	InvokeClient(connectionID string, target string, args []interface{})
	InvokeGroup(groupName string, target string, args []interface{})
	AddToGroup(groupName, connectionID string)
	RemoveFromGroup(groupName, connectionID string)
}

type ClientProxy interface {
	Send(target string, args ...interface{})
}

type HubClients interface {
	All() ClientProxy
	Client(id string) ClientProxy
	Group(groupName string) ClientProxy
}

type GroupManager interface {
	AddToGroup(groupName string, connectionID string)
	RemoveFromGroup(groupName string, connectionID string)
}

type HubContext interface {
	Clients() HubClients
	Groups() GroupManager
}

// Implementation

// If the hub has these methods, we'll call them with the connection information
type hubEventHandler interface {
	OnConnected(id string)
	OnDisconnected(id string)
}

type defaultHubContext struct {
	clients HubClients
	groups  GroupManager
}

func (d *defaultHubContext) Clients() HubClients {
	return d.clients
}

func (d *defaultHubContext) Groups() GroupManager {
	return d.groups
}

type defaultHubLifetimeManager struct {
	clients sync.Map
	groups  sync.Map
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

func (d *defaultHubLifetimeManager) InvokeGroup(groupName string, target string, args []interface{}) {
	groups, ok := d.groups.Load(groupName)

	if !ok {
		// No such group
		return
	}

	for _, v := range groups.(map[string]hubConnection) {
		v.sendInvocation(target, args)
	}
}

func (d *defaultHubLifetimeManager) AddToGroup(groupName string, connectionID string) {
	client, ok := d.clients.Load(connectionID)

	if !ok {
		// No such client
		return
	}

	groups, _ := d.groups.LoadOrStore(groupName, make(map[string]hubConnection))

	groups.(map[string]hubConnection)[connectionID] = client.(hubConnection)
}

func (d *defaultHubLifetimeManager) RemoveFromGroup(groupName string, connectionID string) {
	groups, ok := d.groups.Load(groupName)

	if !ok {
		return
	}

	delete(groups.(map[string]hubConnection), connectionID)
}

type defaultGroupManager struct {
	lifetimeManager HubLifetimeManager
}

func (d *defaultGroupManager) AddToGroup(groupName string, connectionID string) {
	d.lifetimeManager.AddToGroup(groupName, connectionID)
}

func (d *defaultGroupManager) RemoveFromGroup(groupName string, connectionID string) {
	d.lifetimeManager.RemoveFromGroup(groupName, connectionID)
}

type hubInfo struct {
	hub             Hub
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

type groupClientProxy struct {
	groupName       string
	lifetimeManager HubLifetimeManager
}

func (g *groupClientProxy) Send(target string, args ...interface{}) {
	g.lifetimeManager.InvokeGroup(g.groupName, target, args)
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

func (c *defaultHubClients) Group(groupName string) ClientProxy {
	return &groupClientProxy{groupName: groupName, lifetimeManager: c.lifetimeManager}
}

// Protocol
type hubMessage struct {
	Type int `json:"type"`
}

type hubInvocationMessage struct {
	Type         int               `json:"type"`
	Target       string            `json:"target"`
	InvocationID string            `json:"invocationId"`
	Arguments    []json.RawMessage `json:"arguments"`
}

type sendOnlyHubInvocationMessage struct {
	Type      int               `json:"type"`
	Target    string            `json:"target"`
	Arguments []json.RawMessage `json:"arguments"`
}

type completionMessage struct {
	Type         int         `json:"type"`
	InvocationID string      `json:"invocationId"`
	Result       interface{} `json:"result"`
	Error        string      `json:"error"`
}

type closeMessage struct {
	Type           int    `json:"type"`
	Error          string `json:"error"`
	AllowReconnect bool   `json:"allowReconnect"`
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
	var invocationMessage = sendOnlyHubInvocationMessage{
		Type:      1,
		Target:    target,
		Arguments: values,
	}

	var payload, _ = json.Marshal(&invocationMessage)
	websocket.Message.Send(w.ws, string(payload)+"\u001e")
}

func (w *webSocketHubConnection) completion(id string, result interface{}, error string) {
	var completionMessage = completionMessage{
		Type:         3,
		InvocationID: id,
		Result:       result,
		Error:        error,
	}

	var payload, _ = json.Marshal(&completionMessage)
	websocket.Message.Send(w.ws, string(payload)+"\u001e")
}

func (w *webSocketHubConnection) ping() {
	var pingMessage = hubMessage{
		Type: 6,
	}
	var payload, _ = json.Marshal(&pingMessage)
	websocket.Message.Send(w.ws, string(payload)+"\u001e")
}

func (w *webSocketHubConnection) close(error string) {
	atomic.StoreInt32(&w.connected, 0)

	var closeMessage = closeMessage{
		Type:           6,
		Error:          error,
		AllowReconnect: true,
	}
	var payload, _ = json.Marshal(&closeMessage)
	websocket.Message.Send(w.ws, string(payload)+"\u001e")
}

func processHandshake(ws *websocket.Conn, buf *bytes.Buffer) (handshakeRequest, error) {
	var err error
	var data []byte
	request := handshakeRequest{}
	const handshakeResponse = "{}\u001e"

	// TODO: Timeout the handshake after 5 seconds of waiting (make configurable)

	for {
		if err = websocket.Message.Receive(ws, &data); err != nil {
			break
		}

		buf.Write(data)

		rawHandshake, err := parseTextMessageFormat(buf)

		if err != nil {
			// Partial message, read more data
			continue
		}

		fmt.Println("Handshake received")

		json.Unmarshal(rawHandshake, &request)

		// Send the handshake response
		websocket.Message.Send(ws, handshakeResponse)
		break
	}

	return request, err
}

func hubConnectionHandler(connectionID string, ws *websocket.Conn, hubInfo *hubInfo) {
	var err error
	var data []byte
	var waitgroup sync.WaitGroup
	var buf bytes.Buffer

	hubEventHandler, hasEvents := hubInfo.hub.(hubEventHandler)

	conn := webSocketHubConnection{connectionID: connectionID, ws: ws}
	conn.start()

	handshake, err := processHandshake(ws, &buf)

	if err != nil {
		fmt.Println("Unable to process the handshake")
		return
	}

	if handshake.Protocol != "json" {
		fmt.Println("JSON is the only supported protocol")
		return
	}

	hubInfo.lifetimeManager.OnConnected(&conn)

	if hasEvents {
		hubEventHandler.OnConnected(connectionID)
	}

	// Start sending pings to the client
	waitgroup.Add(1)
	go pingLoop(&waitgroup, &conn)

	for conn.isConnected() {
		for {
			message, err := parseTextMessageFormat(&buf)

			if err != nil {
				// Partial message, need more data
				break
			}

			hubMessage := hubMessage{}
			json.Unmarshal(message, &hubMessage)

			switch hubMessage.Type {
			case 1:
				invocation := hubInvocationMessage{}
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

				result := method.Call(in)

				if len(result) > 0 {
					// REVIEW: When is this ever > 1
					conn.completion(invocation.InvocationID, result[0].Interface(), "")
				} else {
					conn.completion(invocation.InvocationID, nil, "")
				}

				break
			case 6:
				// Ping
				break
			}
		}

		if err = websocket.Message.Receive(ws, &data); err != nil {
			fmt.Println(err)
			break
		}

		// Main message loop
		fmt.Println("Message received " + string(data))

		buf.Write(data)
	}

	if hasEvents {
		hubEventHandler.OnDisconnected(connectionID)
	}

	hubInfo.lifetimeManager.OnDisconnected(&conn)
	conn.close("")

	// Wait for pings to complete
	waitgroup.Wait()
}

func parseTextMessageFormat(buf *bytes.Buffer) ([]byte, error) {
	// 30 = ASCII record separator
	data, err := buf.ReadBytes(30)

	if err != nil {
		return data, err
	}
	// Remove the delimeter
	return data[0 : len(data)-1], err
}

func pingLoop(waitGroup *sync.WaitGroup, conn hubConnection) {
	for conn.isConnected() {
		conn.ping()
		time.Sleep(5 * time.Second)
	}

	waitGroup.Done()
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
	connectionID := getConnectionID()

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

func getConnectionID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return base64.StdEncoding.EncodeToString(bytes)
}

func MapHub(mux *http.ServeMux, path string, hub Hub) {
	lifetimeManager := defaultHubLifetimeManager{}
	hubClients := defaultHubClients{
		lifetimeManager: &lifetimeManager,
		allCache:        allClientProxy{lifetimeManager: &lifetimeManager},
	}

	groupManager := defaultGroupManager{
		lifetimeManager: &lifetimeManager,
	}

	hubContext := defaultHubContext{
		clients: &hubClients,
		groups:  &groupManager,
	}

	hubInfo := hubInfo{
		hub:             hub,
		lifetimeManager: &lifetimeManager,
		methods:         make(map[string]reflect.Value),
	}

	hub.Initialize(&hubContext)

	hubType := reflect.TypeOf(hub)
	hubValue := reflect.ValueOf(hub)

	for i := 0; i < hubType.NumMethod(); i++ {
		m := hubType.Method(i)
		hubInfo.methods[strings.ToLower(m.Name)] = hubValue.Method(i)
	}

	mux.HandleFunc(fmt.Sprintf("%s/negotiate", path), negotiateHandler)
	mux.Handle(path, websocket.Handler(func(ws *websocket.Conn) {
		connectionID := ws.Request().URL.Query().Get("id")

		if len(connectionID) == 0 {
			// Support websocket connection without negotiate
			connectionID = getConnectionID()
		}

		hubConnectionHandler(connectionID, ws, &hubInfo)
	}))
}
