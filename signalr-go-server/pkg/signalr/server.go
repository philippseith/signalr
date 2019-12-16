package signalr

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

// MapHub used to register a SignalR Hub with the specified ServeMux
func MapHub(mux *http.ServeMux, path string, hubPrototype HubInterface) {
	lifetimeManager := defaultHubLifetimeManager{}
	defaultHubClients := defaultHubClients{
		lifetimeManager: &lifetimeManager,
		allCache:        allClientProxy{lifetimeManager: &lifetimeManager},
	}

	groupManager := defaultGroupManager{
		lifetimeManager: &lifetimeManager,
	}

	hubType := reflect.TypeOf(hubPrototype)

	mux.HandleFunc(fmt.Sprintf("%s/negotiate", path), negotiateHandler)
	mux.Handle(path, websocket.Handler(func(ws *websocket.Conn) {
		connectionID := ws.Request().URL.Query().Get("id")

		if len(connectionID) == 0 {
			// Support websocket connection without negotiate
			connectionID = getConnectionID()
		}

		// Copy hubPrototype
		hub := reflect.New(reflect.ValueOf(hubPrototype).Elem().Type()).Interface().(HubInterface)

		hubContext := &defaultHubContext{
			clients: &contextHubClients{
				defaultHubClients: &defaultHubClients,
				connectionID:      connectionID,
			},
			groups: &groupManager,
		}

		hub.Initialize(hubContext)

		hubInfo := &hubInfo{
			hub:             hub,
			lifetimeManager: &lifetimeManager,
			methods:         make(map[string]reflect.Value),
		}

		hubValue := reflect.ValueOf(hub)
		for i := 0; i < hubType.NumMethod(); i++ {
			m := hubType.Method(i)
			hubInfo.methods[strings.ToLower(m.Name)] = hubValue.Method(i)
		}

		hubConnectionHandler(connectionID, ws, hubInfo)
	}))
}

// Implementation

type hubInfo struct {
	hub             HubInterface
	lifetimeManager HubLifetimeManager
	methods         map[string]reflect.Value
}

func hubConnectionHandler(connectionID string, ws *websocket.Conn, hubInfo *hubInfo) {
	var err error
	var data []byte
	var waitgroup sync.WaitGroup
	var buf bytes.Buffer
	var streamCancels = make(map[string]chan bool)

	protocol, err := processHandshake(ws, &buf)

	if err != nil {
		fmt.Println(err)
		return
	}

	conn := webSocketHubConnection{protocol: protocol, connectionID: connectionID, ws: ws}
	conn.start()

	hubInfo.lifetimeManager.OnConnected(&conn)
	hubInfo.hub.OnConnected(connectionID)

	// Start sending pings to the client
	waitgroup.Add(1)
	go pingLoop(&waitgroup, &conn)

	for conn.isConnected() {
		for {
			message, err := protocol.ReadMessage(&buf)

			if err != nil {
				// Partial message, need more data
				break
			}

			switch message.(type) {
			case hubInvocationMessage:
				invocation := message.(hubInvocationMessage)

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

				result := func() []reflect.Value {
					defer func() {
						if err := recover(); err != nil {
							conn.completion(invocation.InvocationID, nil, fmt.Sprintf("%v\n%v", err, string(debug.Stack())))
						}
					}()
					return method.Call(in)
				}()

				// if the hub method returns a chan, it should be considered asynchronous or source for a stream
				if len(result) == 1 && result[0].Kind() == reflect.Chan {
					switch invocation.Type {
					// Simple invocation
					case 1:
						go func() {
							if chanResult, ok := result[0].Recv(); ok {
								invokeConnection(conn, invocation, completion, []reflect.Value{chanResult})
							} else {
								conn.completion(invocation.InvocationID, nil, "go channel closed")
							}
						}()
					// StreamInvocation
					case 4:
						cancelChan := make(chan bool)
						streamCancels[invocation.InvocationID] = cancelChan
						go func(cancelChan chan bool) {
							for {
								select {
								case <-cancelChan:
									return
								default:
								}
								if chanResult, ok := result[0].Recv(); ok {
									conn.streamItem(invocation.InvocationID, chanResult.Interface())
								} else {
									conn.completion(invocation.InvocationID, nil, "")
									break
								}
							}
						}(cancelChan)
					}
				} else {
					switch invocation.Type {
					// Simple invocation
					case 1:
						invokeConnection(conn, invocation, completion, result)
					case 4:
						// Stream invocation
						invokeConnection(conn, invocation, streamItem, result)
					}
				}
			case cancelInvocationMessage:
				cancelInvocation := message.(cancelInvocationMessage)
				if cancel, ok := streamCancels[cancelInvocation.InvocationID]; ok {
					cancel <- true
					delete(streamCancels, cancelInvocation.InvocationID)
				}
			case hubMessage:
				// Ping
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

	hubInfo.hub.OnDisconnected(connectionID)
	hubInfo.lifetimeManager.OnDisconnected(&conn)
	conn.close("")

	// Wait for pings to complete
	waitgroup.Wait()
}

func processHandshake(ws *websocket.Conn, buf *bytes.Buffer) (HubProtocol, error) {
	var err error
	var data []byte
	var protocol HubProtocol
	var ok bool
	const handshakeResponse = "{}\u001e"
	const errorHandshakeResponse = "{\"error\":\"%s\"}\u001e"

	// 5 seconds to process the handshake
	ws.SetReadDeadline(time.Now().Add(5 * time.Second))

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

		request := handshakeRequest{}
		err = json.Unmarshal(rawHandshake, &request)

		if err != nil {
			// Malformed handshake
			break
		}

		protocol, ok = protocolMap[request.Protocol]

		if ok {
			// Send the handshake response
			err = websocket.Message.Send(ws, handshakeResponse)
		} else {
			// Protocol not supported
			fmt.Printf("\"%s\" is the only supported protocol\n", request.Protocol)
			err = websocket.Message.Send(ws, fmt.Sprintf(errorHandshakeResponse, fmt.Sprintf("Protocol \"%s\" not supported", request.Protocol)))
		}
		break
	}

	// Disable the timeout (either we already timeout out or)
	ws.SetReadDeadline(time.Time{})

	return protocol, err
}
type connFunc func(conn webSocketHubConnection, invocation hubInvocationMessage, value interface{})

func completion(conn webSocketHubConnection, invocation hubInvocationMessage, value interface{}) {
	conn.completion(invocation.InvocationID, value, "")
}

func streamItem(conn webSocketHubConnection, invocation hubInvocationMessage, value interface{}) {
	conn.streamItem(invocation.InvocationID, value)
}

func invokeConnection(conn webSocketHubConnection, invocation hubInvocationMessage, connFunc connFunc, result []reflect.Value) {
	values := make([]interface{}, len(result))
	for i, rv := range result {
		values[i] = rv.Interface()
	}
	switch len(result) {
	case 0:
		conn.completion(invocation.InvocationID, nil, "")
	case 1:
		connFunc(conn, invocation, values[0])
	default:
		connFunc(conn, invocation, values)
	}
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
	defer waitGroup.Done()

	for conn.isConnected() {
		conn.ping()
		time.Sleep(5 * time.Second)
	}
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
	if req.Method != "POST" {
		w.WriteHeader(400)
		return
	}

	connectionID := getConnectionID()

	response := negotiateResponse{
		ConnectionID: connectionID,
		AvailableTransports: []availableTransport{
			{
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

var protocolMap = map[string]HubProtocol {
	"json": &jsonHubProtocol{},
}


