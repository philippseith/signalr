package signalr

import (
	"context"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("Server.HubClients", func() {
	Context("All().Send()", func() {
		It("should send clients", func(done Done) {
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
			go server.Serve(srvConn)
			// Create the Client
			receiver := &simpleReceiver{}
			clientConn, _ := NewClient(context.TODO(), cliConn,
				Receiver(receiver),
				testLoggerOption(),
				TransferFormat("Text"))
			Expect(clientConn).NotTo(BeNil())
			// Start it
			_ = clientConn.Start()
			// Send from the server to "all" clients
			server.HubClients().All().Send("OnCallback", "All")
			ch := make(chan string, 1)
			go func() {
				for {
					if result, ok := receiver.result.Load().(string); ok {
						ch <- result
						close(ch)
						break
					}
				}
			}()
			// Did the receiver get what we did send?
			Expect(<-ch).To(Equal("All"))
			_ = clientConn.Stop()
			server.cancel()
			close(done)
		}, 1.0)
	})
	Context("Caller()", func() {
		It("should return nil", func() {
			server, _ := NewServer(context.TODO(), SimpleHubFactory(&simpleHub{}),
				testLoggerOption(),
				ChanReceiveTimeout(200*time.Millisecond),
				StreamBufferCapacity(5))
			Expect(server.HubClients().Caller()).To(BeNil())
		})
	})
})
