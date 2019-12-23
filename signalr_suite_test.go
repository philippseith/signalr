package signalr

import (
	"io"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSignalr(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Signalr Suite")
}

type testingHubConnection struct {
	baseHubConnection
	cliReader io.Reader
	srvReader io.Reader
	cliWriter io.Writer
}

func newTestingHubConnection() *testingHubConnection {
	cliReader, srvWriter := io.Pipe()
	srvReader, cliWriter := io.Pipe()
	return &testingHubConnection{
		baseHubConnection: baseHubConnection{
			connectionID: "TestID",
			protocol:     &jsonHubProtocol{},
			connected:    0,
			writer:       srvWriter,
		},
		cliReader: cliReader,
		srvReader: srvReader,
		cliWriter: cliWriter,
	}
}

func (t *testingHubConnection) clientSend(message string) {
	t.cliWriter.Write([]byte(message))
}

func (t *testingHubConnection) receive() (interface{}, error) {

}

