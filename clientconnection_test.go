package signalr

import (
	"context"
	"errors"
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
	fail    error
}

func (pc *pipeConnection) Read(p []byte) (n int, err error) {
	if pc.fail != nil {
		return 0, pc.fail
	}
	return pc.reader.Read(p)
}

func (pc *pipeConnection) Write(p []byte) (n int, err error) {
	if pc.fail != nil {
		return 0, pc.fail
	}
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
	receiveStreamArg        string
	receiveStreamChanValues []int
	receiveStreamDone       chan struct{}
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

func (s *simpleHub) ReceiveStream(arg string, ch <-chan int) {
	s.receiveStreamArg = arg
	s.receiveStreamChanValues = make([]int, 0)
	go func(ch <-chan int, done chan struct{}) {
		for v := range ch {
			s.receiveStreamChanValues = append(s.receiveStreamChanValues, v)
		}
		done <- struct{}{}
	}(ch, s.receiveStreamDone)
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
			server, err := NewServer(context.TODO(), SimpleHubFactory(&simpleHub{}),
				Logger(log.NewLogfmtLogger(os.Stderr), false),
				ChanReceiveTimeout(200*time.Millisecond),
				StreamBufferCapacity(5))
			Expect(err).NotTo(HaveOccurred())
			Expect(server).NotTo(BeNil())
			// Create both ends of the connection
			cliConn, srvConn := newClientServerConnections()
			// Start the server
			svrCtx, svrCancel := context.WithCancel(context.Background())
			go server.Run(svrCtx, srvConn)
			// Create the ClientConnection
			clientConn, err := NewClientConnection(context.TODO(), cliConn)
			Expect(err).NotTo(HaveOccurred())
			Expect(clientConn).NotTo(BeNil())
			// Start it
			err = <-clientConn.Start()
			Expect(err).NotTo(HaveOccurred())
			err = clientConn.Close()
			Expect(err).NotTo(HaveOccurred())
			svrCancel()
			close(done)
		}, 1.0)
	})
	Context("Invoke", func() {
		var cliConn *pipeConnection
		var srvConn *pipeConnection
		var clientConn ClientConnection
		var svrCtx context.Context
		var svrCancel context.CancelFunc
		BeforeEach(func(done Done) {
			server, _ := NewServer(context.TODO(), SimpleHubFactory(&simpleHub{}),
				Logger(log.NewLogfmtLogger(os.Stderr), false),
				ChanReceiveTimeout(200*time.Millisecond),
				StreamBufferCapacity(5))
			// Create both ends of the connection
			cliConn, srvConn = newClientServerConnections()
			// Start the server
			svrCtx, svrCancel = context.WithCancel(context.Background())
			go server.Run(svrCtx, srvConn)
			// Create the ClientConnection
			clientConn, _ = NewClientConnection(context.TODO(), cliConn)
			// Start it
			clientConn.SetReceiver(simpleReceiver{})
			<-clientConn.Start()
			close(done)
		}, 2.0)
		AfterEach(func(done Done) {
			_ = clientConn.Close()
			svrCancel()
			close(done)
		}, 2.0)

		It("should invoke a server method and return the result", func(done Done) {
			r := <-clientConn.Invoke("InvokeMe", "A", 1)
			Expect(r.Value).To(Equal("A1"))
			Expect(r.Error).NotTo(HaveOccurred())
			close(done)
		}, 2.0)
		It("should invoke a server method and return the error when arguments don't match", func(done Done) {
			r := <-clientConn.Invoke("InvokeMe", "A", "B")
			Expect(r.Error).To(HaveOccurred())
			close(done)
		}, 2.0)
		It("should invoke a server method and return the result after a bad invocation", func(done Done) {
			clientConn.Invoke("InvokeMe", "A", "B")
			r := <-clientConn.Invoke("InvokeMe", "A", 1)
			Expect(r.Value).To(Equal("A1"))
			Expect(r.Error).NotTo(HaveOccurred())
			close(done)
		}, 2.0)
		XIt("should return an error when the connection fails", func(done Done) {
			cliConn.fail = errors.New("fail")
			r := <-clientConn.Invoke("InvokeMe", "A", 1)
			Expect(r.Error).To(HaveOccurred())
			close(done)
		}, 2.0)
	})
	Context("Send", func() {
		var cliConn *pipeConnection
		var srvConn *pipeConnection
		var clientConn ClientConnection
		var receiver *simpleReceiver
		var svrCtx context.Context
		var svrCancel context.CancelFunc
		BeforeEach(func(done Done) {
			server, _ := NewServer(context.TODO(), SimpleHubFactory(&simpleHub{}),
				Logger(log.NewLogfmtLogger(os.Stderr), false),
				ChanReceiveTimeout(200*time.Millisecond),
				StreamBufferCapacity(5))
			// Create both ends of the connection
			cliConn, srvConn = newClientServerConnections()
			// Start the server
			svrCtx, svrCancel = context.WithCancel(context.Background())
			go server.Run(svrCtx, srvConn)
			// Create the ClientConnection
			clientConn, _ = NewClientConnection(context.TODO(), cliConn)
			// Start it
			receiver = &simpleReceiver{}
			clientConn.SetReceiver(receiver)
			<-clientConn.Start()
			close(done)
		}, 2.0)
		AfterEach(func(done Done) {
			_ = clientConn.Close()
			svrCancel()
			close(done)
		}, 2.0)

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
		}, 2.0)
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
			// Stop the above go func
			receiver.result = "Stop"
			close(done)
		}, 2.0)
		XIt("should return an error when the connection fails", func(done Done) {
			cliConn.fail = errors.New("fail")
			err := <-clientConn.Send("Callback", 1)
			Expect(err).To(HaveOccurred())
			close(done)
		}, 2.0)
	})
	Context("PullStream", func() {
		var cliConn *pipeConnection
		var srvConn *pipeConnection
		var clientConn ClientConnection
		var svrCtx context.Context
		var svrCancel context.CancelFunc
		BeforeEach(func(done Done) {
			server, _ := NewServer(context.TODO(), SimpleHubFactory(&simpleHub{}),
				Logger(log.NewLogfmtLogger(os.Stderr), false),
				ChanReceiveTimeout(200*time.Millisecond),
				StreamBufferCapacity(5))
			// Create both ends of the connection
			cliConn, srvConn = newClientServerConnections()
			// Start the server
			svrCtx, svrCancel = context.WithCancel(context.Background())
			go server.Run(svrCtx, srvConn)
			// Create the ClientConnection
			clientConn, _ = NewClientConnection(context.TODO(), cliConn)
			// Start it
			receiver := &simpleReceiver{}
			clientConn.SetReceiver(receiver)
			<-clientConn.Start()
			close(done)
		}, 2.0)
		AfterEach(func(done Done) {
			_ = clientConn.Close()
			svrCancel()
			close(done)
		}, 2.0)

		It("should pull a stream from the server", func(done Done) {
			ch := clientConn.PullStream("ReadStream")
			values := make([]interface{}, 0)
			for r := range ch {
				Expect(r.Error).NotTo(HaveOccurred())
				values = append(values, r.Value)
			}
			Expect(values).To(Equal([]interface{}{"A", "B", "C", "D"}))
			close(done)
		})
		It("should return no error when the method returns no stream but a single result", func(done Done) {
			r := <-clientConn.PullStream("InvokeMe", "A", 1)
			Expect(r.Error).NotTo(HaveOccurred())
			Expect(r.Value).To(Equal("A1"))
			close(done)
		}, 2.0)
		It("should return an error when the method returns no result", func(done Done) {
			r := <-clientConn.PullStream("Callback", "A")
			Expect(r.Error).To(HaveOccurred())
			close(done)
		}, 2.0)
		It("should return an error when the method does not exist on the server", func(done Done) {
			r := <-clientConn.PullStream("ReadStream2")
			Expect(r.Error).To(HaveOccurred())
			close(done)
		}, 2.0)
		It("should return an error when the method arguments are not matching", func(done Done) {
			r := <-clientConn.PullStream("ReadStream", "A", 1)
			Expect(r.Error).To(HaveOccurred())
			close(done)
		}, 2.0)
		XIt("should return an error when the connection fails", func(done Done) {
			cliConn.fail = errors.New("fail")
			r := <-clientConn.PullStream("ReadStream")
			Expect(r.Error).To(HaveOccurred())
			close(done)
		}, 2.0)
	})
	Context("GetConnectionID", func() {
		It("should return distinct IDs", func(done Done) {
			c, _ := NewClientConnection(context.TODO(), nil)
			cc := c.(*clientConnection)
			ids := make(map[string]string)
			for i := 1; i < 10000; i++ {
				id := cc.GetNewID()
				_, ok := ids[id]
				Expect(ok).To(BeFalse())
				ids[id] = id
			}
			close(done)
		})
	})
	Context("PushStreams", func() {
		var cliConn *pipeConnection
		var srvConn *pipeConnection
		var clientConn ClientConnection
		var hub *simpleHub
		var svrCtx context.Context
		var svrCancel context.CancelFunc
		BeforeEach(func(done Done) {
			hub = &simpleHub{}
			hub.receiveStreamDone = make(chan struct{}, 1)
			server, _ := NewServer(context.TODO(), HubFactory(func() HubInterface { return hub }),
				Logger(log.NewLogfmtLogger(os.Stderr), false),
				ChanReceiveTimeout(200*time.Millisecond),
				StreamBufferCapacity(5))
			// Create both ends of the connection
			cliConn, srvConn = newClientServerConnections()
			// Start the server
			svrCtx, svrCancel = context.WithCancel(context.Background())
			go server.Run(svrCtx, srvConn)
			// Create the ClientConnection
			clientConn, _ = NewClientConnection(context.TODO(), cliConn)
			// Start it
			receiver := &simpleReceiver{}
			clientConn.SetReceiver(receiver)
			<-clientConn.Start()
			close(done)
		}, 2.0)
		AfterEach(func(done Done) {
			_ = clientConn.Close()
			svrCancel()
			close(done)
		}, 2.0)

		It("should push a stream to the server", func(done Done) {
			ch := make(chan int, 1)
			_ = clientConn.PushStreams("ReceiveStream", "test", ch)
			go func(ch chan int) {
				for i := 1; i < 5; i++ {
					ch <- i
				}
				close(ch)
			}(ch)
			<-hub.receiveStreamDone
			Expect(hub.receiveStreamArg).To(Equal("test"))
			close(done)
		})

		XIt("should return an error when the connection fails", func(done Done) {
			cliConn.fail = errors.New("fail")
			ch := make(chan int, 1)
			err := <-clientConn.PushStreams("ReceiveStream", "test", ch)
			Expect(err).To(HaveOccurred())
			close(done)
		}, 2.0)
	})
})
