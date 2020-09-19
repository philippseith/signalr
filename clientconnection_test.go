package signalr

import (
	"context"
	"github.com/go-kit/kit/log"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io"
	"os"
	"time"
)

type pipeConnection struct {
	reader  io.Reader
	writer  io.Writer
	timeout time.Duration
}

func (pc *pipeConnection) Read(p []byte) (n int, err error) {
	return pc.reader.Read(p)
}

func (pc *pipeConnection) Write(p []byte) (n int, err error) {
	return pc.writer.Write(p)
}

func (pc *pipeConnection) ConnectionID() string {
	return "X"
}

func (pc *pipeConnection) SetTimeout(timeout time.Duration) {
	pc.timeout = timeout
}

func (pc *pipeConnection) Timeout() time.Duration {
	return pc.timeout
}

func newClientServerConnections() (cliConn *pipeConnection, svrConn *pipeConnection) {
	cliReader, srvWriter := io.Pipe()
	srvReader, cliWriter := io.Pipe()
	cliConn = &pipeConnection{
		reader: cliReader,
		writer: cliWriter,
	}
	svrConn = &pipeConnection{
		reader: srvReader,
		writer: srvWriter,
	}
	return cliConn, svrConn
}

type simpleHub struct {
	Hub
}

type simpleReceiver struct {
}

var _ = FDescribe("ClientConnection", func() {
	Context("Start", func() {
		It("should connect to the server", func(done Done) {
			// Create a simple server
			server, err := NewServer(SimpleHubFactory(&simpleHub{}),
				Logger(log.NewLogfmtLogger(os.Stderr), false),
				ChanReceiveTimeout(200*time.Millisecond),
				StreamBufferCapacity(5))
			Expect(err).NotTo(HaveOccurred())
			Expect(server).NotTo(BeNil())
			// Create both ends of the connection
			cliConn, srvConn := newClientServerConnections()
			// Start the server
			go server.Run(context.TODO(), srvConn)
			clientConn, err := NewClientConnection(cliConn)
			Expect(err).NotTo(HaveOccurred())
			Expect(clientConn).NotTo(BeNil())
			err = <-clientConn.Start()
			Expect(err).NotTo(HaveOccurred())
			close(done)
		})
	})
})
