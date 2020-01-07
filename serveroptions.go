package signalr

import (
	"reflect"
)

func UseHub(hub HubInterface) func(*Server) error {
	return func(s *Server) error {
		s.newHub = func() HubInterface { return hub }
		return nil
	}
}

func HubFactory(factoryFunc func() HubInterface) func(*Server) error {
	return func(s *Server) error {
		s.newHub = factoryFunc
		return nil
	}
}

func SimpleHubFactory(hubProto HubInterface) func(*Server) error {
	return HubFactory(
		func() HubInterface {
			return reflect.New(reflect.ValueOf(hubProto).Elem().Type()).Interface().(HubInterface)
		})
}

type StructuredLogger interface {
	Log(keyvals ...interface{}) error
}

func Logger(logger StructuredLogger, debug bool) func(*Server) error {
	return func(s *Server) error {
		i, d := buildInfoDebugLogger(logger, debug)
		s.info = i
		s.dbg = d
		return nil
	}
}

