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
	server := NewServer(hubProto)
	conn := newTestingConnection()
	hubConn := newHubConnection(conn, &JsonHubProtocol{})
	hubConn.Start()
	go server.messageLoop(hubConn, &JsonHubProtocol{})
	return conn
}




