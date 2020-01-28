package signalr

import (
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"time"
)

// TimeoutInterval is the interval one party will consider the other party disconnected
// if it hasn't received a message (including keep-alive) in it.
// The recommended value is double the KeepAliveInterval value.
// Default is 30 seconds.
func TimeoutInterval(timeout time.Duration) func(party) error {
	return func(p party) error {
		p.setTimeout(timeout)
		return nil
	}
}

// HandshakeTimeout is the interval if the other party doesn't send an initial handshake message within,
// the connection is closed. This is an advanced setting that should only be modified
// if handshake timeout errors are occurring due to severe network latency.
// For more detail on the handshake process,
// see https://github.com/dotnet/aspnetcore/blob/master/src/SignalR/docs/specs/HubProtocol.md
func HandshakeTimeout(timeout time.Duration) func(party) error {
	return func(p party) error {
		p.setHandshakeTimeout(timeout)
		return nil
	}
}

// KeepAliveInterval is the interval if the party hasn't sent a message within,
// a ping message is sent automatically to keep the connection open.
// When changing KeepAliveInterval, change the TimeoutInterval setting on the other party.
// The recommended TimeoutInterval value is double the KeepAliveInterval value.
// Default is 15 seconds.
func KeepAliveInterval(interval time.Duration) func(party) error {
	return func(p party) error {
		p.setKeepAliveInterval(interval)
		return nil
	}
}

// EnableDetailedErrors - if true, detailed exception messages are returned to the other party when an exception is thrown in a Hub method.
// The default is false, as these exception messages can contain sensitive information.
func EnableDetailedErrors(enable bool) func(party) error {
	return func(p party) error {
		p.setEnableDetailedErrors(enable)
		return nil
	}
}

// StructuredLogger is the simplest logging interface for structured logging.
// See github.com/go-kit/kit/log
type StructuredLogger interface {
	Log(keyVals ...interface{}) error
}

// Logger stets the logger used by the party to log info events.
// If debug is true, debug log event are generated, too
func Logger(logger StructuredLogger, debug bool) func(party) error {
	return func(p party) error {
		i, d := buildInfoDebugLogger(logger, debug)
		p.setLoggers(i, d)
		return nil
	}
}

func buildInfoDebugLogger(logger log.Logger, debug bool) (log.Logger, log.Logger) {
	if debug {
		logger = level.NewFilter(logger, level.AllowDebug())
	} else {
		logger = level.NewFilter(logger, level.AllowInfo())
	}
	return level.Info(logger), log.With(level.Debug(logger), "caller", log.DefaultCaller)
}
