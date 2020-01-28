package signalr

import (
	"errors"
	"reflect"
	"time"
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

// StreamBufferCapacity is the maximum number of items that can be buffered for client upload streams.
// If this limit is reached, the processing of invocations is blocked until the the server processes stream items.
// Default is 10.
func StreamBufferCapacity(capacity uint) func(party) error {
	return func(p party) error {
		if s, ok := p.(*server); ok {
			if capacity == 0 {
				return errors.New("unsupported StreamBufferCapacity 0")
			}
			s.streamBufferCapacity = capacity
			return nil
		}
		return errors.New("option StreamBufferCapacity is server only")
	}
}

// MaximumReceiveMessageSize is the maximum size of a single incoming hub message.
// Default is 32KB
func MaximumReceiveMessageSize(size uint) func(party) error {
	return func(p party) error {
		if s, ok := p.(*server); ok {
			if size == 0 {
				return errors.New("unsupported MaximumReceiveMessageSize 0")
			}
			s.maximumReceiveMessageSize = size
			return nil
		}
		return errors.New("option MaximumReceiveMessageSize is server only")
	}
}

// HubChanReceiveTimeout is the timeout for processing stream items from the client, after StreamBufferCapacity was reached
// If the hub method is not able to process a stream item during the timeout duration,
// the server will send a completion with error.
// Default is 5 seconds.
func HubChanReceiveTimeout(timeout time.Duration) func(party) error {
	return func(p party) error {
		if s, ok := p.(*server); ok {
			s.hubChanReceiveTimeout = timeout
			return nil
		}
		return errors.New("option HubChanReceiveTimeout is server only")
	}
}
