package signalr

import (
	"github.com/go-kit/kit/log"
	"reflect"
)

func HubSingleton(hub HubInterface) func(*Server) error {
	return func(s *Server) error {
		s.newHub = func() HubInterface { return hub }
		return nil
	}
}

func TransientHubFactory(factoryFunc func() HubInterface) func(*Server) error {
	return func(s *Server) error {
		s.newHub = factoryFunc
		return nil
	}
}

func SimpleTransientHubFactory(hubProto HubInterface) func(*Server) error {
	return TransientHubFactory(
		func() HubInterface {
			return reflect.New(reflect.ValueOf(hubProto).Elem().Type()).Interface().(HubInterface)
		})
}

func Logger(logger log.Logger, debug bool) func(*Server) error {
	return func(s *Server) error {
		i, d := buildInfoDebugLogger(logger, debug)
		s.info = i
		s.debug = d
		return nil
	}
}

