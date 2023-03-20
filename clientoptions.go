package signalr

import (
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
func TransferFormat(format string) func(Party) error {
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
