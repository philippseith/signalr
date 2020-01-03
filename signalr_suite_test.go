package signalr

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSignalr(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Signalr Suite")
}

func connect(hubProto HubInterface) *testingConnection {
	server := NewServer(func() HubInterface {
		return CreateInstance(hubProto)
	})
	conn := newTestingConnection()
	go server.Run(conn)
	return conn
}
