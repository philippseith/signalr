package signalr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"os"
	"reflect"
	"time"
)

// Server is a SignalR server for one type of hub
type Server struct {
	newHub                    func() HubInterface
	lifetimeManager           HubLifetimeManager
	defaultHubClients         *defaultHubClients
	groupManager              GroupManager
	info                      log.Logger
	dbg                       log.Logger
	hubChanReceiveTimeout     time.Duration
	clientTimeoutInterval     time.Duration
	handshakeTimeout          time.Duration
	keepAliveInterval         time.Duration
	enableDetailedErrors      bool
	streamBufferCapacity      int
	maximumReceiveMessageSize int
}

// NewServer creates a new server for one type of hub
// newHub is called each time a hub method is invoked by a client to create the transient hub instance
func NewServer(options ...func(*Server) error) (*Server, error) {
	lifetimeManager := defaultHubLifetimeManager{}
	i, d := buildInfoDebugLogger(log.NewLogfmtLogger(os.Stderr), false)
	server := &Server{
		lifetimeManager: &lifetimeManager,
		defaultHubClients: &defaultHubClients{
			lifetimeManager: &lifetimeManager,
			allCache:        allClientProxy{lifetimeManager: &lifetimeManager},
		},
		groupManager: &defaultGroupManager{
			lifetimeManager: &lifetimeManager,
		},
		info:                      i,
		dbg:                       d,
		hubChanReceiveTimeout:     time.Second * 5,
		clientTimeoutInterval:     time.Second * 30,
		handshakeTimeout:          time.Second * 15,
		keepAliveInterval:         time.Second * 15,
		enableDetailedErrors:      false,
		streamBufferCapacity:      10,
		maximumReceiveMessageSize: 1 << 15, // 32KB
	}
	for _, option := range options {
		if option != nil {
			if err := option(server); err != nil {
				return nil, err
			}
		}
	}
	if server.newHub == nil {
		return server, errors.New("cannot determine hub type. Neither UseHub, HubFactory or SimpleHubFactory given as option")
	}
	return server, nil
}

// Run runs the server on one connection. The same server might be run on different connections in parallel
func (s *Server) Run(conn Connection) {
	for {
		if protocol, err := s.processHandshake(conn); err != nil {
			info, _ := s.prefixLogger()
			_ = info.Log(evt, "processHandshake", "connectionId", conn.ConnectionID(), "error", err, react, "do not connect")
		} else {
			s.newServerLoop(conn, protocol).Run() // TODO Add return value to to allow breaking out of the outer loop
		}
	}
}

func (s *Server) prefixLogger() (info log.Logger, debug log.Logger) {
	return log.WithPrefix(s.info, "ts", log.DefaultTimestampUTC,
			"class", "Server",
			"hub", reflect.ValueOf(s.newHub()).Elem().Type()),
		log.WithPrefix(s.dbg, "ts", log.DefaultTimestampUTC,
			"class", "Server",
			"hub", reflect.ValueOf(s.newHub()).Elem().Type())
}

func buildInfoDebugLogger(logger log.Logger, debug bool) (log.Logger, log.Logger) {
	if debug {
		logger = level.NewFilter(logger, level.AllowDebug())
	} else {
		logger = level.NewFilter(logger, level.AllowInfo())
	}
	return level.Info(logger), log.With(level.Debug(logger), "caller", log.DefaultCaller)
}

func (s *Server) newConnectionHubContext(conn hubConnection) HubContext {
	return &connectionHubContext{
		clients: &callerHubClients{
			defaultHubClients: s.defaultHubClients,
			connectionID:      conn.ConnectionID(),
		},
		groups: s.groupManager,
		items:  conn.Items(),
	}
}

func (s *Server) getHub(conn hubConnection) HubInterface {
	hub := s.newHub()
	hub.Initialize(s.newConnectionHubContext(conn))
	return hub
}

func (s *Server) processHandshake(conn Connection) (HubProtocol, error) {
	var err error
	var protocol HubProtocol
	var ok bool
	const handshakeResponse = "{}\u001e"
	const errorHandshakeResponse = "{\"error\":\"%s\"}\u001e"
	info, dbg := s.prefixLogger()

	defer conn.SetTimeout(0)
	conn.SetTimeout(time.Second * 5)

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
	"json": &JSONHubProtocol{},
}

// const for logging
const evt string = "event"
const msgRecv string = "message received"
const msgSend string = "message send"
const msg string = "message"
const react string = "reaction"
