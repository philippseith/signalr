package signalr

import (
	"errors"
	"reflect"
)

// UseHub sets the hub instance used by the server
func UseHub(hub HubInterface) func(party) error {
	return func(p party) error {
		if s, ok := p.(*server); ok {
			s.newHub = func() HubInterface { return hub }
			return nil
		}
		return errors.New("option UseHub is server only")
	}
}

// HubFactory sets the function which returns the hub instance for every hub method invocation
// The function might create a new hub instance on every invocation.
// If hub instances should be created and initialized by a DI framework,
// the frameworks factory method can be called here.
func HubFactory(factoryFunc func() HubInterface) func(party) error {
	return func(p party) error {
		if s, ok := p.(*server); ok {
			s.newHub = factoryFunc
			return nil
		}
		return errors.New("option HubFactory is server only")
	}
}

// SimpleHubFactory sets a HubFactory which creates a new hub with the underlying type
// of hubProto on each hub method invocation.
func SimpleHubFactory(hubProto HubInterface) func(party) error {
	return HubFactory(
		func() HubInterface {
			return reflect.New(reflect.ValueOf(hubProto).Elem().Type()).Interface().(HubInterface)
		})
}
