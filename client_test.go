package signalr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type pipeConnection struct {
	reader       io.Reader
	writer       io.Writer
	timeout      time.Duration
	fail         atomic.Value
	connectionID string
}

func (pc *pipeConnection) Context() context.Context {
	return context.TODO()
}

func (pc *pipeConnection) Read(p []byte) (n int, err error) {
	if err, ok := pc.fail.Load().(error); ok {
		return 0, err
	}
	return pc.reader.Read(p)
}

func (pc *pipeConnection) Write(p []byte) (n int, err error) {
	if err, ok := pc.fail.Load().(error); ok {
		return 0, err
	}
	return pc.writer.Write(p)
}

func (pc *pipeConnection) ConnectionID() string {
	return pc.connectionID
}

func (pc *pipeConnection) SetConnectionID(cID string) {
	pc.connectionID = cID
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
		reader:       cliReader,
		writer:       cliWriter,
		connectionID: "X",
	}
	svrConn = &pipeConnection{
		reader:       srvReader,
		writer:       srvWriter,
		connectionID: "X",
	}
	return cliConn, svrConn
}

type simpleHub struct {
	Hub
	receiveStreamArg  string
	receiveStreamDone chan struct{}
}

func (s *simpleHub) InvokeMe(arg1 string, arg2 int) string {
	return fmt.Sprintf("%v%v", arg1, arg2)
}

func (s *simpleHub) Callback(arg1 string) {
	s.Hub.Clients().Caller().Send("OnCallback", strings.ToUpper(arg1))
}

func (s *simpleHub) ReadStream(i int) chan string {
	ch := make(chan string)
	go func() {
		ch <- fmt.Sprintf("A%v", i)
		ch <- fmt.Sprintf("B%v", i)
		ch <- fmt.Sprintf("C%v", i)
		ch <- fmt.Sprintf("D%v", i)
		close(ch)
	}()
	return ch
}

func (s *simpleHub) ReceiveStream(arg string, ch <-chan int) int {
	s.receiveStreamArg = arg
	receiveStreamChanValues := make([]int, 0)
	for v := range ch {
		receiveStreamChanValues = append(receiveStreamChanValues, v)
	}
	s.receiveStreamDone <- struct{}{}
	return 100
}

func (s *simpleHub) Abort() {
	s.Hub.Abort()
}

type simpleReceiver struct {
	result atomic.Value
	ch     chan string
}

func (s *simpleReceiver) OnCallback(result string) {
	s.ch <- result
}

type noLogAfterStopLogger struct {
	StructuredLogger
	shouldPanic atomic.Bool
}

func (n *noLogAfterStopLogger) Log(keyVals ...interface{}) error {
	if n.shouldPanic.Load() {
		panic("oh no")
	}
	return n.StructuredLogger.Log(keyVals)
}

