package signalr

import (
	"context"
	"fmt"
	"github.com/go-kit/kit/log"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io"
	"os"
	"strings"
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

func (s *simpleHub) InvokeMe(arg1 string, arg2 int) string {
	return fmt.Sprintf("%v%v", arg1, arg2)
}

func (s *simpleHub) Callback(arg1 string) {
	s.Hub.context.Clients().Caller().Send("OnCallback", strings.ToUpper(arg1))
}

func (s *simpleHub) ReadStream() chan string {
	ch := make(chan string)
	go func() {
		ch <- "A"
		ch <- "B"
		ch <- "C"
		ch <- "D"
		close(ch)
	}()
	return ch
}

type simpleReceiver struct {
	result string
}

func (s *simpleReceiver) OnCallback(result string) {
	s.result = result
}

var _ = Describe("ClientConnection", func() {
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
			// Create the ClientConnection
			clientConn, err := NewClientConnection(cliConn)
			Expect(err).NotTo(HaveOccurred())
			Expect(clientConn).NotTo(BeNil())
			// Start it
			err = <-clientConn.Start()
			Expect(err).NotTo(HaveOccurred())
			close(done)
		})
	})
	Context("Invoke", func() {
		server, _ := NewServer(SimpleHubFactory(&simpleHub{}),
			Logger(log.NewLogfmtLogger(os.Stderr), false),
			ChanReceiveTimeout(200*time.Millisecond),
			StreamBufferCapacity(5))
		// Create both ends of the connection
		cliConn, srvConn := newClientServerConnections()
		// Start the server
		go server.Run(context.TODO(), srvConn)
		// Create the ClientConnection
		clientConn, _ := NewClientConnection(cliConn)
		// Start it
		clientConn.SetReceiver(simpleReceiver{})
		<-clientConn.Start()
		It("should invoke a server method and return the result", func(done Done) {
			ch, errCh := clientConn.Invoke("InvokeMe", "A", 1)
			select {
			case val := <-ch:
				Expect(val).To(Equal("A1"))
			case err := <-errCh:
				Expect(err).NotTo(HaveOccurred())
			}
			close(done)
		})
		It("should invoke a server method and return the error when arguments don't match", func(done Done) {
			ch, errCh := clientConn.Invoke("InvokeMe", "A", "B")
			select {
			case <-ch:
				Fail("Value should not be returned")
			case err := <-errCh:
				Expect(err).To(HaveOccurred())
			}
			close(done)
		})
		It("should invoke a server method and return the result after a bad invocation", func(done Done) {
			clientConn.Invoke("InvokeMe", "A", "B")
			ch, errCh := clientConn.Invoke("InvokeMe", "A", 1)
			select {
			case val := <-ch:
				Expect(val).To(Equal("A1"))
			case err := <-errCh:
				Expect(err).NotTo(HaveOccurred())
			}
			close(done)
		})
	})
	Context("Send", func() {
		server, _ := NewServer(SimpleHubFactory(&simpleHub{}),
			Logger(log.NewLogfmtLogger(os.Stderr), false),
			ChanReceiveTimeout(200*time.Millisecond),
			StreamBufferCapacity(5))
		// Create both ends of the connection
		cliConn, srvConn := newClientServerConnections()
		// Start the server
		go server.Run(context.TODO(), srvConn)
		// Create the ClientConnection
		clientConn, _ := NewClientConnection(cliConn)
		// Start it
		receiver := &simpleReceiver{}
		clientConn.SetReceiver(receiver)
		<-clientConn.Start()
		It("should invoke a server method and get the result via callback", func(done Done) {
			receiver.result = ""
			errCh := clientConn.Send("Callback", "low")
			ch := make(chan string, 1)
			go func() {
				for {
					if receiver.result != "" {
						ch <- receiver.result
						break
					}
				}
			}()
			select {
			case val := <-ch:
				Expect(val).To(Equal("LOW"))
			case err := <-errCh:
				Expect(err).NotTo(HaveOccurred())
			}
			close(done)
		})
		It("should invoke a server method and return the error when arguments don't match", func(done Done) {
			receiver.result = ""
			errCh := clientConn.Send("Callback", 1)
			ch := make(chan string, 1)
			go func() {
				for {
					if receiver.result != "" {
						ch <- receiver.result
						break
					}
				}
			}()
			select {
			case <-ch:
				Fail("Value should not be returned")
			case err := <-errCh:
				Expect(err).To(HaveOccurred())
			}
			close(done)
		})

	})
	Context("PullStream", func() {
		server, _ := NewServer(SimpleHubFactory(&simpleHub{}),
			Logger(log.NewLogfmtLogger(os.Stderr), false),
			ChanReceiveTimeout(200*time.Millisecond),
			StreamBufferCapacity(5))
		// Create both ends of the connection
		cliConn, srvConn := newClientServerConnections()
		// Start the server
		go server.Run(context.TODO(), srvConn)
		// Create the ClientConnection
		clientConn, _ := NewClientConnection(cliConn)
		// Start it
		receiver := &simpleReceiver{}
		clientConn.SetReceiver(receiver)
		<-clientConn.Start()
		It("should pull a stream from the server", func(done Done) {
			ch, err := clientConn.PullStream("ReadStream")
			Expect(err).NotTo(HaveOccurred())
			values := make([]interface{}, 0)
			for val := range ch {
				values = append(values, val)
			}
			Expect(values).To(Equal([]interface{}{"A", "B", "C", "D"}))
			close(done)
		})
	})
	Context("GetConnectionID", func() {
		It("should return distinct IDs", func() {
			c, _ := NewClientConnection(nil)
			cc := c.(*clientConnection)
			ids := make(map[string]string, 0)
			for i := 1; i < 100000; i++ {
				id := cc.GetNewInvocationID()
				_, ok := ids[id]
				Expect(ok).To(BeFalse())
				ids[id] = id
			}
		})
	})
})
