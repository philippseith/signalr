package signalr

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"golang.org/x/net/websocket"
	"net/http"
	"strings"
	"sync"
)

type httpMux struct {
	mx            sync.Mutex
	connectionMap map[string]cancelableConnection
	server        Server
	wsServer      websocket.Server
}

func newHttpMux(server Server) *httpMux {
	return &httpMux{
		connectionMap: make(map[string]cancelableConnection),
		server:        server,
	}
}

func (h *httpMux) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case "POST":
		h.handlePost(writer, request)
	case "GET":
		h.handleGet(writer, request)
	default:
		writer.WriteHeader(400)
	}
}

func (h *httpMux) handlePost(writer http.ResponseWriter, request *http.Request) {
	connectionID := request.URL.Query().Get("id")
	if connectionID == "" {
		writer.WriteHeader(400) // Bad request
		return
	}
	h.mx.Lock()
	c, ok := h.connectionMap[connectionID]
	h.mx.Unlock()
	if ok {
		if c == nil {
			// Connection is negotiated but not initiated
			if request.Header.Get("Accept") == "text/event-stream" {
				h.serveConnection(newServerSentEventConnection())
			} else {
				//  TODO Long polling
			}
		} else {
			// Connection is initiated
			switch conn := c.(type) {
			case *serverSentEventConnection:
				writer.WriteHeader(conn.consumeRequest(request))
			// TODO case longPolling
			default:
				// ConnectionID for WebSocket
				writer.WriteHeader(409) // Conflict
			}
		}
	} else {
		writer.WriteHeader(404) // Not found
	}
}

func (h *httpMux) handleGet(writer http.ResponseWriter, request *http.Request) {
	upgrade := false
	for _, connHead := range strings.Split(request.Header.Get("Connection"), ",") {
		if strings.ToLower(strings.TrimSpace(connHead)) == "upgrade" {
			upgrade = true
			break
		}
	}
	if upgrade &&
		strings.ToLower(request.Header.Get("Upgrade")) == "websocket" {
		h.wsServer = websocket.Server{
			// Use custom Handshake. Default with websocket.Handler is to reject nil origin.
			// This not useful when testing using the typescript client outside the browser (e.g. in node.js)
			// or any other client with origin not set.
			Handshake: func(config *websocket.Config, req *http.Request) (err error) {
				config.Origin, err = websocket.Origin(config, req)
				return err
			},
			Handler: h.handleWebsocket,
		}
		h.wsServer.ServeHTTP(writer, request)
	} else {
		writer.WriteHeader(400)
	}
}

func (h *httpMux) handleWebsocket(ws *websocket.Conn) {
	connectionID := ws.Request().URL.Query().Get("id")
	if connectionID == "" {
		// Support websocket connection without negotiate
		connectionID = newConnectionID()
		h.mx.Lock()
		h.connectionMap[connectionID] = nil
		h.mx.Unlock()
	}
	h.mx.Lock()
	c, ok := h.connectionMap[connectionID]
	h.mx.Unlock()
	if ok {
		if c == nil {
			// Connection is negotiated but not initiated
			h.serveConnection(newWebSocketConnection(connectionID, ws))
		} else {
			// Already initiated
			_ = ws.WriteClose(409) // Bad request
			// TODO Test client and server reaction (should we call c.cancel()?)
		}
	} else {
		_ = ws.WriteClose(404) // Not found
		// TODO Test client and server reaction (should we call c.cancel()?)
	}

}

func (h *httpMux) negotiate(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		w.WriteHeader(400)
	} else {
		connectionID := newConnectionID()
		h.mx.Lock()
		h.connectionMap[connectionID] = nil
		h.mx.Unlock()
		response := negotiateResponse{
			ConnectionID: connectionID,
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

func (h *httpMux) serveConnection(cc cancelableConnection) {
	h.mx.Lock()
	cc.initCancel(h.server.context())
	h.connectionMap[cc.ConnectionID()] = cc
	h.mx.Unlock()
	h.server.ServeConnection(cc.context(), cc)
}

func newConnectionID() string {
	bytes := make([]byte, 16)
	// rand.Read only fails when the systems random number generator fails. Rare case, ignore
	_, _ = rand.Read(bytes)
	// Important: Use URLEncoding. StdEncoding contains "/" which will be randomly part of the connectionID and cause parsing problems
	return base64.URLEncoding.EncodeToString(bytes)
}

type availableTransport struct {
	Transport       string   `json:"transport"`
	TransferFormats []string `json:"transferFormats"`
}

type negotiateResponse struct {
	ConnectionID        string               `json:"connectionId"`
	AvailableTransports []availableTransport `json:"availableTransports"`
}
