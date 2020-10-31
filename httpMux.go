package signalr

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
		// Connection is initiated
		switch conn := c.(type) {
		case *serverSentEventConnection:
			writer.WriteHeader(conn.consumeRequest(request))
		// TODO case longPolling
		default:
			// ConnectionID for WebSocket or
			writer.WriteHeader(409) // Conflict
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
	} else if strings.ToLower(request.Header.Get("Accept")) == "text/event-stream" {
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
				// Check for SSE
				if strings.ToLower(request.Header.Get("Accept")) == "text/event-stream" {
					// We compose http and send it over sse
					writer.Header().Set("Content-Type", "text/event-stream")
					writer.Header().Set("Connection", "keep-alive")
					writer.Header().Set("Cache-Control", "no-cache")
					writer.WriteHeader(200)
					// End this Server Sent Event (yes, your response now is one and the client will wait for this initial event to end)
					_, _ = fmt.Fprint(writer, ":\r\n\r\n")
					writer.(http.Flusher).Flush()
					h.serveConnection(newServerSentEventConnection(connectionID, writer))
				} else {
					//  TODO Long polling
				}
			} else {
				// connectionID in use
				writer.WriteHeader(409) // Conflict
			}
		} else {
			writer.WriteHeader(404) // Not found
		}
	} else {
		writer.WriteHeader(400) // Bad request
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
		var availableTransports []availableTransport
		for _, transport := range h.server.availableTransports() {
			switch transport {
			case "ServerSentEvents":
				availableTransports = append(availableTransports,
					availableTransport{
						Transport:       "ServerSentEvents",
						TransferFormats: []string{"Text"},
					})
			case "WebSockets":
				availableTransports = append(availableTransports,
					availableTransport{
						Transport:       "WebSockets",
						TransferFormats: []string{"Text"},
						// TODO TransferFormats: []string{"Text", "Binary"},
					})
			}
		}
		response := negotiateResponse{
			ConnectionID:        connectionID,
			AvailableTransports: availableTransports,
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
