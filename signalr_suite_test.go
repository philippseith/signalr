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

func connect(hubProto HubInterface) *testingHubConnection {
	server := NewServer(hubProto)
	conn := newTestingHubConnection()
	go server.messageLoop(conn, "bla", &JsonHubProtocol{})
	return conn
}




