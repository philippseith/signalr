package signalr

import (
	"os"
	"testing"

	"github.com/go-kit/kit/log"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSignalR(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SignalR Suite")
}

func connect(hubProto HubInterface) *testingConnection {
	server, err := NewServer(SimpleHubFactory(hubProto),
		Logger(log.NewLogfmtLogger(os.Stderr), false))
	if err != nil {
		Fail(err.Error())
		return nil
	}
	conn := newTestingConnection()
	go server.Run(conn)
	return conn
}
