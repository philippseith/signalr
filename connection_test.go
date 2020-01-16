package signalr

import (
	"context"
	"encoding/json"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("Connection", func() {

	Describe("Connection closed", func() {
		Context("When the connection is closed", func() {
			It("should close the connection and not answer an invocation", func() {
				conn := connect(&Hub{})
				conn.ClientSend(`{"type":7}`)
				conn.ClientSend(`{"type":1,"invocationId": "123","target":"simple"}`)
				// When the connection is closed, the server should either send a closeMessage or nothing at all
				select {
				case message := <-conn.received:
					Expect(message.(closeMessage)).NotTo(BeNil())
				case <-time.After(100 * time.Millisecond):
				}
			})
		})
		Context("When the connection is closed with an invalid close message", func() {
			It("should close the connection and not should not answer an invocation", func() {
				conn := connect(&Hub{})
				conn.ClientSend(`{"type":7,"error":1}`)
				conn.ClientSend(`{"type":1,"invocationId": "123","target":"simple"}`)
				// When the connection is closed, the server should either send a closeMessage or nothing at all
				select {
				case message := <-conn.received:
					Expect(message.(closeMessage)).NotTo(BeNil())
				case <-time.After(100 * time.Millisecond):
				}
			})
		})
	})
})

var _ = Describe("Protocol", func() {

	Describe("Invalid messages", func() {
		Context("When a message with invalid id is sent", func() {
			It("should close the connection with an error", func() {
				conn := connect(&Hub{})
				conn.ClientSend(`{"type":99}`)
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
				case <-time.After(100 * time.Millisecond):
					Fail("timed out")
				}
			})
		})
	})

	Describe("Ping", func() {
		Context("When a ping is received", func() {
			It("should ignore it", func() {
				conn := connect(&Hub{})
				conn.ClientSend(`{"type":6}`)
				select {
				case <-conn.received:
					Fail("ping not ignored")
				case <-time.After(100 * time.Millisecond):
				}
			})
		})
	})
})

var _ = Describe("Handshake", func() {

	Context("When the handshake is sent as partial message to the server", func() {
		It("should be connected", func() {
			server, _ := NewServer(SimpleHubFactory(&invocationHub{}))
			conn := newTestingConnectionBeforeHandshake()
			go server.Run(conn, context.TODO())
			_, _ = conn.cliWriter.Write([]byte(`{"protocol"`))
			conn.ClientSend(`: "json","version": 1}`)
			conn.SetConnected(true)
			conn.ClientSend(`{"type":1,"invocationId": "123","target":"simple"}`)
			Expect(<-invocationQueue).To(Equal("Simple()"))
		})
	})
	Context("When an invalid handshake is sent as partial message to the server", func() {
		It("should not be connected", func() {
			server, _ := NewServer(SimpleHubFactory(&invocationHub{}))
			conn := newTestingConnectionBeforeHandshake()
			go server.Run(conn, context.TODO())
			_, _ = conn.cliWriter.Write([]byte(`{"protocol"`))
			// Opening curly brace is invalid
			conn.ClientSend(`{: "json","version": 1}`)
			conn.SetConnected(true)
			conn.ClientSend(`{"type":1,"invocationId": "123","target":"simple"}`)
			select {
			case <-invocationQueue:
				Fail("server connected with invalid handshake")
			case <-time.After(100 * time.Millisecond):
			}
		})
	})
	Context("When a handshake is sent with an unsupported protocol", func() {
		It("should return an error handshake response and be not connected", func() {
			server, _ := NewServer(SimpleHubFactory(&invocationHub{}))
			conn := newTestingConnectionBeforeHandshake()
			go server.Run(conn, context.TODO())
			conn.ClientSend(`{"protocol": "bson","version": 1}`)
			response, err := conn.ClientReceive()
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
			jsonMap := make(map[string]interface{})
			err = json.Unmarshal([]byte(response), &jsonMap)
			Expect(err).To(BeNil())
			Expect(jsonMap["error"]).NotTo(BeNil())
			conn.ClientSend(`{"type":1,"invocationId": "123","target":"simple"}`)
			select {
			case <-invocationQueue:
				Fail("server connected with invalid handshake")
			case <-time.After(100 * time.Millisecond):
			}
		})
	})
})
