package signalr

import (
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
				go server.Run(conn1)
				<-singleHubMsg
				conn2 := newTestingConnection()
				Expect(conn2).NotTo(BeNil())
				go server.Run(conn2)
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
			It("should call the hubfactory on each hub method invocation", func() {
				server, err := NewServer(SimpleHubFactory(&singleHub{}))
				Expect(server).NotTo(BeNil())
				Expect(err).To(BeNil())
				conn := newTestingConnection()
				Expect(conn).NotTo(BeNil())
				go server.Run(conn)
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
				go server.Run(conn)
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
				go server.Run(conn)
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
				go server.Run(conn)
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