var _ = Describe("Client", func() {
	formatOption := TransferFormat("Text")
	j := 1
	Context("Start/Cancel", func() {
		It("should connect to the server and then be stopped without error", func(done Done) {
			// Create a simple server
			server, err := NewServer(context.TODO(), SimpleHubFactory(&simpleHub{}),
				testLoggerOption(),
				ChanReceiveTimeout(200*time.Millisecond),
				StreamBufferCapacity(5))
			Expect(err).NotTo(HaveOccurred())
			Expect(server).NotTo(BeNil())
			// Create both ends of the connection
			cliConn, srvConn := newClientServerConnections()
			// Start the server
			go func() { _ = server.Serve(srvConn) }()
			// Create the Client
			ctx, cancelClient := context.WithCancel(context.Background())
			clientConn, err := NewClient(ctx, WithConnection(cliConn), testLoggerOption(), formatOption)
			Expect(err).NotTo(HaveOccurred())
			Expect(clientConn).NotTo(BeNil())
			// Start it
			clientConn.Start()
			Expect(<-clientConn.WaitForState(context.Background(), ClientConnected)).NotTo(HaveOccurred())
			cancelClient()
			server.cancel()
			close(done)
		}, 1.0)
	})
	Context("Stop", func() {
		It("should stop the client properly", func(done Done) {
			// Create a simple server
			server, err := NewServer(context.TODO(), SimpleHubFactory(&simpleHub{}),
				testLoggerOption(),
				ChanReceiveTimeout(200*time.Millisecond),
				StreamBufferCapacity(5))
			Expect(err).NotTo(HaveOccurred())
			Expect(server).NotTo(BeNil())
			// Create both ends of the connection
			cliConn, srvConn := newClientServerConnections()
			// Start the server
			go func() { _ = server.Serve(srvConn) }()
			// Create the Client
			clientConn, err := NewClient(context.Background(), WithConnection(cliConn), testLoggerOption(), formatOption)
			Expect(err).NotTo(HaveOccurred())
			Expect(clientConn).NotTo(BeNil())
			// Start it
			clientConn.Start()
			Expect(<-clientConn.WaitForState(context.Background(), ClientConnected)).NotTo(HaveOccurred())
			clientConn.Stop()
			Expect(clientConn.State()).To(BeEquivalentTo(ClientClosed))
			server.cancel()
			close(done)
		})
		It("should not log after stop", func(done Done) {
			// Create a simple server
			server, err := NewServer(context.TODO(), SimpleHubFactory(&simpleHub{}),
				testLoggerOption(),
				ChanReceiveTimeout(200*time.Millisecond),
				StreamBufferCapacity(5))
			Expect(err).NotTo(HaveOccurred())
			Expect(server).NotTo(BeNil())
			// Create both ends of the connection
			cliConn, srvConn := newClientServerConnections()
			// Start the server
			go func() { _ = server.Serve(srvConn) }()
			// Create the Client
			clientConn, err := NewClient(context.Background(), WithConnection(cliConn), testLoggerOption(), formatOption)
			Expect(err).NotTo(HaveOccurred())
			Expect(clientConn).NotTo(BeNil())
			// Replace loggers with loggers that panic after stop
			info, debug := clientConn.loggers()
			panicableInfo, panicableDebug := &noLogAfterStopLogger{StructuredLogger: info}, &noLogAfterStopLogger{StructuredLogger: debug}
			clientConn.setLoggers(panicableInfo, panicableDebug)
			// Start it
			clientConn.Start()
			Expect(<-clientConn.WaitForState(context.Background(), ClientConnected)).NotTo(HaveOccurred())
			clientConn.Stop()
			panicableInfo.shouldPanic.Store(true)
			panicableDebug.shouldPanic.Store(true)
			// Ensure that we really don't get any logs anymore
			time.Sleep(500 * time.Millisecond)
			Expect(clientConn.State()).To(BeEquivalentTo(ClientClosed))
			server.cancel()
			close(done)
		})
	})
	Context("Invoke", func() {
		It("should invoke a server method and return the result", func(done Done) {
			_, client, _, cancelClient := getTestBed(&simpleReceiver{}, formatOption)
			r := <-client.Invoke("InvokeMe", "A", 1)
			Expect(r.Value).To(Equal("A1"))
			Expect(r.Error).NotTo(HaveOccurred())
			cancelClient()
			close(done)
		}, 2.0)
		It("should invoke a server method and return the error when arguments don't match", func(done Done) {
			_, client, _, cancelClient := getTestBed(&simpleReceiver{}, formatOption)
			r := <-client.Invoke("InvokeMe", "A", "B")
			Expect(r.Error).To(HaveOccurred())
			cancelClient()
			close(done)
		}, 2.0)
		It("should invoke a server method and return the result after a bad invocation", func(done Done) {
			_, client, _, cancelClient := getTestBed(&simpleReceiver{}, formatOption)
			client.Invoke("InvokeMe", "A", "B")
			r := <-client.Invoke("InvokeMe", "A", 1)
			Expect(r.Value).To(Equal("A1"))
			Expect(r.Error).NotTo(HaveOccurred())
			cancelClient()
			close(done)
		}, 2.0)
		It(fmt.Sprintf("should return an error when the connection fails: invocation %v", j), func(done Done) {
			_, client, cliConn, cancelClient := getTestBed(&simpleReceiver{}, formatOption)
			cliConn.fail.Store(errors.New("fail"))
			r := <-client.Invoke("InvokeMe", "A", 1)
			Expect(r.Error).To(HaveOccurred())
			cancelClient()
			close(done)
		}, 1.0)
	})
	Context("Send", func() {
		It("should invoke a server method and get the result via callback", func(done Done) {
			receiver := &simpleReceiver{}
			_, client, _, cancelClient := getTestBed(receiver, formatOption)
			receiver.result.Store("x")
			errCh := client.Send("Callback", "low")
			ch := make(chan string, 1)
			go func() {
				for {
					if result, ok := receiver.result.Load().(string); ok {
						if result != "x" {
							ch <- result
							break
						}
					}
				}
			}()
			select {
			case val := <-ch:
				Expect(val).To(Equal("LOW"))
			case err := <-errCh:
				Expect(err).NotTo(HaveOccurred())
			}
			cancelClient()
			close(done)
		}, 1.0)
		It("should invoke a server method and return the error when arguments don't match", func(done Done) {
			receiver := &simpleReceiver{}
			_, client, _, cancelClient := getTestBed(receiver, formatOption)
			receiver.result.Store("x")
			errCh := client.Send("Callback", 1)
			ch := make(chan string, 1)
			go func() {
				for {
					if result, ok := receiver.result.Load().(string); ok {
						if result != "x" {
							ch <- result
							break
						}
					}
				}
			}()
			select {
			case val := <-ch:
				Fail(fmt.Sprintf("Value %v should not be returned", val))
			case err := <-errCh:
				Expect(err).To(HaveOccurred())
			}
			// Stop the above go func
			receiver.result.Store("Stop")
			cancelClient()
			close(done)
		}, 2.0)
		It(fmt.Sprintf("should return an error when the connection fails: invocation %v", j), func(done Done) {
			_, client, cliConn, cancelClient := getTestBed(&simpleReceiver{}, formatOption)
			cliConn.fail.Store(errors.New("fail"))
			err := <-client.Send("Callback", 1)
			Expect(err).To(HaveOccurred())
			cancelClient()
			close(done)
		}, 1.0)
	})
	Context("PullStream", func() {
		j := 1
		It("should pull a stream from the server", func(done Done) {
			_, client, _, cancelClient := getTestBed(&simpleReceiver{}, formatOption)
			ch := client.PullStream("ReadStream", j)
			values := make([]interface{}, 0)
			for r := range ch {
				Expect(r.Error).NotTo(HaveOccurred())
				values = append(values, r.Value)
			}
			Expect(values).To(Equal([]interface{}{
				fmt.Sprintf("A%v", j),
				fmt.Sprintf("B%v", j),
				fmt.Sprintf("C%v", j),
				fmt.Sprintf("D%v", j),
			}))
			cancelClient()
			close(done)
		})
		It("should return no error when the method returns no stream but a single result", func(done Done) {
			_, client, _, cancelClient := getTestBed(&simpleReceiver{}, formatOption)
			r := <-client.PullStream("InvokeMe", "A", 1)
			Expect(r.Error).NotTo(HaveOccurred())
			Expect(r.Value).To(Equal("A1"))
			cancelClient()
			close(done)
		}, 2.0)
		It("should return an error when the method returns no result", func(done Done) {
			_, client, _, cancelClient := getTestBed(&simpleReceiver{}, formatOption)
			r := <-client.PullStream("Callback", "A")
			Expect(r.Error).To(HaveOccurred())
			cancelClient()
			close(done)
		}, 2.0)
		It("should return an error when the method does not exist on the server", func(done Done) {
			_, client, _, cancelClient := getTestBed(&simpleReceiver{}, formatOption)
			r := <-client.PullStream("ReadStream2")
			Expect(r.Error).To(HaveOccurred())
			cancelClient()
			close(done)
		}, 2.0)
		It("should return an error when the method arguments are not matching", func(done Done) {
			_, client, _, cancelClient := getTestBed(&simpleReceiver{}, formatOption)
			r := <-client.PullStream("ReadStream", "A", 1)
			Expect(r.Error).To(HaveOccurred())
			cancelClient()
			close(done)
		}, 2.0)
		It("should return an error when the connection fails", func(done Done) {
			_, client, cliConn, cancelClient := getTestBed(&simpleReceiver{}, formatOption)
			cliConn.fail.Store(errors.New("fail"))
			r := <-client.PullStream("ReadStream")
			Expect(r.Error).To(HaveOccurred())
			cancelClient()
			close(done)
		}, 2.0)
	})
	Context("PushStreams", func() {
		var cliConn *pipeConnection
		var srvConn *pipeConnection
		var client Client
		var cancelClient context.CancelFunc
		var server Server
		hub := &simpleHub{}
		BeforeEach(func(done Done) {
			hub.receiveStreamDone = make(chan struct{}, 1)
			server, _ = NewServer(context.TODO(), HubFactory(func() HubInterface { return hub }),
				testLoggerOption(),
				ChanReceiveTimeout(200*time.Millisecond),
				StreamBufferCapacity(5))
			// Create both ends of the connection
			cliConn, srvConn = newClientServerConnections()
			// Start the server
			go func() { _ = server.Serve(srvConn) }()
			// Create the Client
			receiver := &simpleReceiver{}
			var ctx context.Context
			ctx, cancelClient = context.WithCancel(context.Background())
			client, _ = NewClient(ctx, WithConnection(cliConn), WithReceiver(receiver), testLoggerOption(), formatOption)
			// Start it
			client.Start()
			Expect(<-client.WaitForState(context.Background(), ClientConnected)).NotTo(HaveOccurred())
			close(done)
		}, 2.0)
		AfterEach(func(done Done) {
			cancelClient()
			server.cancel()
			close(done)
		}, 2.0)

		It("should push a stream to the server", func(done Done) {
			ch := make(chan int, 1)
			r := client.PushStreams("ReceiveStream", "test", ch)
			go func(ch chan int) {
				for i := 1; i < 5; i++ {
					ch <- i
				}
				close(ch)
			}(ch)
			<-hub.receiveStreamDone
			ir := <-r
			Expect(ir.Error).To(BeNil())
			Expect(ir.Value).To(Equal(float64(100)))
			Expect(hub.receiveStreamArg).To(Equal("test"))
			cancelClient()
			close(done)
		}, 1.0)

		It("should return an error when the connection fails", func(done Done) {
			cliConn.fail.Store(errors.New("fail"))
			ch := make(chan int, 1)
			ir := <-client.PushStreams("ReceiveStream", "test", ch)
			Expect(ir.Error).To(HaveOccurred())
			cancelClient()
			close(done)
		}, 1.0)
	})

	Context("Reconnect", func() {
		var cliConn *pipeConnection
		var srvConn *pipeConnection
		var client Client
		var cancelClient context.CancelFunc
		var server Server
		hub := &simpleHub{}
		BeforeEach(func(done Done) {
			hub.receiveStreamDone = make(chan struct{}, 1)
			server, _ = NewServer(context.TODO(), HubFactory(func() HubInterface { return hub }),
				testLoggerOption(),
				ChanReceiveTimeout(200*time.Millisecond),
				StreamBufferCapacity(5))
			// Create both ends of the connection
			cliConn, srvConn = newClientServerConnections()
			// Start the server
			go func() { _ = server.Serve(srvConn) }()
			// Create the Client
			receiver := &simpleReceiver{}
			var ctx context.Context
			ctx, cancelClient = context.WithCancel(context.Background())
			client, _ = NewClient(ctx, WithConnection(cliConn), WithReceiver(receiver), testLoggerOption(), formatOption)
			// Start it
			client.Start()
			Expect(<-client.WaitForState(context.Background(), ClientConnected)).NotTo(HaveOccurred())
			close(done)
		}, 2.0)
		AfterEach(func(done Done) {
			cancelClient()
			server.cancel()
			close(done)
		}, 2.0)
		// TODO
	})
})

func getTestBed(receiver interface{}, formatOption func(Party) error) (Server, Client, *pipeConnection, context.CancelFunc) {
	server, _ := NewServer(context.TODO(), SimpleHubFactory(&simpleHub{}),
		testLoggerOption(),
		ChanReceiveTimeout(200*time.Millisecond),
		StreamBufferCapacity(5))
	// Create both ends of the connection
	cliConn, srvConn := newClientServerConnections()
	// Start the server
	go func() { _ = server.Serve(srvConn) }()
	// Create the Client
	var ctx context.Context
	ctx, cancelClient := context.WithCancel(context.Background())
	client, _ := NewClient(ctx, WithConnection(cliConn), WithReceiver(receiver), testLoggerOption(), formatOption)
	// Start it
	client.Start()
	return server, client, cliConn, cancelClient
}
