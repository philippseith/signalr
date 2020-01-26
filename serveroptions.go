package signalr

import (
	"errors"
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

// ClientTimeoutInterval is the interval the server will consider the client disconnected
// if it hasn't received a message (including keep-alive) in it.
// The recommended value is double the KeepAliveInterval value.
// Default is 30 seconds.
func ClientTimeoutInterval(timeout time.Duration) func(*Server) error {
	return func(s *Server) error {
		s.clientTimeoutInterval = timeout
		return nil
	}
}

// HandshakeTimeout is the interval if the client doesn't send an initial handshake message within,
// the connection is closed. This is an advanced setting that should only be modified
// if handshake timeout errors are occurring due to severe network latency.
// For more detail on the handshake process,
// see https://github.com/dotnet/aspnetcore/blob/master/src/SignalR/docs/specs/HubProtocol.md
func HandshakeTimeout(timeout time.Duration) func(*Server) error {
	return func(s *Server) error {
		s.handshakeTimeout = timeout
		return nil
	}
}

// KeepAliveInterval is the interval if the server hasn't sent a message within,
// a ping message is sent automatically to keep the connection open.
// When changing KeepAliveInterval, change the ServerTimeout/serverTimeoutInMilliseconds setting on the client.
// The recommended ServerTimeout/serverTimeoutInMilliseconds value is double the KeepAliveInterval value.
// Default is 15 seconds.
func KeepAliveInterval(timeout time.Duration) func(*Server) error {
	return func(s *Server) error {
		s.keepAliveInterval = timeout
		return nil
	}
}

// EnableDetailedErrors - if true, detailed exception messages are returned to clients when an exception is thrown in a Hub method.
// The default is false, as these exception messages can contain sensitive information.
func EnableDetailedErrors(enable bool) func(*Server) error {
	return func(s *Server) error {
		s.enableDetailedErrors = enable
		return nil
	}
}

// StreamBufferCapacity is the maximum number of items that can be buffered for client upload streams.
// If this limit is reached, the processing of invocations is blocked until the the server processes stream items.
// Default is 10.
func StreamBufferCapacity(capacity uint) func(*Server) error {
	return func(s *Server) error {
		if capacity == 0 {
			return errors.New("unsupported StreamBufferCapacity 0")
		}
		s.streamBufferCapacity = capacity
		return nil
	}
}

// MaximumReceiveMessageSize is the maximum size of a single incoming hub message.
// Default is 32KB
func MaximumReceiveMessageSize(size uint) func(*Server) error {
	return func(s *Server) error {
		if size == 0 {
			return errors.New("unsupported MaximumReceiveMessageSize 0")
		}
		s.maximumReceiveMessageSize = size
		return nil
	}
}

// HubChanReceiveTimeout is the timeout for processing stream items from the client, after StreamBufferCapacity was reached
// If the hub method is not able to process a stream item during the timeout duration,
// the server will send a completion with error.
// Default is 5 seconds.
func HubChanReceiveTimeout(timeout time.Duration) func(*Server) error {
	return func(s *Server) error {
		s.hubChanReceiveTimeout = timeout
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
