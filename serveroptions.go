package signalr

import (
	"errors"
	"fmt"
	"reflect"
)

// The Server option UseHub sets the hub instance used by the server
func UseHub(hub HubInterface) func(Party) error {
	return func(p Party) error {
		if s, ok := p.(*server); ok {
			s.newHub = func() HubInterface { return hub }
			return nil
		}
		return errors.New("option UseHub is server only")
	}
}

// The Server option HubFactory sets the function which returns the hub instance for every hub method invocation
// The function might create a new hub instance on every invocation.
// If hub instances should be created and initialized by a DI framework,
// the frameworks factory method can be called here.
func HubFactory(factoryFunc func() HubInterface) func(Party) error {
	return func(p Party) error {
		if s, ok := p.(*server); ok {
			s.newHub = factoryFunc
			return nil
		}
		return errors.New("option HubFactory is server only")
	}
}

// The Server option SimpleHubFactory sets a HubFactory which creates a new hub with the underlying type
// of hubProto on each hub method invocation.
func SimpleHubFactory(hubProto HubInterface) func(Party) error {
	return HubFactory(
		func() HubInterface {
			return reflect.New(reflect.ValueOf(hubProto).Elem().Type()).Interface().(HubInterface)
		})
}

// The Server option HTTPTransports sets the list of available transports for http connections. Allowed transports are
// "WebSockets", "ServerSentEvents". Default is both transports are available.
func HTTPTransports(transports ...string) func(Party) error {
	return func(p Party) error {
		if s, ok := p.(*server); ok {
			for _, transport := range transports {
				switch transport {
				case "WebSockets", "ServerSentEvents":
					s.transports = append(s.transports, transport)
				default:
					return fmt.Errorf("unsupported transport: %v", transport)
				}
			}
			return nil
		}
		return errors.New("option Transports is server only")
	}
}
