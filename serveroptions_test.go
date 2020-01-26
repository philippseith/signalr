package signalr

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"time"
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
			It("should use the same hub instance on all invocations", func() {
				server, err := NewServer(UseHub(&singleHub{}))
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn1 := newTestingConnection()
				Expect(conn1).NotTo(BeNil())
				go server.Run(context.TODO(), conn1)
				<-singleHubMsg
				conn2 := newTestingConnection()
				Expect(conn2).NotTo(BeNil())
				go server.Run(context.TODO(), conn2)
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
			})
		})
	})

	Describe("SimpleHubFactory option", func() {
		Context("When the SimpleHubFactory option is used", func() {
			It("should call the hub factory on each hub method invocation", func() {
				server, err := NewServer(SimpleHubFactory(&singleHub{}))
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn := newTestingConnection()
				Expect(conn).NotTo(BeNil())
				go server.Run(context.TODO(), conn)
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
			})
		})
	})

	Describe("Logger option", func() {
		Context("When the Logger option with debug false is used", func() {
			It("calling a method correctly should log no events", func() {
				cw := newChannelWriter()
				server, err := NewServer(UseHub(&invocationHub{}), Logger(log.NewLogfmtLogger(cw), false))
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn := newTestingConnection()
				Expect(conn).NotTo(BeNil())
				go server.Run(context.TODO(), conn)
				conn.ClientSend(`{"type":1,"invocationId": "123","target":"simple"}`)
				<-invocationQueue
				select {
				case logEntry := <-cw.Chan():
					Fail(fmt.Sprintf("log entry written: %v", string(logEntry)))
				case <-time.After(1000 * time.Millisecond):
					break
				}
			})
		})
		Context("When the Logger option with debug true is used", func() {
			It("calling a method correctly should log events", func() {
				cw := newChannelWriter()
				server, err := NewServer(UseHub(&invocationHub{}), Logger(log.NewLogfmtLogger(cw), true))
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn := newTestingConnection()
				Expect(conn).NotTo(BeNil())
				go server.Run(context.TODO(), conn)
				conn.ClientSend(`{"type":1,"invocationId": "123","target":"simple"}`)
				<-invocationQueue
				select {
				case <-cw.Chan():
					break
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
			})
		})
		Context("When the Logger option with debug false is used", func() {
			It("calling a method incorrectly should log events", func() {
				cw := newChannelWriter()
				server, err := NewServer(UseHub(&invocationHub{}), Logger(log.NewLogfmtLogger(cw), false))
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn := newTestingConnection()
				Expect(conn).NotTo(BeNil())
				go server.Run(context.TODO(), conn)
				conn.ClientSend(`{"type":1,"invocationId": "123","target":"sumple is simple with typo"}`)
				select {
				case <-cw.Chan():
					break
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
			})
		})
		Context("When no option which sets the hub type is used, NewServer", func() {
			It("should return an error", func() {
				_, err := NewServer()
				Expect(err).NotTo(BeNil())
			})
		})
		Context("When an option returns an error, NewServer", func() {
			It("should return an error", func() {
				_, err := NewServer(func(*Server) error { return errors.New("bad option") })
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("EnableDetailedErrors option", func() {
		Context("When the EnableDetailedErrors option false is used, calling a method which panics", func() {
			It("should return a completion, which contains only the panic", func() {
				server, err := NewServer(UseHub(&invocationHub{}))
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn := newTestingConnection()
				Expect(conn).NotTo(BeNil())
				go server.Run(context.TODO(), conn)
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
			})
		})
		Context("When the EnableDetailedErrors option true is used, calling a method which panics", func() {
			It("should return a completion, which contains only the panic", func() {
				server, err := NewServer(UseHub(&invocationHub{}), EnableDetailedErrors(true))
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn := newTestingConnection()
				Expect(conn).NotTo(BeNil())
				go server.Run(context.TODO(), conn)
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
			})
		})
	})

	Describe("ClientTimeoutInterval option", func() {
		Context("When the ClientTimeoutInterval has expired without any client message", func() {
			It("the connection should be closed", func() {
				server, err := NewServer(UseHub(&invocationHub{}), ClientTimeoutInterval(100*time.Millisecond))
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn := newTestingConnection()
				Expect(conn).NotTo(BeNil())
				go server.Run(context.TODO(), conn)
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

			})
		})
	})

	Describe("KeepAliveInterval option", func() {
		Context("When the KeepAliveInterval has expired without any server message", func() {
			It("a ping should have been sent", func() {
				server, err := NewServer(UseHub(&invocationHub{}), KeepAliveInterval(200*time.Millisecond))
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn := newTestingConnectionBeforeHandshake()
				go server.Run(context.TODO(), conn)
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
						Expect(m).To(Equal("{\"type\":6}\n"))
					case <-time.After(300 * time.Millisecond):
						Fail("timed out")
					}
				}
			})
		})
	})

	Describe("StreamBufferCapacity option", func() {
		Context("When the StreamBufferCapacity is 0", func() {
			It("should return an error", func() {
				_, err := NewServer(UseHub(&singleHub{}), StreamBufferCapacity(0))
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("MaximumReceiveMessageSize option", func() {
		Context("When the MaximumReceiveMessageSize is 0", func() {
			It("should return an error", func() {
				_, err := NewServer(UseHub(&singleHub{}), MaximumReceiveMessageSize(0))
				Expect(err).NotTo(BeNil())
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
