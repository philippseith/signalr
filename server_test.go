package signalr

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Server.HubClients", func() {
	Context("All().Send()", func() {
		j := 1
		It(fmt.Sprintf("should send clients %v", j), func(done Done) {
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
			// Give the server some time. In contrast to the client, we have not connected state to query
			<-time.After(100 * time.Millisecond)
			// Create the Client
			receiver := &simpleReceiver{ch: make(chan string, 1)}
			ctx, cancelClient := context.WithCancel(context.Background())
			client, _ := NewClient(ctx,
				WithConnection(cliConn),
				WithReceiver(receiver),
				testLoggerOption(),
				TransferFormat("Text"))
			Expect(client).NotTo(BeNil())
			// Start it
			client.Start()
			// Wait for client running
			Expect(<-client.WaitForState(context.Background(), ClientConnected)).NotTo(HaveOccurred())
			// Send from the server to "all" clients
			<-time.After(100 * time.Millisecond)
			server.HubClients().All().Send("OnCallback", fmt.Sprintf("All%v", j))
			// Did the receiver get what we did send?
			Expect(<-receiver.ch).To(Equal(fmt.Sprintf("All%v", j)))
			cancelClient()
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
