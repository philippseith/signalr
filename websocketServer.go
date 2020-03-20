package signalr

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"golang.org/x/net/websocket"
	"net/http"
)

// MapHub used to register a SignalR Hub with the specified ServeMux
func MapHub(mux *http.ServeMux, path string, options ...func(*Server) error) *Server {
	mux.HandleFunc(fmt.Sprintf("%s/negotiate", path), negotiateHandler)
	server, _ := NewServer(options...)
	mux.Handle(path, websocket.Handler(func(ws *websocket.Conn) {
		connectionID := ws.Request().URL.Query().Get("id")
		if len(connectionID) == 0 {
			// Support websocket connection without negotiateWebSocketTestServer
			connectionID = getConnectionID()
		}
		server.Run(context.TODO(), &webSocketConnection{ws, connectionID, 0})
	}))
	return server
}

func negotiateHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		w.WriteHeader(400)
	} else {
		response := negotiateResponse{
			ConnectionID: getConnectionID(),
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

func getConnectionID() string {
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
