package signalr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"runtime/debug"

	"github.com/go-kit/log"
)

// Server is a SignalR server for one type of hub.
//
// 	MapHTTP(mux *http.ServeMux, path string)
// maps the servers hub to an path on an http.ServeMux.
//
// 	Serve(conn Connection)
// serves the hub of the server on one connection.
// The same server might serve different connections in parallel. Serve does not return until the connection is closed
// or the servers context is canceled.
//
// HubClients()
// allows to call all HubClients of the server from server-side, non-hub code.
// Note that HubClients.Caller() returns nil, because there is no real caller which can be reached over a HubConnection.
type Server interface {
	Party
	MapHTTP(mux MappableRouter, path string)
	Serve(conn Connection) error
	HubClients() HubClients
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

// MappableRouter encapsulates the methods used by server.MapHTTP to configure the
// handlers required by the signalr protocol. this abstraction removes the explicit
// binding to http.ServerMux and allows use of any mux which implements those basic
// Handle and HandleFunc methods.
type MappableRouter interface {
	HandleFunc(string, func(w http.ResponseWriter, r *http.Request))
	Handle(string, http.Handler)
}

// MapHTTP maps the servers hub to an path in an http.ServeMux
func (s *server) MapHTTP(mux MappableRouter, path string) {
	httpMux := newHTTPMux(s)
	mux.HandleFunc(fmt.Sprintf("%s/negotiate", path), httpMux.negotiate)
	mux.Handle(path, httpMux)
}

// Serve serves the hub of the server on one connection.
// The same server might serve different connections in parallel. Serve does not return until the connection is closed
// or the servers' context is canceled.
func (s *server) Serve(conn Connection) error {
	if protocol, err := s.processHandshake(conn); err != nil {
		info, _ := s.prefixLoggers("")
		_ = info.Log(evt, "processHandshake", "connectionId", conn.ConnectionID(), "error", err, react, "do not connect")
		return err
	} else {
		connected := make(chan struct{}, 1)
		defer close(connected)
		return newLoop(s, conn, protocol).Run(connected)
	}
}

func (s *server) HubClients() HubClients {
	return s.defaultHubClients
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

func (s *server) processHandshake(conn Connection) (hubProtocol, error) {
	var err error
	var protocol hubProtocol
	var ok bool
	const handshakeResponse = "{}\u001e"
	const errorHandshakeResponse = "{\"error\":\"%s\"}\u001e"
	info, dbg := s.prefixLoggers(conn.ConnectionID())

	defer conn.SetTimeout(0)
	conn.SetTimeout(s._handshakeTimeout)
	var remainBuf bytes.Buffer
	rawHandshake, err := readJSONFrames(conn, &remainBuf)
	if err != nil {
		return nil, err
	}
	_ = dbg.Log(evt, "handshake received", "msg", string(rawHandshake[0]))
	request := handshakeRequest{}
	if err = json.Unmarshal(rawHandshake[0], &request); err != nil {
		// Malformed handshake
		return nil, err
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
	return protocol, err
}

var protocolMap = map[string]hubProtocol{
	"json":        &jsonHubProtocol{},
	"messagepack": &messagePackHubProtocol{},
}

// const for logging
const evt string = "event"
const msgRecv string = "message received"
const msgSend string = "message send"
const msg string = "message"
const react string = "reaction"
