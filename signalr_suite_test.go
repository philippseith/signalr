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
	server := newServer(func() HubInterface {
		return Clone(hubProto)
	})
	conn := newTestingConnection()
	go server.messageLoop(conn)
	return conn
}
