package signalr

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSignalR(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SignalR Suite")
}

func connect(hubProto HubInterface) (Server, *testingConnection) {
	server, err := NewServer(context.TODO(), SimpleHubFactory(hubProto),
		testLoggerOption(),
		ChanReceiveTimeout(200*time.Millisecond),
		StreamBufferCapacity(5))
	if err != nil {
		Fail(err.Error())
		return nil, nil
	}
	conn := newTestingConnectionForServer()
	go func() { _ = server.Serve(conn) }()
	return server, conn
}
