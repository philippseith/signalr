package signalr_test

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/philippseith/signalr"
	"net"
	"testing"
	"time"
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

var _ = Describe("NetConnection", func() {
	Context("Smoke", func() {
		It("should transport a simple invocation over raw rcp", func(done Done) {
			var ctx context.Context
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
					client, err = signalr.NewClient(ctx, signalr.NewNetConnection(ctx, clientConn), testLoggerOption())
					Expect(err).NotTo(HaveOccurred())

					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			err = client.Start()
			Expect(err).NotTo(HaveOccurred())
			result := <-client.Invoke("smoke")
			Expect(result.Value).To(Equal("no smoke!"))
			err = client.Stop()
			Expect(err).NotTo(HaveOccurred())
			cancel()
			close(done)
		})
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
	Context("Timeout", func() {
		It("should be used when reading", func() {
			clientConn, serverConn := net.Pipe()
			// Server loop
			go func() {
				b := make([]byte, 5)
				for {
					if _, err := serverConn.Read(b); err != nil {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
			}()
			conn := signalr.NewNetConnection(context.Background(), clientConn)
			conn.SetTimeout(50 * time.Millisecond)
			Expect(conn.Timeout()).To(Equal(50 * time.Millisecond))
			// Write more than the server could take in one step
			_, err := conn.Write([]byte("foobar-barfoo"))
			Expect(err).To(HaveOccurred())
		})
		It("should be used when reading", func(done Done) {
			clientConn, serverConn := net.Pipe()
			// Server loop
			go func() {
				for {
					if _, err := serverConn.Write([]byte("foo")); err != nil {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
			}()
			conn := signalr.NewNetConnection(context.Background(), clientConn)
			conn.SetTimeout(50 * time.Millisecond)
			// Read some bytes
			b := make([]byte, 50)
			// This will read everything available
			_, _ = conn.Read(b)
			// Because the server is so slow, now an error will occur
			_, err := conn.Read(b)
			Expect(err).To(HaveOccurred())
			close(done)
		})
	})
})
