package signalr

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSignalR(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SignalR Suite")
}

func connect(hubProto HubInterface) *testingConnection {
	if server, err := NewServer(SimpleTransientHubFactory(hubProto)); err != nil {
		Fail(err.Error())
		return nil
	} else {
		conn := newTestingConnection()
		go server.Run(conn)
		return conn
	}
}
