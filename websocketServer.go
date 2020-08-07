package signalr

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"golang.org/x/net/websocket"
	"io/ioutil"
	"net/http"
)

// MapHub used to register a SignalR Hub with the specified ServeMux
func MapHub(mux *http.ServeMux, path string, options ...func(party) error) Server {
	mux.HandleFunc(fmt.Sprintf("%s/negotiate", path), negotiateHandler)
	server, _ := NewServer(options...)
	mux.Handle(path, websocket.Server{
		// Use custom Handshake. Default with websocket.Handler is to reject nil origin.
		// This not useful when testing using the typescript client outside the browser (e.g. in node.js)
		Handshake: func(config *websocket.Config, req *http.Request) (err error) {
			config.Origin, err = websocket.Origin(config, req)
			return err
		},
		Handler: func(ws *websocket.Conn) {
			connectionID := ws.Request().URL.Query().Get("id")
			if len(connectionID) == 0 {
				// Support websocket connection without negotiateWebSocketTestServer
				connectionID = NewConnectionId()
			}
			server.Run(context.TODO(), &webSocketConnection{ws, connectionID, 0})
		},
	})
	return server
}

func negotiateHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		w.WriteHeader(400)
	} else {
		slurp, _ := ioutil.ReadAll(req.Body)
		fmt.Printf("%v", string(slurp))
		response := negotiateResponse{
			ConnectionID: NewConnectionId(),
			AvailableTransports: []availableTransport{
				{
					Transport:       "WebSockets",
					TransferFormats: []string{"Text", "Binary"},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(response) // Can't imagine an error when encoding
	}
}

func NewConnectionId() string {
	bytes := make([]byte, 16)
	// rand.Read only fails when the systems random number generator fails. Rare case, ignore
	_, _ = rand.Read(bytes)
	return base64.StdEncoding.EncodeToString(bytes)
}

type availableTransport struct {
	Transport       string   `json:"transport"`
	TransferFormats []string `json:"transferFormats"`
}

type negotiateResponse struct {
	ConnectionID        string               `json:"connectionId"`
	AvailableTransports []availableTransport `json:"availableTransports"`
}
