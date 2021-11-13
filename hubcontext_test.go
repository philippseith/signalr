package signalr

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type contextHub struct {
	Hub
}

func (c *contextHub) OnConnected(string) {
}

func (c *contextHub) CallAll() {
	c.Clients().All().Send("clientFunc")
}

func (c *contextHub) CallCaller() {
	c.Clients().Caller().Send("clientFunc")
}

func (c *contextHub) CallClient(connectionID string) {
	c.Clients().Client(connectionID).Send("clientFunc")
}

func (c *contextHub) BuildGroup(connectionID1 string, connectionID2 string) {
	c.Groups().AddToGroup("local", connectionID1)
	c.Groups().AddToGroup("local", connectionID2)
}

func (c *contextHub) RemoveFromGroup(connectionID string) {
	c.Groups().RemoveFromGroup("local", connectionID)
}

func (c *contextHub) CallGroup() {
	c.Clients().Group("local").Send("clientFunc")
}

func (c *contextHub) AddItem(key string, value interface{}) {
	c.Items().Store(key, value)
}

func (c *contextHub) GetItem(key string) interface{} {
	if item, ok := c.Items().Load(key); ok {
		return item
	}
	return nil
}

func (c *contextHub) TestConnectionID() {
}

func (c *contextHub) Abort() {
	c.Hub.Abort()
}

func connectMany() (Server, []*testingConnection, []string) {
	s, err := NewServer(context.TODO(), SimpleHubFactory(&contextHub{}),
		testLoggerOption())
	if err != nil {
		Fail(err.Error())
		return nil, nil, nil
	}
	conns := make([]*testingConnection, 3)
	connIds := make([]string, 0)
	for i := 0; i < 3; i++ {
		conns[i] = newTestingConnectionForServer()
	}
	var wg sync.WaitGroup
	wg.Add(3)
	for i := 0; i < 3; i++ {
		go func(i int) {
			wg.Done()
			_ = s.Serve(conns[i])
		}(i)
	}
	wg.Wait()
	return s, conns, connIds
}

func expectInvocationMessageFromConnection(ctx context.Context, conn *testingConnection, i int, errCh chan error) {
	var msg interface{}
	select {
	case msg = <-conn.received:
		if _, ok := msg.(completionMessage); ok {
			fmt.Printf("Skipped completion %#v\n", msg)
			select {
			case msg = <-conn.received:
			case <-time.After(time.Second):
				errCh <- fmt.Errorf("timeout client %v waiting for message", i)
				return
			case <-ctx.Done():
				return
			}
		}
	case <-time.After(time.Second):
		errCh <- fmt.Errorf("timeout client %v waiting for message", i)
		return
	case <-ctx.Done():
		return
	}
	if ivMsg, ok := msg.(invocationMessage); !ok {
		errCh <- fmt.Errorf("client %v expected invocationMessage, got %T %#v", i, msg, msg)
		return
	} else {
		if strings.ToLower(ivMsg.Target) != "clientfunc" {
			errCh <- fmt.Errorf("client %v expected clientfunc, got got %#v", i, ivMsg)
			return
		}
	}
	errCh <- nil
}

func expectNoMessageFromConnection(ctx context.Context, conn *testingConnection, errCh chan error) {
	select {
	case msg := <-conn.received:
		errCh <- fmt.Errorf("received unexpected message %T %#v", msg, msg)
	case <-ctx.Done():
	}
}

type SimpleReceiver struct {
	ch chan struct{}
}

func (sr *SimpleReceiver) ClientFunc() {
	close(sr.ch)
}

