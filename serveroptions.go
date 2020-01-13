package signalr

import (
	"reflect"
	"time"
)

// UseHub sets the hub instance used by the server
func UseHub(hub HubInterface) func(*Server) error {
	return func(s *Server) error {
		s.newHub = func() HubInterface { return hub }
		return nil
	}
}

// HubFactory sets the function which returns the hub instance for every hub method invocation
// The function might create a new hub instance on every invocation.
// If hub instances should be created and initialized by a DI framework,
// the frameworks factory method can be called here.
func HubFactory(factoryFunc func() HubInterface) func(*Server) error {
	return func(s *Server) error {
		s.newHub = factoryFunc
		return nil
	}
}

// SimpleHubFactory sets a HubFactory which creates a new hub with the underlying type
// of hubProto on each hub method invocation.
func SimpleHubFactory(hubProto HubInterface) func(*Server) error {
	return HubFactory(
		func() HubInterface {
			return reflect.New(reflect.ValueOf(hubProto).Elem().Type()).Interface().(HubInterface)
		})
}

// HubChanReceiveTimeout is the timeout for receiving stream items from the client.
// If the hub method is not able to receive a stream item during the timeout duration,
// the server will send a completion with error
func HubChanReceiveTimeout(duration time.Duration) func(*Server) error {
	return func(s *Server) error {
		s.hubChanReceiveTimeout = duration
		return nil
	}
}

// StructuredLogger is the simplest logging interface for structured logging.
// See github.com/go-kit/kit/log
type StructuredLogger interface {
	Log(keyvals ...interface{}) error
}

// Logger stets the logger used by the server to log info events.
// If debug is true, debug log event are generated, too
func Logger(logger StructuredLogger, debug bool) func(*Server) error {
	return func(s *Server) error {
		i, d := buildInfoDebugLogger(logger, debug)
		s.info = i
		s.dbg = d
		return nil
	}
}
