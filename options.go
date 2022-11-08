package signalr

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

// TimeoutInterval is the interval one Party will consider the other Party disconnected
// if it hasn't received a message (including keep-alive) in it.
// The recommended value is double the KeepAliveInterval value.
// Default is 30 seconds.
func TimeoutInterval(timeout time.Duration) func(Party) error {
	return func(p Party) error {
		p.setTimeout(timeout)
		return nil
	}
}

// HandshakeTimeout is the interval if the other Party doesn't send an initial handshake message within,
// the connection is closed. This is an advanced setting that should only be modified
// if handshake timeout errors are occurring due to severe network latency.
// For more detail on the handshake process,
// see https://github.com/dotnet/aspnetcore/blob/master/src/SignalR/docs/specs/HubProtocol.md
func HandshakeTimeout(timeout time.Duration) func(Party) error {
	return func(p Party) error {
		p.setHandshakeTimeout(timeout)
		return nil
	}
}

// KeepAliveInterval is the interval if the Party hasn't sent a message within,
// a ping message is sent automatically to keep the connection open.
// When changing KeepAliveInterval, change the Timeout setting on the other Party.
// The recommended Timeout value is double the KeepAliveInterval value.
// Default is 15 seconds.
func KeepAliveInterval(interval time.Duration) func(Party) error {
	return func(p Party) error {
		p.setKeepAliveInterval(interval)
		return nil
	}
}

// StreamBufferCapacity is the maximum number of items that can be buffered for client upload streams.
// If this limit is reached, the processing of invocations is blocked until the server processes stream items.
// Default is 10.
func StreamBufferCapacity(capacity uint) func(Party) error {
	return func(p Party) error {
		if capacity == 0 {
			return errors.New("unsupported StreamBufferCapacity 0")
		}
		p.setStreamBufferCapacity(capacity)
		return nil
	}
}

// MaximumReceiveMessageSize is the maximum size in bytes of a single incoming hub message.
// Default is 32768 bytes (32KB)
func MaximumReceiveMessageSize(sizeInBytes uint) func(Party) error {
	return func(p Party) error {
		if sizeInBytes == 0 {
			return errors.New("unsupported maximumReceiveMessageSize 0")
		}
		p.setMaximumReceiveMessageSize(sizeInBytes)
		return nil
	}
}

// ChanReceiveTimeout is the timeout for processing stream items from the client, after StreamBufferCapacity was reached
// If the hub method is not able to process a stream item during the timeout duration,
// the server will send a completion with error.
// Default is 5 seconds.
func ChanReceiveTimeout(timeout time.Duration) func(Party) error {
	return func(p Party) error {
		p.setChanReceiveTimeout(timeout)
		return nil
	}
}

// EnableDetailedErrors If true, detailed exception messages are returned to the other
// Party when an exception is thrown in a Hub method.
// The default is false, as these exception messages can contain sensitive information.
func EnableDetailedErrors(enable bool) func(Party) error {
	return func(p Party) error {
		p.setEnableDetailedErrors(enable)
		return nil
	}
}

// StructuredLogger is the simplest logging interface for structured logging.
// See github.com/go-kit/log
type StructuredLogger interface {
	Log(keyVals ...interface{}) error
}

// Logger sets the logger used by the Party to log info events.
// If debug is true, debug log event are generated, too
func Logger(logger StructuredLogger, debug bool) func(Party) error {
	return func(p Party) error {
		i, d := buildInfoDebugLogger(logger, debug)
		p.setLoggers(i, d)
		return nil
	}
}

type recoverLogger struct {
	logger log.Logger
}

func (r *recoverLogger) Log(keyVals ...interface{}) error {
	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("recovering from panic in logger: %v\n", err)
		}
	}()
	return r.logger.Log(keyVals...)
}

func buildInfoDebugLogger(logger log.Logger, debug bool) (log.Logger, log.Logger) {
	if debug {
		logger = level.NewFilter(logger, level.AllowDebug())
	} else {
		logger = level.NewFilter(logger, level.AllowInfo())
	}
	infoLogger := &recoverLogger{level.Info(logger)}
	debugLogger := log.With(&recoverLogger{level.Debug(logger)}, "caller", log.DefaultCaller)
	return infoLogger, debugLogger
}
