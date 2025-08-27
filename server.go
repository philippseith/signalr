package signalr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os" // Add os import
	"reflect"
	"runtime/debug"
	"sync"

	"github.com/go-kit/log"
)

// Server is a SignalR server for one type of hub.
//
//	MapHTTP(mux *http.ServeMux, path string)
//
// maps the servers' hub to a path on a http.ServeMux.
//
//	Serve(conn Connection)
//
// serves the hub of the server on one connection.
// The same server might serve different connections in parallel. Serve does not return until the connection is closed
// or the servers' context is canceled.
//
// HubClients()
// allows to call all HubClients of the server from server-side, non-hub code.
// Note that HubClients.Caller() returns nil, because there is no real caller which can be reached over a HubConnection.
type Server interface {
	Party
	MapHTTP(routerFactory func() MappableRouter, path string)
	Serve(conn Connection) error
	HubClients() HubClients
	availableTransports() []TransportType
}

type server struct {
	partyBase
	newHub                  func() HubInterface
	perConnectionHubFactory func(connectionID string) HubInterface
	connectionHubs          map[string]HubInterface
	connectionHubsMutex     sync.RWMutex
	lifetimeManager         HubLifetimeManager
	defaultHubClients       *defaultHubClients
	groupManager            GroupManager
	reconnectAllowed        bool
	transports              []TransportType
}

