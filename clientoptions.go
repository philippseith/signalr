package signalr

import (
	"context"
	"errors"
	"fmt"

	"github.com/cenkalti/backoff/v4"
)

// WithConnection sets the Connection of the Client
func WithConnection(connection Connection) func(party Party) error {
	return func(party Party) error {
		if client, ok := party.(*client); ok {
			if client.connectionFactory != nil {
				return errors.New("options WithConnection and WithConnector can not be used together")
			}
			client.conn = connection
			return nil
		}
		return errors.New("option WithConnection is client only")
	}
}

// WithConnector allows the Client to establish a connection
// using the Connection build by the connectionFactory.
// It is also used for auto reconnect if the connection is lost.
func WithConnector(connectionFactory func() (Connection, error)) func(Party) error {
	return func(party Party) error {
		if client, ok := party.(*client); ok {
			if client.conn != nil {
				return errors.New("options WithConnection and WithConnector can not be used together")
			}
			client.connectionFactory = connectionFactory
			return nil
		}
		return errors.New("option WithConnector is client only")
	}
}

// HttpConnectionFactory is a connectionFactory for WithConnector which first tries to create a connection
// with WebSockets (if it is allowed by the HttpConnection options) and if this fails, falls back to a SSE based connection.
func HttpConnectionFactory(ctx context.Context, address string, options ...func(*httpConnection) error) (Connection, error) {
	conn := &httpConnection{}
	for i, option := range options {
		if err := option(conn); err != nil {
			return nil, err
		}
		if conn.transports != nil {
			// Remove the WithTransports option
			options = append(options[:i], options[i+1:]...)
			break
		}
	}
	// If no WithTransports was given, NewHTTPConnection fallbacks to both
	if conn.transports == nil {
		conn.transports = []TransportType{TransportWebSockets, TransportServerSentEvents}
	}

	for _, transport := range conn.transports {
		// If Websockets are allowed, we try to connect with these
		if transport == TransportWebSockets {
			wsOptions := append(options, WithTransports(TransportWebSockets))
			conn, err := NewHTTPConnection(ctx, address, wsOptions...)
			// If this is ok, return the conn
			if err == nil {
				return conn, err
			}
			break
		}
	}
	for _, transport := range conn.transports {
		// If SSE is allowed, with fallback to try these
		if transport == TransportServerSentEvents {
			sseOptions := append(options, WithTransports(TransportServerSentEvents))
			return NewHTTPConnection(ctx, address, sseOptions...)
		}
	}
	// None of the transports worked
	return nil, fmt.Errorf("can not connect with supported transports: %v", conn.transports)
}

// WithHttpConnection first tries to create a connection
// with WebSockets (if it is allowed by the HttpConnection options) and if this fails, falls back to a SSE based connection.
// This strategy is also used for auto reconnect if this option is used.
// WithHttpConnection is a shortcut for WithConnector(HttpConnectionFactory(...))
func WithHttpConnection(ctx context.Context, address string, options ...func(*httpConnection) error) func(Party) error {
	return WithConnector(func() (Connection, error) {
		return HttpConnectionFactory(ctx, address, options...)
	})
}

// WithReceiver sets the object which will receive server side calls to client methods (e.g. callbacks)
func WithReceiver(receiver interface{}) func(Party) error {
	return func(party Party) error {
		if client, ok := party.(*client); ok {
			client.receiver = receiver
			if receiver, ok := receiver.(ReceiverInterface); ok {
				receiver.Init(client)
			}
			return nil
		}
		return errors.New("option WithReceiver is client only")
	}
}

// WithBackoff sets the backoff.BackOff used for repeated connection attempts in the client.
// See https://pkg.go.dev/github.com/cenkalti/backoff for configuration options.
// If the option is not set, backoff.NewExponentialBackOff() without any further configuration will be used.
func WithBackoff(backoffFactory func() backoff.BackOff) func(party Party) error {
	return func(party Party) error {
		if client, ok := party.(*client); ok {
			client.backoffFactory = backoffFactory
			return nil
		}
		return errors.New("option WithBackoff is client only")
	}
}

// TransferFormat sets the transfer format used on the transport. Allowed values are "Text" and "Binary"
func TransferFormat(format TransferFormatType) func(Party) error {
	return func(p Party) error {
		if c, ok := p.(*client); ok {
			switch format {
			case "Text":
				c.format = "json"
			case "Binary":
				c.format = "messagepack"
			default:
				return fmt.Errorf("invalid transferformat %v", format)
			}
			return nil
		}
		return errors.New("option TransferFormat is client only")
	}
}
