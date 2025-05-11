package signalr

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/redis/go-redis/v9"
)

// UseHub sets the hub instance used by the server
func UseHub(hub HubInterface) func(Party) error {
	return func(p Party) error {
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
// the frameworks' factory method can be called here.
func HubFactory(factory func() HubInterface) func(Party) error {
	return func(p Party) error {
		if s, ok := p.(*server); ok {
			s.newHub = factory
			return nil
		}
		return errors.New("option HubFactory is server only")
	}
}

// SimpleHubFactory sets a HubFactory which creates a new hub with the underlying type
// of hubProto on each hub method invocation.
func SimpleHubFactory(hubProto HubInterface) func(Party) error {
	return HubFactory(
		func() HubInterface {
			return reflect.New(reflect.ValueOf(hubProto).Elem().Type()).Interface().(HubInterface)
		})
}

// HTTPTransports sets the list of available transports for http connections. Allowed transports are
// "WebSockets", "ServerSentEvents". Default is both transports are available.
func HTTPTransports(transports ...TransportType) func(Party) error {
	return func(p Party) error {
		if s, ok := p.(*server); ok {
			for _, transport := range transports {
				switch transport {
				case TransportWebSockets, TransportServerSentEvents:
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

// InsecureSkipVerify disables Accepts origin verification behaviour which is used to avoid same origin strategy.
// See https://pkg.go.dev/github.com/coder/websocket#AcceptOptions
func InsecureSkipVerify(skip bool) func(Party) error {
	return func(p Party) error {
		p.setInsecureSkipVerify(skip)
		return nil
	}
}

// AllowOriginPatterns lists the host patterns for authorized origins which is used for avoid same origin strategy.
// See https://pkg.go.dev/github.com/coder/websocket#AcceptOptions
func AllowOriginPatterns(origins []string) func(Party) error {
	return func(p Party) error {
		p.setOriginPatterns(origins)
		return nil
	}
}

// WithRedisHubLifetimeManager configures the server to use a Redis-backed HubLifetimeManager.
// This enables scaling the SignalR server across multiple instances using Redis Pub/Sub and Sets.
// Provide redis.Options to connect to your Redis instance.
func WithRedisHubLifetimeManager(redisClient *redis.Client) func(Party) error {
	return func(p Party) error {
		s, ok := p.(*server)
		if !ok {
			return errors.New("option WithRedisHubLifetimeManager is server only")
		}

		// The RedisLifetimeManager needs access to the protocol to handle message serialization.
		// However, the protocol is determined *after* options are processed during the handshake.
		// This requires a slight adjustment: the Redis manager needs to be initialized with the protocol later.
		// For now, we'll create the manager and signal it needs protocol setup post-handshake.

		// Create the Redis manager (protocol will be set later)
		mgr, err := newRedisHubLifetimeManager(s.context(), s.info, redisClient, nil) // Pass nil for protocol initially
		if err != nil {
			return fmt.Errorf("failed to create Redis HubLifetimeManager: %w", err)
		}

		// Assign the new manager
		s.lifetimeManager = mgr

		// Note: We need a mechanism to set the protocol on the redisHubLifetimeManager
		// after the handshake is done in the Serve method. This requires modifying Serve.

		return nil
	}
}
