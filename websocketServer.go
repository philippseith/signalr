package signalr

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"golang.org/x/net/websocket"
	"net/http"
	"sync"
	"time"
)

// MapHub used to register a SignalR Hub with the specified ServeMux
func MapHub(mux *http.ServeMux, path string, hubPrototype HubInterface) {
	mux.HandleFunc(fmt.Sprintf("%s/negotiate", path), negotiateHandler)
	server := NewServer(hubPrototype)
	mux.Handle(path, websocket.Handler(func(ws *websocket.Conn) {
		connectionID := ws.Request().URL.Query().Get("id")
		if len(connectionID) == 0 {
			// Support websocket connection without negotiate
			connectionID = getConnectionID()
		}
		if protocol, err := processHandshake(ws); err != nil {
			fmt.Println(err)
		} else {
			conn := newHubConnection(&webSocketConnection{ws, nil}, connectionID, protocol)
			// start sending pings to the client
			pings := startPingClientLoop(conn)
			conn.Start()
			// Process messages
			server.messageLoop(conn, connectionID, protocol)
			conn.Close("")
			// Wait for pings to complete
			pings.Wait()
		}
	}))
}

func processHandshake(ws *websocket.Conn) (HubProtocol, error) {
	var err error
	var data []byte
	var protocol HubProtocol
	var ok bool
	const handshakeResponse = "{}\u001e"
	const errorHandshakeResponse = "{\"error\":\"%s\"}\u001e"

	// 5 seconds to process the handshake
	ws.SetReadDeadline(time.Now().Add(5 * time.Second))

	var buf bytes.Buffer
	for {
		if err = websocket.Message.Receive(ws, &data); err != nil {
			break
		}

		buf.Write(data)

		rawHandshake, err := parseTextMessageFormat(&buf)

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
			fmt.Printf("\"%s\" is the only supported Protocol\n", request.Protocol)
			err = websocket.Message.Send(ws, fmt.Sprintf(errorHandshakeResponse, fmt.Sprintf("Protocol \"%s\" not supported", request.Protocol)))
		}
		break
	}

	// Disable the timeout (either we already timeout out or)
	ws.SetReadDeadline(time.Time{})

	return protocol, err
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

var protocolMap = map[string]HubProtocol{
	"json": &JsonHubProtocol{},
}

type availableTransport struct {
	Transport       string   `json:"transport"`
	TransferFormats []string `json:"transferFormats"`
}

type negotiateResponse struct {
	ConnectionID        string               `json:"connectionId"`
	AvailableTransports []availableTransport `json:"availableTransports"`
}

func getConnectionID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return base64.StdEncoding.EncodeToString(bytes)
}

func startPingClientLoop(conn hubConnection) *sync.WaitGroup {
	var waitgroup sync.WaitGroup
	waitgroup.Add(1)
	go func(waitGroup *sync.WaitGroup, conn hubConnection) {
		defer waitGroup.Done()

		for conn.IsConnected() {
			conn.Ping()
			time.Sleep(5 * time.Second)
		}
	}(&waitgroup, conn)
	return &waitgroup
}

