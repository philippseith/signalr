package signalr

import (
	"errors"
	"fmt"
)

// The Client option Receiver sets the object which will receive server side calls to client methods (e.g. callbacks)
func Receiver(receiver interface{}) func(Party) error {
	return func(party Party) error {
		if client, ok := party.(*client); ok {
			client.receiver = receiver
			return nil
		}
		return errors.New("option Receiver is client only")
	}
}

// The Client option TransferFormat sets the transfer format used on the transport. Allowed values are "Text" and "Binary"
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
