package signalr

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSignalR(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SignalR Suite")
}

func connect(hubProto HubInterface) (Server, *testingConnection) {
	server, err := NewServer(context.TODO(), SimpleHubFactory(hubProto),
		Logger(log.NewLogfmtLogger(os.Stderr), false),
		ChanReceiveTimeout(200*time.Millisecond),
		StreamBufferCapacity(5))
	if err != nil {
		Fail(err.Error())
		return nil, nil
	}
	conn := newTestingConnectionForServer()
	go server.Serve(conn)
	return server, conn
}