// NewServer creates a new server for one type of hub. The hub type is set by one of the
// options UseHub, HubFactory or SimpleHubFactory
func NewServer(ctx context.Context, options ...func(Party) error) (Server, error) {
	info, dbg := buildInfoDebugLogger(log.NewLogfmtLogger(os.Stderr), false)
	lifetimeManager := newLifeTimeManager(info)
	server := &server{
		lifetimeManager: &lifetimeManager,
		connectionHubs:  make(map[string]HubInterface),
		// Do NOT initialize defaultHubClients or groupManager yet
		partyBase:        newPartyBase(ctx, info, dbg),
		reconnectAllowed: true,
	}
	// apply any option including lifetime manager, logger, etc.
	for _, option := range options {
		if option != nil {
			if err := option(server); err != nil {
				return nil, err
			}
		}
	}

	// Now initialize them, using the (possibly replaced) lifetimeManager
	server.defaultHubClients = &defaultHubClients{
		lifetimeManager: server.lifetimeManager,
		allCache:        allClientProxy{lifetimeManager: server.lifetimeManager},
	}

	server.groupManager = &defaultGroupManager{
		lifetimeManager: server.lifetimeManager,
	}

	if server.transports == nil {
		server.transports = []TransportType{TransportWebSockets, TransportServerSentEvents}
	}

	if server.newHub == nil && server.perConnectionHubFactory == nil {
		return server, errors.New("cannot determine hub type. Neither UseHub, HubFactory, SimpleHubFactory, or PerConnectionHubFactory given as option")
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

// WithHTTPServeMux is a MappableRouter factory for MapHTTP which converts a
// http.ServeMux to a MappableRouter.
// For factories for other routers, see github.com/mojtabaRKS/signalr/router
func WithHTTPServeMux(serveMux *http.ServeMux) func() MappableRouter {
	return func() MappableRouter {
		return serveMux
	}
}

// MapHTTP maps the servers' hub to a path in a MappableRouter
func (s *server) MapHTTP(routerFactory func() MappableRouter, path string) {
	httpMux := newHTTPMux(s)
	router := routerFactory()
	router.HandleFunc(fmt.Sprintf("%s/negotiate", path), httpMux.negotiate)
	router.Handle(path, httpMux)
}

// Serve serves the hub of the server on one connection.
// The same server might serve different connections in parallel. Serve does not return until the connection is closed
// or the servers' context is canceled.
func (s *server) Serve(conn Connection) error {

	protocol, err := s.processHandshake(conn)
	if err != nil {
		info, _ := s.prefixLoggers("")
		_ = info.Log(evt, "processHandshake", "connectionId", conn.ConnectionID(), "error", err, react, "do not connect")
		return err
	}

	return newLoop(s, conn, protocol).Run(make(chan struct{}, 1))
}

func (s *server) HubClients() HubClients {
	return s.defaultHubClients
}

func (s *server) availableTransports() []TransportType {
	return s.transports
}

func (s *server) onConnected(hc HubConnection) {
	s.lifetimeManager.OnConnected(hc)
	go func() {
		defer s.recoverHubLifeCyclePanic()
		s.invocationTarget(hc).(HubInterface).OnConnected(hc.ConnectionID())
	}()
}

func (s *server) onDisconnected(hc HubConnection) {
	go func() {
		defer s.recoverHubLifeCyclePanic()
		s.invocationTarget(hc).(HubInterface).OnDisconnected(hc.ConnectionID())
	}()
	s.lifetimeManager.OnDisconnected(hc)

	// Clean up the connection hub
	s.removeConnectionHub(hc.ConnectionID())
}

func (s *server) invocationTarget(conn HubConnection) interface{} {
	if s.perConnectionHubFactory != nil {
		// Use per-connection hub factory
		connectionID := conn.ConnectionID()

		// Check if we already have a hub instance for this connection
		if hub, exists := s.getConnectionHub(connectionID); exists {
			return hub
		}

		// Create new hub instance for this connection
		hub := s.perConnectionHubFactory(connectionID)
		hub.Initialize(s.newConnectionHubContext(conn))
		s.setConnectionHub(connectionID, hub)
		return hub
	}

	// Fall back to the original behavior
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
	var hubType reflect.Type
	if s.perConnectionHubFactory != nil {
		// Create a temporary hub to get the type for logging
		tempHub := s.perConnectionHubFactory(connectionID)
		hubValue := reflect.ValueOf(tempHub)
		if hubValue.Kind() == reflect.Ptr {
			hubType = hubValue.Elem().Type()
		} else {
			hubType = hubValue.Type()
		}
	} else if s.newHub != nil {
		hubValue := reflect.ValueOf(s.newHub())
		if hubValue.Kind() == reflect.Ptr {
			hubType = hubValue.Elem().Type()
		} else {
			hubType = hubValue.Type()
		}
	} else {
		hubType = reflect.TypeOf(nil)
	}

	return log.WithPrefix(s.info, "ts", log.DefaultTimestampUTC,
			"class", "Server",
			"connection", connectionID,
			"hub", hubType),
		log.WithPrefix(s.dbg, "ts", log.DefaultTimestampUTC,
			"class", "Server",
			"connection", connectionID,
			"hub", hubType)
}

func (s *server) newConnectionHubContext(hubConn HubConnection) HubContext {
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
	if request, err := s.receiveHandshakeRequest(conn); err != nil {
		return nil, err
	} else {
		return s.sendHandshakeResponse(conn, request)
	}
}

func (s *server) receiveHandshakeRequest(conn Connection) (handshakeRequest, error) {
	_, dbg := s.prefixLoggers(conn.ConnectionID())
	ctx, cancelRead := context.WithTimeout(s.Context(), s.HandshakeTimeout())
	defer cancelRead()
	readJSONFramesChan := make(chan []interface{}, 1)
	go func() {
		var remainBuf bytes.Buffer
		rawHandshake, err := readJSONFrames(conn, &remainBuf)
		readJSONFramesChan <- []interface{}{rawHandshake, err}
	}()
	request := handshakeRequest{}
	select {
	case result := <-readJSONFramesChan:
		if result[1] != nil {
			return request, result[1].(error)
		}
		rawHandshake := result[0].([][]byte)
		_ = dbg.Log(evt, "handshake received", "msg", string(rawHandshake[0]))
		return request, json.Unmarshal(rawHandshake[0], &request)
	case <-ctx.Done():
		return request, ctx.Err()
	}
}

func (s *server) sendHandshakeResponse(conn Connection, request handshakeRequest) (protocol hubProtocol, err error) {
	info, dbg := s.prefixLoggers(conn.ConnectionID())
	ctx, cancelWrite := context.WithTimeout(s.Context(), s.HandshakeTimeout())
	defer cancelWrite()
	var ok bool
	if protocol, ok = protocolMap[request.Protocol]; ok {
		// Send the handshake response
		const handshakeResponse = "{}\u001e"
		if _, err = ReadWriteWithContext(ctx,
			func() (int, error) {
				return conn.Write([]byte(handshakeResponse))
			}, func() {}); err != nil {
			_ = dbg.Log(evt, "handshake sent", "error", err)
		} else {
			_ = dbg.Log(evt, "handshake sent", "msg", handshakeResponse)
		}
	} else {
		err = fmt.Errorf("protocol %v not supported", request.Protocol)
		_ = info.Log(evt, "protocol requested", "error", err)
		if _, respErr := ReadWriteWithContext(ctx,
			func() (int, error) {
				const errorHandshakeResponse = "{\"error\":\"%s\"}\u001e"
				return conn.Write([]byte(fmt.Sprintf(errorHandshakeResponse, err)))
			}, func() {}); respErr != nil {
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

// getConnectionHub retrieves a hub instance for a specific connection
func (s *server) getConnectionHub(connectionID string) (HubInterface, bool) {
	s.connectionHubsMutex.RLock()
	defer s.connectionHubsMutex.RUnlock()
	hub, exists := s.connectionHubs[connectionID]
	return hub, exists
}

// setConnectionHub stores a hub instance for a specific connection
func (s *server) setConnectionHub(connectionID string, hub HubInterface) {
	s.connectionHubsMutex.Lock()
	defer s.connectionHubsMutex.Unlock()
	s.connectionHubs[connectionID] = hub
}

// removeConnectionHub removes a hub instance for a specific connection
func (s *server) removeConnectionHub(connectionID string) {
	s.connectionHubsMutex.Lock()
	defer s.connectionHubsMutex.Unlock()
	delete(s.connectionHubs, connectionID)
}