func makeServerAndClients(ctx context.Context, clientCount int) (Server, []Client, []*SimpleReceiver, []Connection, []Connection, error) {
	server, err := NewServer(ctx, SimpleHubFactory(&contextHub{}), testLoggerOption())
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	cliConn := make([]Connection, clientCount)
	srvConn := make([]Connection, clientCount)
	receiver := make([]*SimpleReceiver, clientCount)
	client := make([]Client, clientCount)
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	for i := 0; i < clientCount; i++ {
		listener, err := net.ListenTCP("tcp", addr)
		go func(i int) {
			for {
				tcpConn, _ := listener.Accept()
				conn := NewNetConnection(ctx, tcpConn)
				conn.SetConnectionID(fmt.Sprint(i))
				srvConn[i] = conn
				go func() { _ = server.Serve(conn) }()
				break
			}
		}(i)
		tcpConn, err := net.Dial("tcp",
			fmt.Sprintf("localhost:%v", listener.Addr().(*net.TCPAddr).Port))
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		cliConn[i] = NewNetConnection(ctx, tcpConn)
		receiver[i] = &SimpleReceiver{ch: make(chan struct{})}
		client[i], err = NewClient(ctx, WithConnection(cliConn[i]), WithReceiver(receiver[i]), TransferFormat("Text"), testLoggerOption())
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		client[i].Start()
		select {
		case err := <-client[i].WaitForState(ctx, ClientConnected):
			if err != nil {
				return nil, nil, nil, nil, nil, err
			}
		case <-ctx.Done():
			return nil, nil, nil, nil, nil, ctx.Err()
		}
	}
	return server, client, receiver, srvConn, cliConn, nil
}

var _ = Describe("HubContext", func() {
	Context("Clients().All()", func() {
		It("should invoke all clients", func() {
			server, conns, _ := connectMany()
			defer server.cancel()
			conns[0].ClientSend(`{"type":1,"invocationId": "123","target":"callall"}`)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			errCh1 := make(chan error, 1)
			go expectInvocationMessageFromConnection(ctx, conns[0], 1, errCh1)
			errCh2 := make(chan error, 1)
			go expectInvocationMessageFromConnection(ctx, conns[1], 2, errCh2)
			errCh3 := make(chan error, 1)
			go expectInvocationMessageFromConnection(ctx, conns[2], 3, errCh3)
			done := make(chan error, 1)
			go func(ctx context.Context, done, errCh1, errCh2, errCh3 chan error) {
				results := 0
				for results < 3 {
					select {
					case err := <-errCh1:
						if err != nil {
							done <- err
						}
						results++
					case err := <-errCh2:
						if err != nil {
							done <- err
						}
						results++
					case err := <-errCh3:
						if err != nil {
							done <- err
						}
						results++
					case <-ctx.Done():
						done <- ctx.Err()
						return
					}
				}
				done <- nil
			}(ctx, done, errCh1, errCh2, errCh3)
			select {
			case err := <-done:
				Expect(err).NotTo(HaveOccurred())
			case <-time.After(2 * time.Second):
				Fail("timeout waiting for clients getting results")
			}
		})
	})
	Context("Clients().Caller()", func() {
		It("should invoke only the caller", func() {
			server, conns, _ := connectMany()
			defer server.cancel()
			conns[0].ClientSend(`{"type":1,"invocationId": "123","target":"callcaller"}`)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			errCh1 := make(chan error, 1)
			go expectInvocationMessageFromConnection(ctx, conns[0], 1, errCh1)
			errCh2 := make(chan error, 1)
			go expectNoMessageFromConnection(ctx, conns[1], errCh2)
			errCh3 := make(chan error, 1)
			go expectNoMessageFromConnection(ctx, conns[2], errCh3)
			done := make(chan error, 1)
			go func(ctx context.Context, done, errCh1, errCh2, errCh3 chan error) {
				for {
					select {
					case err := <-errCh1:
						if err != nil {
							done <- err
							return
						}
					case err := <-errCh2:
						done <- err
						return
					case err := <-errCh3:
						done <- err
						return
					case <-time.After(100 * time.Millisecond):
						done <- nil
						return
					case <-ctx.Done():
						done <- ctx.Err()
						return
					}
				}
			}(ctx, done, errCh1, errCh2, errCh3)
			select {
			case err := <-done:
				Expect(err).NotTo(HaveOccurred())
			case <-time.After(1 * time.Second):
				Fail("timeout waiting for client getting result")
			}
		})
	})
	Context("Clients().Client()", func() {
		It("should invoke only the client which was addressed", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, client, receiver, srvConn, _, err := makeServerAndClients(ctx, 3)
			Expect(err).NotTo(HaveOccurred())

			select {
			case ir := <-client[0].Invoke("CallClient", srvConn[2].ConnectionID()):
				Expect(ir.Error).NotTo(HaveOccurred())
			case <-time.After(100 * time.Millisecond):
				Fail("timeout in invoke")
			}
			gotCalled := false
			select {
			case <-receiver[0].ch:
				Fail("client 1 received message for client 3")
			case <-receiver[1].ch:
				Fail("client 2 received message for client 3")
			case <-receiver[2].ch:
				gotCalled = true
			case <-time.After(100 * time.Millisecond):
				if !gotCalled {
					Fail("timeout without client 3 got called")
				}
			}
		})
	})
})

