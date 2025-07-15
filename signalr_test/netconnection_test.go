package signalr_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/mojtabaRKS/signalr"
)

func TestSignalR(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SignalR external Suite")
}

type NetHub struct {
	signalr.Hub
}

func (n *NetHub) Smoke() string {
	return "no smoke!"
}

func (n *NetHub) ContinuousSmoke() chan string {
	ch := make(chan string, 1)
	go func() {
	loop:
		for i := 0; i < 5; i++ {
			select {
			case ch <- "smoke...":
			case <-n.Context().Done():
				break loop
			}
			<-time.After(100 * time.Millisecond)
		}
		close(ch)
	}()
	return ch
}

var _ = Describe("NetConnection", func() {
	Context("Smoke", func() {
		It("should transport a simple invocation over raw rcp", func(done Done) {
			ctx, cancel := context.WithCancel(context.Background())
			server, err := signalr.NewServer(ctx, signalr.SimpleHubFactory(&NetHub{}), testLoggerOption())
			Expect(err).NotTo(HaveOccurred())
			addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
			Expect(err).NotTo(HaveOccurred())
			listener, err := net.ListenTCP("tcp", addr)
			Expect(err).NotTo(HaveOccurred())
			go func() {
				for {
					tcpConn, err := listener.Accept()
					Expect(err).NotTo(HaveOccurred())
					go func() { _ = server.Serve(signalr.NewNetConnection(ctx, tcpConn)) }()
					break
				}
			}()
			var client signalr.Client
			for {
				if clientConn, err := net.Dial("tcp",
					fmt.Sprintf("localhost:%v", listener.Addr().(*net.TCPAddr).Port)); err == nil {
					client, err = signalr.NewClient(ctx, signalr.WithConnection(signalr.NewNetConnection(ctx, clientConn)), testLoggerOption())
					Expect(err).NotTo(HaveOccurred())
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			client.Start()
			result := <-client.Invoke("smoke")
			Expect(result.Value).To(Equal("no smoke!"))
			cancel()
			close(done)
		})
	})
	Context("Stream and Timeout", func() {
		It("Client and Server should timeout when no messages are exchanged, but message exchange should prevent timeout", func(done Done) {
			ctx, cancel := context.WithCancel(context.Background())
			server, err := signalr.NewServer(ctx, signalr.SimpleHubFactory(&NetHub{}), testLoggerOption(),
				// Set KeepAlive and Timeout so KeepAlive can't keep it alive
				signalr.TimeoutInterval(500*time.Millisecond), signalr.KeepAliveInterval(2*time.Second))
			Expect(err).NotTo(HaveOccurred())
			addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
			Expect(err).NotTo(HaveOccurred())
			listener, err := net.ListenTCP("tcp", addr)
			Expect(err).NotTo(HaveOccurred())
			serverDone := make(chan struct{}, 1)
			go func() {
				tcpConn, err := listener.Accept()
				Expect(err).NotTo(HaveOccurred())
				go func() {
					_ = server.Serve(signalr.NewNetConnection(ctx, tcpConn))
					serverDone <- struct{}{}
				}()
			}()
			var client signalr.Client
			for {
				if clientConn, err := net.Dial("tcp",
					fmt.Sprintf("localhost:%v", listener.Addr().(*net.TCPAddr).Port)); err == nil {
					client, err = signalr.NewClient(ctx, signalr.WithConnection(signalr.NewNetConnection(ctx, clientConn)), testLoggerOption(),
						// Set KeepAlive and Timeout so KeepAlive can't keep it alive
						signalr.TimeoutInterval(500*time.Millisecond), signalr.KeepAliveInterval(2*time.Second))
					Expect(err).NotTo(HaveOccurred())
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			client.Start()
			// The Server will send values each 100ms, so this should keep the connection alive
			i := 0
			for range client.PullStream("continuoussmoke") {
				i++
			}
			// some smoke messages and one error message when timed out
			Expect(i).To(BeNumerically(">", 1))
			// Wait for client and server to timeout
			<-time.After(time.Second)
			select {
			case <-serverDone:
				Expect(client.State() == signalr.ClientClosed)
			case <-time.After(10 * time.Millisecond):
				Fail("server not closed")
			}
			cancel()
			close(done)
		}, 5.0)
	})
	Context("SetConnectionID", func() {
		It("should change the ConnectionID", func() {
			clientConn, _ := net.Pipe()
			conn := signalr.NewNetConnection(context.Background(), clientConn)
			id := conn.ConnectionID()
			conn.SetConnectionID("Other" + id)
			Expect(conn.ConnectionID()).To(Equal("Other" + id))
		})
	})
	Context("Cancel", func() {
		It("should cancel the connection", func(done Done) {
			clientConn, serverConn := net.Pipe()
			ctx, cancel := context.WithCancel(context.Background())
			conn := signalr.NewNetConnection(ctx, clientConn)
			// Server loop
			go func() {
				b := make([]byte, 1024)
				for {
					if _, err := serverConn.Read(b); err != nil {
						break
					}
				}
			}()
			go func() {
				time.Sleep(500 * time.Millisecond)
				// cancel the connection
				cancel()
			}()
			for {
				if _, err := conn.Write([]byte("foobar")); err != nil {
					// This will never happen if the connection is not canceled
					break
				}
			}
			close(done)
		}, 2.0)
	})
})
