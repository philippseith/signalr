package signalr

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Connection", func() {

	Describe("Connection closed", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&Hub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("When the connection is closed", func() {
			It("should close the connection and not answer an invocation", func(done Done) {
				conn.ClientSend(`{"type":7}`)
				conn.ClientSend(`{"type":1,"invocationId": "123","target":"unknownFunc"}`)
				// When the connection is closed, the server should either send a closeMessage or nothing at all
				select {
				case message := <-conn.received:
					Expect(message.(closeMessage)).NotTo(BeNil())
				case <-time.After(100 * time.Millisecond):
				}
				close(done)
			})
		})
		Context("When the connection is closed with an invalid close message", func() {
			It("should close the connection and should not answer an invocation", func(done Done) {
				conn.ClientSend(`{"type":7,"error":1}`)
				conn.ClientSend(`{"type":1,"invocationId": "123","target":"unknownFunc"}`)
				// When the connection is closed, the server should either send a closeMessage or nothing at all
				select {
				case message := <-conn.received:
					Expect(message.(closeMessage)).NotTo(BeNil())
				case <-time.After(100 * time.Millisecond):
				}
				close(done)
			})
		})
	})
})

var _ = Describe("Protocol", func() {
	var server Server
	var conn *testingConnection
	BeforeEach(func(done Done) {
		server, conn = connect(&Hub{})
		close(done)
	})
	AfterEach(func(done Done) {
		server.cancel()
		close(done)
	})
	Describe("Invalid messages", func() {
		Context("When a message with invalid id is sent", func() {
			It("should close the connection with an error", func(done Done) {
				conn.ClientSend(`{"type":99}`)
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
				case <-time.After(100 * time.Millisecond):
					Fail("timed out")
				}
				close(done)
			})
		})
	})

	Describe("Ping", func() {
		Context("When a ping is received", func() {
			It("should ignore it", func(done Done) {
				conn.ClientSend(`{"type":6}`)
				select {
				case <-conn.received:
					Fail("ping not ignored")
				case <-time.After(100 * time.Millisecond):
				}
				close(done)
			})
		})
	})
})

type handshakeHub struct {
	Hub
}

func (h *handshakeHub) Shake() {
	shakeQueue <- "Shake()"
}

var shakeQueue = make(chan string, 10)

func getTestBedHandshake() (*testingConnection, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	server, _ := NewServer(ctx, SimpleHubFactory(&handshakeHub{}), testLoggerOption())
	conn := newTestingConnection()
	go func() { _ = server.Serve(conn) }()
	return conn, cancel
}

var _ = Describe("Handshake", func() {
	Context("When the handshake is sent as one message to the server", func() {
		It("should be connected", func(done Done) {
			conn, cancel := getTestBedHandshake()
			conn.ClientSend(`{"protocol": "json","version": 1}`)
			conn.ClientSend(`{"type":1,"invocationId": "123A","target":"shake"}`)
			Expect(<-shakeQueue).To(Equal("Shake()"))
			cancel()
			close(done)
		})
	})
	Context("When the handshake is sent as partial message to the server", func() {
		It("should be connected", func(done Done) {
			conn, cancel := getTestBedHandshake()
			_, _ = conn.cliWriter.Write([]byte(`{"protocol"`))
			conn.ClientSend(`: "json","version": 1}`)
			conn.ClientSend(`{"type":1,"invocationId": "123B","target":"shake"}`)
			Expect(<-shakeQueue).To(Equal("Shake()"))
			cancel()
			close(done)
		})
	})
	Context("When an invalid handshake is sent as partial message to the server", func() {
		It("should not be connected", func(done Done) {
			conn, cancel := getTestBedHandshake()
			_, _ = conn.cliWriter.Write([]byte(`{"protocol"`))
			// Opening curly brace is invalid
			conn.ClientSend(`{: "json","version": 1}`)
			conn.ClientSend(`{"type":1,"invocationId": "123C","target":"shake"}`)
			select {
			case <-shakeQueue:
				Fail("server connected with invalid handshake")
			case <-time.After(100 * time.Millisecond):
			}
			cancel()
			close(done)
		})
	})
	Context("When a handshake is sent with an unsupported protocol", func() {
		It("should return an error handshake response and be not connected", func(done Done) {
			conn, cancel := getTestBedHandshake()
			conn.ClientSend(`{"protocol": "bson","version": 1}`)
			response, err := conn.ClientReceive()
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			jsonMap := make(map[string]interface{})
			err = json.Unmarshal([]byte(response), &jsonMap)
			Expect(err).To(BeNil())
			Expect(jsonMap["error"]).NotTo(BeNil())
			conn.ClientSend(`{"type":1,"invocationId": "123D","target":"shake"}`)
			select {
			case <-shakeQueue:
				Fail("server connected with invalid handshake")
			case <-time.After(100 * time.Millisecond):
			}
			cancel()
			close(done)
		})
	})
	Context("When the connection fails before the server can receive handshake request", func() {
		It("should not be connected", func(done Done) {
			conn, cancel := getTestBedHandshake()
			conn.SetFailRead("failed read in handshake")
			conn.ClientSend(`{"protocol": "json","version": 1}`)
			conn.ClientSend(`{"type":1,"invocationId": "123E","target":"shake"}`)
			select {
			case <-shakeQueue:
				Fail("server connected with fail before handshake")
			case <-time.After(100 * time.Millisecond):
			}
			cancel()
			close(done)
		})
	})
	Context("When the handshake is received by the server but the connection fails when the response should be sent ", func() {
		It("should not be connected", func(done Done) {
			conn, cancel := getTestBedHandshake()
			conn.SetFailWrite("failed write in handshake")
			conn.ClientSend(`{"protocol": "json","version": 1}`)
			conn.ClientSend(`{"type":1,"invocationId": "123F","target":"shake"}`)
			select {
			case <-shakeQueue:
				Fail("server connected with fail before handshake")
			case <-time.After(100 * time.Millisecond):
			}
			cancel()
			close(done)
		})
	})
	Context("When the handshake with an unsupported protocol is received by the server but the connection fails when the response should be sent ", func() {
		It("should not be connected", func(done Done) {
			conn, cancel := getTestBedHandshake()
			conn.SetFailWrite("failed write in handshake")
			conn.ClientSend(`{"protocol": "bson","version": 1}`)
			conn.ClientSend(`{"type":1,"invocationId": "123G","target":"shake"}`)
			select {
			case <-shakeQueue:
				Fail("server connected with fail before handshake")
			case <-time.After(100 * time.Millisecond):
			}
			cancel()
			close(done)
		})
	})
	Context("When the handshake connection is initiated, but the client does not send a handshake request within the handshake timeout ", func() {
		It("should not be connected", func(done Done) {
			server, _ := NewServer(context.TODO(), SimpleHubFactory(&handshakeHub{}), HandshakeTimeout(time.Millisecond*100), testLoggerOption())
			conn := newTestingConnection()
			go func() { _ = server.Serve(conn) }()
			time.Sleep(time.Millisecond * 200)
			conn.ClientSend(`{"protocol": "json","version": 1}`)
			conn.ClientSend(`{"type":1,"invocationId": "123H","target":"shake"}`)
			select {
			case <-shakeQueue:
				Fail("server connected with fail before handshake")
			case <-time.After(100 * time.Millisecond):
			}
			server.cancel()
			close(done)
		}, 2.0)
	})
})
