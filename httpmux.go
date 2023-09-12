package signalr

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/teivah/onecontext"
	"nhooyr.io/websocket"
)

type httpMux struct {
	mx            sync.RWMutex
	connectionMap map[string]Connection
	server        Server
}

func newHTTPMux(server Server) *httpMux {
	return &httpMux{
		connectionMap: make(map[string]Connection),
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
		writer.WriteHeader(http.StatusBadRequest)
	}
}

func (h *httpMux) handlePost(writer http.ResponseWriter, request *http.Request) {
	connectionID := request.URL.Query().Get("id")
	if connectionID == "" {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	info, _ := h.server.prefixLoggers("")
	for {
		h.mx.RLock()
		c, ok := h.connectionMap[connectionID]
		h.mx.RUnlock()
		if ok {
			// Connection is initiated
			switch conn := c.(type) {
			case *serverSSEConnection:
				writer.WriteHeader(conn.consumeRequest(request))
				return
			case *negotiateConnection:
				// connection start initiated but not completed
			default:
				// ConnectionID already used for WebSocket(?)
				writer.WriteHeader(http.StatusConflict)
				return
			}
		} else {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		<-time.After(10 * time.Millisecond)
		_ = info.Log("event", "handlePost for SSE connection repeated")
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
		h.handleWebsocket(writer, request)
	} else if strings.ToLower(request.Header.Get("Accept")) == "text/event-stream" {
		h.handleServerSentEvent(writer, request)
	} else {
		writer.WriteHeader(http.StatusBadRequest)
	}
}

func (h *httpMux) handleServerSentEvent(writer http.ResponseWriter, request *http.Request) {
	connectionID := request.URL.Query().Get("id")
	if connectionID == "" {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	h.mx.RLock()
	c, ok := h.connectionMap[connectionID]
	h.mx.RUnlock()
	if ok {
		if _, ok := c.(*negotiateConnection); ok {
			ctx, _ := onecontext.Merge(h.server.context(), request.Context())
			sseConn, jobChan, jobResultChan, err := newServerSSEConnection(ctx, c.ConnectionID())
			if err != nil {
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}
			flusher, ok := writer.(http.Flusher)
			if !ok {
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}
			// Connection is negotiated but not initiated
			// We compose http and send it over sse
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.Header().Set("Connection", "keep-alive")
			writer.Header().Set("Cache-Control", "no-cache")
			writer.WriteHeader(http.StatusOK)
			// End this Server Sent Event (yes, your response now is one and the client will wait for this initial event to end)
			_, _ = fmt.Fprint(writer, ":\r\n\r\n")
			writer.(http.Flusher).Flush()
			go func() {
				// We can't WriteHeader 500 if we get an error as we already wrote the header, so ignore it.
				_ = h.serveConnection(sseConn)
			}()
			// Loop for write jobs from the sseServerConnection
			for buf := range jobChan {
				n, err := writer.Write(buf)
				if err == nil {
					flusher.Flush()
				}
				jobResultChan <- RWJobResult{n: n, err: err}
			}
			close(jobResultChan)
		} else {
			// connectionID in use
			writer.WriteHeader(http.StatusConflict)
		}
	} else {
		writer.WriteHeader(http.StatusNotFound)
	}
}

func (h *httpMux) handleWebsocket(writer http.ResponseWriter, request *http.Request) {
	accOptions := &websocket.AcceptOptions{
		CompressionMode:    websocket.CompressionContextTakeover,
		InsecureSkipVerify: h.server.insecureSkipVerify(),
		OriginPatterns:     h.server.originPatterns(),
	}
	websocketConn, err := websocket.Accept(writer, request, accOptions)
	if err != nil {
		_, debug := h.server.loggers()
		_ = debug.Log(evt, "handleWebsocket", msg, "error accepting websockets", "error", err)
		// don't need to write an error header here as websocket.Accept has already used http.Error
		return
	}
	websocketConn.SetReadLimit(int64(h.server.maximumReceiveMessageSize()))
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
	h.mx.RLock()
	c, ok := h.connectionMap[connectionMapKey]
	h.mx.RUnlock()
	if ok {
		if _, ok := c.(*negotiateConnection); ok {
			// Connection is negotiated but not initiated
			ctx, _ := onecontext.Merge(h.server.context(), request.Context())
			err = h.serveConnection(newWebSocketConnection(ctx, c.ConnectionID(), websocketConn))
			if err != nil {
				_ = websocketConn.Close(1005, err.Error())
			}
		} else {
			// Already initiated
			_ = websocketConn.Close(1002, "Bad request")
		}
	} else {
		// Not negotiated
		_ = websocketConn.Close(1002, "Not found")
	}
}

func (h *httpMux) negotiate(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		w.WriteHeader(http.StatusBadRequest)
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
			case TransportServerSentEvents:
				availableTransports = append(availableTransports,
					availableTransport{
						Transport:       string(TransportServerSentEvents),
						TransferFormats: []string{string(TransferFormatText)},
					})
			case TransportWebSockets:
				availableTransports = append(availableTransports,
					availableTransport{
						Transport:       string(TransportWebSockets),
						TransferFormats: []string{string(TransferFormatText), string(TransferFormatBinary)},
					})
			}
		}
		response := negotiateResponse{
			ConnectionToken:     connectionToken,
			ConnectionID:        connectionID,
			NegotiateVersion:    negotiateVersion,
			AvailableTransports: availableTransports,
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response) // Can't imagine an error when encoding
	}
}

func (h *httpMux) serveConnection(c Connection) error {
	h.mx.Lock()
	h.connectionMap[c.ConnectionID()] = c
	h.mx.Unlock()
	return h.server.Serve(c)
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
