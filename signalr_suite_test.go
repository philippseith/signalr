package signalr_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/philippseith/signalr"
)

func TestSignalr(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Signalr Suite")
}

func connect(hubProto signalr.HubInterface) *testingHubConnection {
	server := signalr.NewServer(hubProto)
	conn := newTestingHubConnection()
	go server.MessageLoop(conn, "bla", &signalr.JsonHubProtocol{})
	return conn
}




