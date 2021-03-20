package signalr

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"nhooyr.io/websocket"
	"strconv"
	"strings"
	"sync"
)

// HttpMux exposes an http.Handler implementation which knows how to
// handle signalr protocol requests (e.g. requests from the signalr
// client library)
type HttpMux struct {
	mx            sync.Mutex
	connectionMap map[string]Connection
	server        Server
}

// NewHTTPMux creates a new HttpMux bound to the given Server
func NewHTTPMux(server Server) *HttpMux {
	return &HttpMux{
		connectionMap: make(map[string]Connection),
		server:        server,
	}
}

// ServeHTTP implements http.Handler
func (h *HttpMux) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case "POST":
		h.handlePost(writer, request)
	case "GET":
		h.handleGet(writer, request)
	default:
		writer.WriteHeader(400)
	}
}

func (h *HttpMux) handlePost(writer http.ResponseWriter, request *http.Request) {
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
		case *serverSSEConnection:
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

func (h *HttpMux) handleGet(writer http.ResponseWriter, request *http.Request) {
	upgrade := false
	for _, connHead := range strings.Split(request.Header.Get("Connection"), ",") {
		if strings.ToLower(strings.TrimSpace(connHead)) == "upgrade" {
			upgrade = true
			break
		}
	}
	if upgrade &&
		strings.ToLower(request.Header.Get("Upgrade")) == "websocket" {
		h.handleWebsocket(writer, request)
	} else if strings.ToLower(request.Header.Get("Accept")) == "text/event-stream" {
		h.handleServerSentEvent(writer, request)
	} else {
		writer.WriteHeader(400) // Bad request
	}
}

func (h *HttpMux) handleServerSentEvent(writer http.ResponseWriter, request *http.Request) {
	connectionID := request.URL.Query().Get("id")
	if connectionID == "" {
		writer.WriteHeader(400) // Bad request
		return
	}
	h.mx.Lock()
	c, ok := h.connectionMap[connectionID]
	h.mx.Unlock()
	if ok {
		if _, ok := c.(*negotiateConnection); ok {
			// Connection is negotiated but not initiated
			// We compose http and send it over sse
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.Header().Set("Connection", "keep-alive")
			writer.Header().Set("Cache-Control", "no-cache")
			writer.WriteHeader(200)
			// End this Server Sent Event (yes, your response now is one and the client will wait for this initial event to end)
			_, _ = fmt.Fprint(writer, ":\r\n\r\n")
			writer.(http.Flusher).Flush()
			if sseConn, err := newServerSSEConnection(h.server.context(), request.Context(), c.ConnectionID(), writer); err != nil {
				writer.WriteHeader(500) // Internal server error
			} else {
				h.serveConnection(sseConn)
			}
		} else {
			// connectionID in use
			writer.WriteHeader(409) // Conflict
		}
	} else {
		writer.WriteHeader(404) // Not found
	}
}

func (h *HttpMux) handleWebsocket(writer http.ResponseWriter, request *http.Request) {
	websocketConn, err := websocket.Accept(writer, request, nil)
	if err != nil {
		writer.WriteHeader(400) // Bad request
		return
	}
	connectionMapKey := request.URL.Query().Get("id")
	if connectionMapKey == "" {
		// Support websocket connection without negotiate
		connectionMapKey = newConnectionID()
		h.mx.Lock()
		h.connectionMap[connectionMapKey] = &negotiateConnection{
			ConnectionBase{connectionID: connectionMapKey},
		}
		h.mx.Unlock()
	}
	h.mx.Lock()
	c, ok := h.connectionMap[connectionMapKey]
	h.mx.Unlock()
	if ok {
		if _, ok := c.(*negotiateConnection); ok {
			// Connection is negotiated but not initiated
			h.serveConnection(newWebSocketConnection(h.server.context(), request.Context(), c.ConnectionID(), websocketConn))
		} else {
			// Already initiated
			_ = websocketConn.Close(409, "Bad request")
		}
	} else {
		// Not negotiated
		_ = websocketConn.Close(404, "Not found")
	}
}

// Negotiate is an http.HandleFunc which knows how to initiate a signalr connection
func (h *HttpMux) Negotiate(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		w.WriteHeader(400)
	} else {
		connectionID := newConnectionID()
		connectionMapKey := connectionID
		negotiateVersion, err := strconv.Atoi(req.Header.Get("negotiateVersion"))
		if err != nil {
			negotiateVersion = 0
		}
		connectionToken := ""
		if negotiateVersion == 1 {
			connectionToken = newConnectionID()
			connectionMapKey = connectionToken
		}
		h.mx.Lock()
		h.connectionMap[connectionMapKey] = &negotiateConnection{
			ConnectionBase{connectionID: connectionID},
		}
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
						TransferFormats: []string{"Text", "Binary"},
					})
			}
		}
		response := negotiateResponse{
			ConnectionToken:     connectionToken,
			ConnectionID:        connectionID,
			NegotiateVersion:    negotiateVersion,
			AvailableTransports: availableTransports,
		}
		_ = json.NewEncoder(w).Encode(response) // Can't imagine an error when encoding
	}
}

func (h *HttpMux) serveConnection(c Connection) {
	h.mx.Lock()
	h.connectionMap[c.ConnectionID()] = c
	h.mx.Unlock()
	h.server.Serve(c)
}

func newConnectionID() string {
	bytes := make([]byte, 16)
	// rand.Read only fails when the systems random number generator fails. Rare case, ignore
	_, _ = rand.Read(bytes)
	// Important: Use URLEncoding. StdEncoding contains "/" which will be randomly part of the connectionID and cause parsing problems
	return base64.URLEncoding.EncodeToString(bytes)
}

type negotiateConnection struct {
	ConnectionBase
}

func (n *negotiateConnection) Read([]byte) (int, error) {
	return 0, nil
}

func (n *negotiateConnection) Write([]byte) (int, error) {
	return 0, nil
}
