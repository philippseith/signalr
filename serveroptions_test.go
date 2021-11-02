package signalr

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type singleHub struct {
	Hub
	id string
}

func (s *singleHub) OnConnected(string) {
	s.initUUID()
	singleHubMsg <- s.id
}

func (s *singleHub) GetUUID() string {
	s.initUUID()
	singleHubMsg <- s.id
	return s.id
}

func (s *singleHub) initUUID() {
	if s.id == "" {
		s.id = uuid.New().String()
	}
}

var singleHubMsg = make(chan string, 100)

var _ = Describe("Server options", func() {

	Describe("UseHub option", func() {
		Context("When the UseHub option is used", func() {
			It("should use the same hub instance on all invocations", func(done Done) {
				server, err := NewServer(context.TODO(), UseHub(&singleHub{}))
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn1 := newTestingConnectionForServer()
				Expect(conn1).NotTo(BeNil())
				go func() { _ = server.Serve(conn1) }()
				<-singleHubMsg
				conn2 := newTestingConnectionForServer()
				Expect(conn2).NotTo(BeNil())
				go func() { _ = server.Serve(conn2) }()
				<-singleHubMsg
				// Call GetId twice. If different instances used the results are different
				var uuid string
				conn1.ClientSend(`{"type":1,"invocationId": "123","target":"getuuid"}`)
				<-singleHubMsg
				select {
				case message := <-conn1.received:
					Expect(message).To(BeAssignableToTypeOf(completionMessage{}))
					uuid = fmt.Sprint(message.(completionMessage).Result)
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
				conn1.ClientSend(`{"type":1,"invocationId": "456","target":"getuuid"}`)
				<-singleHubMsg
				select {
				case message := <-conn1.received:
					Expect(message).To(BeAssignableToTypeOf(completionMessage{}))
					Expect(fmt.Sprint(message.(completionMessage).Result)).To(Equal(uuid))
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
				// Even on another connection, the uuid should be the same
				conn2.ClientSend(`{"type":1,"invocationId": "456","target":"getuuid"}`)
				<-singleHubMsg
				select {
				case message := <-conn2.received:
					Expect(message).To(BeAssignableToTypeOf(completionMessage{}))
					Expect(fmt.Sprint(message.(completionMessage).Result)).To(Equal(uuid))
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
				close(done)
			}, 3.0)
		})
		Context("When UseHub is used on a client", func() {
			It("should return an error", func(done Done) {
				_, err := NewClient(context.TODO(), WithConnection(newTestingConnection()), testLoggerOption(), UseHub(&singleHub{}))
				Expect(err).To(HaveOccurred())
				close(done)
			})
		})
	})

	Describe("SimpleHubFactory option", func() {
		Context("When the SimpleHubFactory option is used", func() {
			It("should call the hub factory on each hub method invocation", func(done Done) {
				server, err := NewServer(context.TODO(), SimpleHubFactory(&singleHub{}), testLoggerOption())
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn := newTestingConnectionForServer()
				Expect(conn).NotTo(BeNil())
				go func() { _ = server.Serve(conn) }()
				uuids := make(map[string]interface{})
				uuid := <-singleHubMsg
				if _, ok := uuids[uuid]; ok {
					Fail("same hub called twice")
				} else {
					uuids[uuid] = nil
				}
				conn.ClientSend(`{"type":1,"invocationId": "123","target":"getuuid"}`)
				select {
				case uuid = <-singleHubMsg:
					if _, ok := uuids[uuid]; ok {
						Fail("same hub called twice")
					} else {
						uuids[uuid] = nil
					}
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
				conn.ClientSend(`{"type":1,"invocationId": "456","target":"getuuid"}`)
				select {
				case uuid = <-singleHubMsg:
					if _, ok := uuids[uuid]; ok {
						Fail("same hub called twice")
					} else {
						uuids[uuid] = nil
					}
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
				close(done)
			}, 2.0)
		})
		Context("When SimpleHubFactory is used on a client", func() {
			It("should return an error", func(done Done) {
				_, err := NewClient(context.TODO(), WithConnection(newTestingConnection()), testLoggerOption(), SimpleHubFactory(&singleHub{}))
				Expect(err).To(HaveOccurred())
				close(done)
			})
		})
	})

	Describe("Logger option", func() {
		Context("When the Logger option with debug false is used", func() {
			It("calling a method correctly should log no events", func(done Done) {
				cw := newChannelWriter()
				server, err := NewServer(context.TODO(), UseHub(&invocationHub{}), Logger(log.NewLogfmtLogger(cw), false))
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn := newTestingConnectionForServer()
				Expect(conn).NotTo(BeNil())
				go func() { _ = server.Serve(conn) }()
				conn.ClientSend(`{"type":1,"invocationId": "123","target":"simple"}`)
				<-invocationQueue
				select {
				case logEntry := <-cw.Chan():
					Fail(fmt.Sprintf("log entry written: %v", string(logEntry)))
				case <-time.After(1000 * time.Millisecond):
					break
				}
				close(done)
			}, 2.0)
		})
		Context("When the Logger option with debug true is used", func() {
			It("calling a method correctly should log events", func(done Done) {
				cw := newChannelWriter()
				server, err := NewServer(context.TODO(), UseHub(&invocationHub{}), Logger(log.NewLogfmtLogger(cw), true))
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn := newTestingConnectionForServer()
				Expect(conn).NotTo(BeNil())
				go func() { _ = server.Serve(conn) }()
				conn.ClientSend(`{"type":1,"invocationId": "123","target":"simple"}`)
				<-invocationQueue
				select {
				case <-cw.Chan():
					break
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
				close(done)
			}, 2.0)
		})
		Context("When the Logger option with debug false is used", func() {
			It("calling a method incorrectly should log events", func(done Done) {
				cw := newChannelWriter()
				server, err := NewServer(context.TODO(), UseHub(&invocationHub{}), Logger(log.NewLogfmtLogger(cw), false))
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn := newTestingConnectionForServer()
				Expect(conn).NotTo(BeNil())
				go func() { _ = server.Serve(conn) }()
				conn.ClientSend(`{"type":1,"invocationId": "123","target":"sumple is simple with typo"}`)
				select {
				case <-cw.Chan():
					break
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
				close(done)
			}, 2.0)
		})
		Context("When no option which sets the hub type is used, NewServer", func() {
			It("should return an error", func(done Done) {
				_, err := NewServer(context.TODO(), testLoggerOption())
				Expect(err).NotTo(BeNil())
				close(done)
			})
		})
		Context("When an option returns an error, NewServer", func() {
			It("should return an error", func(done Done) {
				_, err := NewServer(context.TODO(), func(Party) error { return errors.New("bad option") })
				Expect(err).NotTo(BeNil())
				close(done)
			})
		})
	})

	Describe("EnableDetailedErrors option", func() {
		Context("When the EnableDetailedErrors option false is used, calling a method which panics", func() {
			It("should return a completion, which contains only the panic", func(done Done) {
				server, err := NewServer(context.TODO(), UseHub(&invocationHub{}), testLoggerOption())
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn := newTestingConnectionForServer()
				Expect(conn).NotTo(BeNil())
				go func() { _ = server.Serve(conn) }()
				conn.ClientSend(`{"type":1,"invocationId": "ppp","target":"Panic"}`)
				<-invocationQueue
				select {
				case m := <-conn.ReceiveChan():
					Expect(m).To(BeAssignableToTypeOf(completionMessage{}))
					cm := m.(completionMessage)
					Expect(cm.Error).To(Equal("Don't panic!\n"))
				case <-time.After(100 * time.Millisecond):
					Fail("timed out")
				}
				close(done)
			}, 1.0)
		})
		Context("When the EnableDetailedErrors option true is used, calling a method which panics", func() {
			It("should return a completion, which contains only the panic", func(done Done) {
				server, err := NewServer(context.TODO(), UseHub(&invocationHub{}), EnableDetailedErrors(true), testLoggerOption())
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn := newTestingConnectionForServer()
				Expect(conn).NotTo(BeNil())
				go func() { _ = server.Serve(conn) }()
				conn.ClientSend(`{"type":1,"invocationId": "ppp","target":"Panic"}`)
				<-invocationQueue
				select {
				case m := <-conn.ReceiveChan():
					Expect(m).To(BeAssignableToTypeOf(completionMessage{}))
					cm := m.(completionMessage)
					Expect(cm.Error).NotTo(Equal("Don't panic!\n"))
				case <-time.After(100 * time.Millisecond):
					Fail("timed out")
				}
				close(done)
			}, 1.0)
		})
	})

	Describe("TimeoutInterval option", func() {
		Context("When the TimeoutInterval has expired without any client message", func() {
			It("the connection should be closed", func(done Done) {
				server, err := NewServer(context.TODO(), UseHub(&invocationHub{}), TimeoutInterval(100*time.Millisecond), testLoggerOption())
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn := newTestingConnectionForServer()
				Expect(conn).NotTo(BeNil())
				go func() { _ = server.Serve(conn) }()
				time.Sleep(200 * time.Millisecond)
				conn.ClientSend(`{"type":1,"invocationId": "timeout100","target":"Simple"}`)
				select {
				case m := <-conn.ReceiveChan():
					Expect(m).To(BeAssignableToTypeOf(closeMessage{}))
					cm := m.(closeMessage)
					Expect(cm.Error).NotTo(BeNil())
				case <-time.After(200 * time.Millisecond):
					Fail("timed out")
				case <-invocationQueue:
					Fail("hub method invoked")
				}
				close(done)
			}, 2.0)
		})
	})

	Describe("KeepAliveInterval option", func() {
		Context("When the KeepAliveInterval has expired without any server message", func() {
			It("a ping should have been sent", func(done Done) {
				server, err := NewServer(context.TODO(), UseHub(&invocationHub{}), KeepAliveInterval(200*time.Millisecond), testLoggerOption())
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn := newTestingConnection()
				go func() { _ = server.Serve(conn) }()
				// Send initial Handshake
				conn.ClientSend(`{"protocol": "json","version": 1}`)
				// Handshake response
				hr, _ := conn.ClientReceive()
				Expect(hr).To(Equal("{}"))
				for i := 0; i < 5; i++ {
					// Wait for ping
					hmc := make(chan interface{}, 1)
					go func() {
						m, _ := conn.ClientReceive()
						hmc <- m
					}()
					select {
					case m := <-hmc:
						Expect(strings.TrimRight(m.(string), "\n")).To(Equal("{\"type\":6}"))
					case <-time.After(600 * time.Millisecond):
						Fail("timed out")
					}
				}
				server.cancel()
				close(done)
			}, 2.0)
		})
	})

	Describe("StreamBufferCapacity option", func() {
		Context("When the StreamBufferCapacity is 0", func() {
			It("should return an error", func(done Done) {
				_, err := NewServer(context.TODO(), UseHub(&singleHub{}), StreamBufferCapacity(0), testLoggerOption())
				Expect(err).NotTo(BeNil())
				close(done)
			})
		})
	})

	Describe("MaximumReceiveMessageSize option", func() {
		Context("When the MaximumReceiveMessageSize is 0", func() {
			It("should return an error", func(done Done) {
				_, err := NewServer(context.TODO(), UseHub(&singleHub{}), MaximumReceiveMessageSize(0), testLoggerOption())
				Expect(err).NotTo(BeNil())
				close(done)
			})
		})
	})
	Describe("HTTPTransports option", func() {
		Context("When HTTPTransports is one of WebSockets, ServerSentEvents or both", func() {
			It("should set these transports", func(done Done) {
				s, err := NewServer(context.TODO(), UseHub(&singleHub{}), HTTPTransports("WebSockets"), testLoggerOption())
				Expect(err).NotTo(HaveOccurred())
				Expect(s.availableTransports()).To(ContainElement("WebSockets"))
				close(done)
			})
			It("should set these transports", func(done Done) {
				s, err := NewServer(context.TODO(), UseHub(&singleHub{}), HTTPTransports("ServerSentEvents"), testLoggerOption())
				Expect(err).NotTo(HaveOccurred())
				Expect(s.availableTransports()).To(ContainElement("ServerSentEvents"))
				close(done)
			})
			It("should set these transports", func(done Done) {
				s, err := NewServer(context.TODO(), UseHub(&singleHub{}), HTTPTransports("ServerSentEvents", "WebSockets"), testLoggerOption())
				Expect(err).NotTo(HaveOccurred())
				Expect(s.availableTransports()).To(ContainElement("WebSockets"))
				Expect(s.availableTransports()).To(ContainElement("ServerSentEvents"))
				close(done)
			})
		})
		Context("When HTTPTransports is none of WebSockets, ServerSentEvents", func() {
			It("should return an error", func(done Done) {
				_, err := NewServer(context.TODO(), UseHub(&singleHub{}), HTTPTransports("WebTransport"), testLoggerOption())
				Expect(err).To(HaveOccurred())
				close(done)
			})
		})
		Context("When HTTPTransports is used on a client", func() {
			It("should return an error", func(done Done) {
				_, err := NewClient(context.TODO(), WithConnection(newTestingConnection()), HTTPTransports("ServerSentEvents"), testLoggerOption())
				Expect(err).To(HaveOccurred())
				close(done)
			})
		})

	})
})

type channelWriter struct {
	channel chan []byte
}

func (c *channelWriter) Write(p []byte) (n int, err error) {
	c.channel <- p
	return len(p), nil
}

func (c *channelWriter) Chan() chan []byte {
	return c.channel
}

func newChannelWriter() *channelWriter {
	return &channelWriter{make(chan []byte, 100)}
}

//type mapLogger struct {
//	c chan bool
//	m map[string]string
//}
//
//func (m *mapLogger) Log(keyvals ...interface{}) error {
//	m.m = make(map[string]string)
//	for i := 0; i < len(keyvals); i += 2 {
//		m.m[keyvals[i].(string)] = keyvals[i+1].(string)
//	}
//	m.c <- true
//	return nil
//}
