// Package signalr implements a SignalR server in go.
// SignalR is an open-source library that simplifies adding real-time web functionality to apps.
// Real-time web functionality enables server-side code to push content to clients instantly.
// Historically it was tied to ASP.NET Core but the protocol is open and implementable in any language.
// The server currently supports transport over http/WebSockets and TCP. The supported protocol encoding in JSON.
package signalr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/mailru/easyjson/jwriter"
	"net/http"
	"os"
	"reflect"
	"runtime/debug"
)

// Server is a SignalR server for one type of hub.
//
// 	MapHTTP(mux *http.ServeMux, path string)
// maps the servers hub to an path on an http.ServeMux.
//
// 	MapConnection(conn Connection)]
// maps its hub on one connection. The same server might serve different connections in parallel.
type Server interface {
	Party
	MapHTTP(mux *http.ServeMux, path string)
	MapConnection(conn Connection)
	availableTransports() []string
}

type server struct {
	partyBase
	newHub            func() HubInterface
	lifetimeManager   HubLifetimeManager
	defaultHubClients *defaultHubClients
	groupManager      GroupManager
	reconnectAllowed  bool
	transports        []string
}

// NewServer creates a new server for one type of hub. The hub type is set by one of the
// options UseHub, HubFactory or SimpleHubFactory
func NewServer(ctx context.Context, options ...func(Party) error) (Server, error) {
	info, dbg := buildInfoDebugLogger(log.NewLogfmtLogger(os.Stderr), false)
	lifetimeManager := newLifeTimeManager(info)
	server := &server{
		lifetimeManager: &lifetimeManager,
		defaultHubClients: &defaultHubClients{
			lifetimeManager: &lifetimeManager,
			allCache:        allClientProxy{lifetimeManager: &lifetimeManager},
		},
		groupManager: &defaultGroupManager{
			lifetimeManager: &lifetimeManager,
		},
		partyBase:        newPartyBase(ctx, info, dbg),
		reconnectAllowed: true,
	}
	for _, option := range options {
		if option != nil {
			if err := option(server); err != nil {
				return nil, err
			}
		}
	}
	if server.transports == nil {
		server.transports = []string{"WebSockets", "ServerSentEvents"}
	}
	if server.newHub == nil {
		return server, errors.New("cannot determine hub type. Neither UseHub, HubFactory or SimpleHubFactory given as option")
	}
	return server, nil
}

// MapHTTP maps the servers hub to an path on an http.ServeMux
func (s *server) MapHTTP(mux *http.ServeMux, path string) {
	httpMux := newHTTPMux(s)
	mux.HandleFunc(fmt.Sprintf("%s/negotiate", path), httpMux.negotiate)
	mux.Handle(path, httpMux)
}

// MapConnection maps its hub on one connection. The same server might serve different connections in parallel
func (s *server) MapConnection(conn Connection) {
	if protocol, err := s.processHandshake(conn); err != nil {
		info, _ := s.prefixLoggers("")
		_ = info.Log(evt, "processHandshake", "connectionId", conn.ConnectionID(), "error", err, react, "do not connect")
	} else {
		newLoop(s, conn, protocol).Run(make(chan struct{}, 1))
	}
}

func (s *server) availableTransports() []string {
	return s.transports
}

func (s *server) onConnected(hc hubConnection) {
	s.lifetimeManager.OnConnected(hc)
	go func() {
		defer s.recoverHubLifeCyclePanic()
		s.invocationTarget(hc).(HubInterface).OnConnected(hc.ConnectionID())
	}()
}

func (s *server) onDisconnected(hc hubConnection) {
	go func() {
		defer s.recoverHubLifeCyclePanic()
		s.invocationTarget(hc).(HubInterface).OnDisconnected(hc.ConnectionID())
	}()
	s.lifetimeManager.OnDisconnected(hc)

}

func (s *server) invocationTarget(conn hubConnection) interface{} {
	hub := s.newHub()
	hub.Initialize(s.newConnectionHubContext(conn))
	return hub
}

func (s *server) allowReconnect() bool {
	return s.reconnectAllowed
}

func (s *server) recoverHubLifeCyclePanic() {
	if err := recover(); err != nil {
		s.reconnectAllowed = false
		info, dbg := s.prefixLoggers("")
		_ = info.Log(evt, "panic in hub lifecycle", "error", err, react, "close connection, allow no reconnect")
		_ = dbg.Log(evt, "panic in hub lifecycle", "error", err, react, "close connection, allow no reconnect", "stack", string(debug.Stack()))
		s.cancel()
	}
}

func (s *server) prefixLoggers(connectionID string) (info StructuredLogger, dbg StructuredLogger) {
	return log.WithPrefix(s.info, "ts", log.DefaultTimestampUTC,
			"class", "Server",
			"connection", connectionID,
			"hub", reflect.ValueOf(s.newHub()).Elem().Type()),
		log.WithPrefix(s.dbg, "ts", log.DefaultTimestampUTC,
			"class", "Server",
			"connection", connectionID,
			"hub", reflect.ValueOf(s.newHub()).Elem().Type())
}

func (s *server) newConnectionHubContext(hubConn hubConnection) HubContext {
	return &connectionHubContext{
		abort: hubConn.Abort,
		clients: &callerHubClients{
			defaultHubClients: s.defaultHubClients,
			connectionID:      hubConn.ConnectionID(),
		},
		groups:     s.groupManager,
		connection: hubConn,
		info:       s.info,
		dbg:        s.dbg,
	}
}

func (s *server) processHandshake(conn Connection) (HubProtocol, error) {
	var err error
	var protocol HubProtocol
	var ok bool
	const handshakeResponse = "{}\u001e"
	const errorHandshakeResponse = "{\"error\":\"%s\"}\u001e"
	info, dbg := s.prefixLoggers(conn.ConnectionID())

	defer conn.SetTimeout(0)
	conn.SetTimeout(s._handshakeTimeout)

	var buf bytes.Buffer
	data := make([]byte, 1<<12)
	for {
		var n int
		if n, err = conn.Read(data); err != nil {
			break
		} else {
			buf.Write(data[:n])
			var rawHandshake []byte
			if rawHandshake, err = parseTextMessageFormat(&buf); err != nil {
				// Partial message, read more data
				buf.Write(data[:n])
			} else {
				_ = dbg.Log(evt, "handshake received", "msg", string(rawHandshake))
				request := handshakeRequest{}
				if err = json.Unmarshal(rawHandshake, &request); err != nil {
					// Malformed handshake
					break
				}
				if protocol, ok = protocolMap[request.Protocol]; ok {
					// Send the handshake response
					if _, err = conn.Write([]byte(handshakeResponse)); err != nil {
						_ = dbg.Log(evt, "handshake sent", "error", err)
					} else {
						_ = dbg.Log(evt, "handshake sent", "msg", handshakeResponse)
					}
				} else {
					err = fmt.Errorf("protocol %v not supported", request.Protocol)
					_ = info.Log(evt, "protocol requested", "error", err)
					if _, respErr := conn.Write([]byte(fmt.Sprintf(errorHandshakeResponse, err))); respErr != nil {
						_ = dbg.Log(evt, "handshake sent", "error", respErr)
						err = respErr
					}
				}
				break
			}
		}
	}
	return protocol, err
}

var protocolMap = map[string]HubProtocol{
	"json":        &JSONHubProtocol{easyWriter: jwriter.Writer{}},
	"messagepack": &MessagePackHubProtocol{},
}

// const for logging
const evt string = "event"
const msgRecv string = "message received"
const msgSend string = "message send"
const msg string = "message"
const react string = "reaction"