func TestGroupShouldInvokeOnlyTheClientsInTheGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, client, receiver, srvConn, _, err := makeServerAndClients(ctx, 3)
	assert.NoError(t, err)
	select {
	case ir := <-client[0].Invoke("buildgroup", srvConn[1].ConnectionID(), srvConn[2].ConnectionID()):
		assert.NoError(t, ir.Error)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "timeout in invoke")
	}
	select {
	case ir := <-client[0].Invoke("callgroup"):
		assert.NoError(t, ir.Error)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "timeout in invoke")
	}
	gotCalled := 0
	select {
	case <-receiver[0].ch:
		assert.Fail(t, "client 1 received message for client 2, 3")
	case <-receiver[1].ch:
		gotCalled++
	case <-receiver[2].ch:
		gotCalled++
	case <-time.After(100 * time.Millisecond):
		if gotCalled < 2 {
			assert.Fail(t, "timeout without client 2 and 3 got called")
		}
	}
}

func TestRemoveClientsShouldRemoveClientsFromTheGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, client, receiver, srvConn, _, err := makeServerAndClients(ctx, 3)
	assert.NoError(t, err)
	select {
	case ir := <-client[0].Invoke("buildgroup", srvConn[1].ConnectionID(), srvConn[2].ConnectionID()):
		assert.NoError(t, ir.Error)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "timeout in invoke")
	}
	select {
	case ir := <-client[0].Invoke("removefromgroup", srvConn[2].ConnectionID()):
		assert.NoError(t, ir.Error)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "timeout in invoke")
	}
	select {
	case ir := <-client[0].Invoke("callgroup"):
		assert.NoError(t, ir.Error)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "timeout in invoke")
	}
	gotCalled := false
	select {
	case <-receiver[0].ch:
		assert.Fail(t, "client 1 received message for client 2")
	case <-receiver[1].ch:
		gotCalled = true
	case <-receiver[2].ch:
		assert.Fail(t, "client 3 received message for client 2")
	case <-time.After(100 * time.Millisecond):
		if !gotCalled {
			assert.Fail(t, "timeout without client 3 got called")
		}
	}
}

func TestItemsShouldHoldItemsConnectionWise(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, client, _, _, _, err := makeServerAndClients(ctx, 2)
	assert.NoError(t, err)
	select {
	case ir := <-client[0].Invoke("additem", "first", 1):
		assert.NoError(t, ir.Error)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "timeout in invoke")
	}
	select {
	case ir := <-client[0].Invoke("getitem", "first"):
		assert.NoError(t, ir.Error)
		assert.Equal(t, ir.Value, 1.0)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "timeout in invoke")
	}
	select {
	case ir := <-client[1].Invoke("getitem", "first"):
		assert.NoError(t, ir.Error)
		assert.Equal(t, ir.Value, nil)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "timeout in invoke")
	}
}

func TestAbortShouldAbortTheConnectionOfTheCurrentCaller(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, client, _, _, _, err := makeServerAndClients(ctx, 2)
	assert.NoError(t, err)
	select {
	case ir := <-client[0].Invoke("abort"):
		assert.Error(t, ir.Error)
		select {
		case err := <-client[0].WaitForState(ctx, ClientClosed):
			assert.NoError(t, err)
		case <-time.After(500 * time.Millisecond):
			assert.Fail(t, "timeout waiting for client close")
		}
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "timeout in invoke")
	}
	select {
	case ir := <-client[1].Invoke("additem", "first", 2):
		assert.NoError(t, ir.Error)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "timeout in invoke")
	}
	select {
	case ir := <-client[1].Invoke("getitem", "first"):
		assert.NoError(t, ir.Error)
		assert.Equal(t, ir.Value, 2.0)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "timeout in invoke")
	}
}
