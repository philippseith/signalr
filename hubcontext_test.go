package signalr

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	. "github.com/onsi/ginkgo/v2"
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

type SimpleReceiver struct {
	ch chan struct{}
}

func (sr *SimpleReceiver) ClientFunc() {
	close(sr.ch)
}

func makeTCPServerAndClients(ctx context.Context, clientCount int) (Server, []Client, []*SimpleReceiver, []Connection, []Connection, error) {
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

func makePipeClientsAndReceivers() ([]Client, []*SimpleReceiver, context.CancelFunc) {
	cliConn := make([]*pipeConnection, 3)
	srvConn := make([]*pipeConnection, 3)
	for i := 0; i < 3; i++ {
		cliConn[i], srvConn[i] = newClientServerConnections()
		cliConn[i].SetConnectionID(fmt.Sprint(i))
		srvConn[i].SetConnectionID(fmt.Sprint(i))
	}
	ctx, cancel := context.WithCancel(context.Background())
	server, _ := NewServer(ctx, SimpleHubFactory(&contextHub{}), testLoggerOption())
	var wg sync.WaitGroup
	wg.Add(3)
	for i := 0; i < 3; i++ {
		go func(i int) {
			wg.Done()
			_ = server.Serve(srvConn[i])
		}(i)
	}
	wg.Wait()
	client := make([]Client, 3)
	receiver := make([]*SimpleReceiver, 3)
	for i := 0; i < 3; i++ {
		receiver[i] = &SimpleReceiver{ch: make(chan struct{})}
		client[i], _ = NewClient(ctx, WithConnection(cliConn[i]), WithReceiver(receiver[i]), testLoggerOption())
		client[i].Start()
		<-client[i].WaitForState(ctx, ClientConnected)
	}
	return client, receiver, cancel
}

var _ = Describe("HubContext", func() {
	for i := 0; i < 10; i++ {
		Context("Clients().All()", func() {
			It("should invoke all clients", func() {
				client, receiver, cancel := makePipeClientsAndReceivers()
				r := <-client[0].Invoke("CallAll")
				Expect(r.Error).NotTo(HaveOccurred())
				result := 0
				for result < 3 {
					select {
					case <-receiver[0].ch:
						result++
					case <-receiver[1].ch:
						result++
					case <-receiver[2].ch:
						result++
					case <-time.After(2 * time.Second):
						Fail("timeout waiting for clients getting results")
					}
				}
				cancel()
			})
		})
		Context("Clients().Caller()", func() {
			It("should invoke only the caller", func() {
				client, receiver, cancel := makePipeClientsAndReceivers()
				r := <-client[0].Invoke("CallCaller")
				Expect(r.Error).NotTo(HaveOccurred())
				select {
				case <-receiver[0].ch:
				case <-receiver[1].ch:
					Fail("Wrong client received message")
				case <-receiver[2].ch:
					Fail("Wrong client received message")
				case <-time.After(2 * time.Second):
					Fail("timeout waiting for clients getting results")
				}
				cancel()
			})
		})
		Context("Clients().Client()", func() {
			It("should invoke only the client which was addressed", func() {
				client, receiver, cancel := makePipeClientsAndReceivers()
				r := <-client[0].Invoke("CallClient", "1")
				Expect(r.Error).NotTo(HaveOccurred())
				select {
				case <-receiver[0].ch:
					Fail("Wrong client received message")
				case <-receiver[1].ch:
				case <-receiver[2].ch:
					Fail("Wrong client received message")
				case <-time.After(2 * time.Second):
					Fail("timeout waiting for clients getting results")
				}
				cancel()
			})
		})
	}
})

func TestGroupShouldInvokeOnlyTheClientsInTheGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, client, receiver, srvConn, _, err := makeTCPServerAndClients(ctx, 3)
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
	_, client, receiver, srvConn, _, err := makeTCPServerAndClients(ctx, 3)
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
	_, client, _, _, _, err := makeTCPServerAndClients(ctx, 2)
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
		assert.Equal(t, 1.0, ir.Value)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "timeout in invoke")
	}
}

func TestAbortShouldAbortTheConnectionOfTheCurrentCaller(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, client, _, _, _, err := makeTCPServerAndClients(ctx, 2)
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
		assert.Equal(t, 2.0, ir.Value)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "timeout in invoke")
	}
}

// TestGroupMapConcurrentAccess verifies no data race when AddToGroup, RemoveFromGroup
// and InvokeGroup run concurrently on the same group — detectable by go test -race.
func TestGroupMapConcurrentAccess(t *testing.T) {
	mgr := newLifeTimeManager(testLogger())

	const n = 50
	conns := make([]*mockHubConn, n)
	for i := range conns {
		conns[i] = &mockHubConn{id: fmt.Sprintf("conn-%d", i)}
		mgr.OnConnected(conns[i])
	}

	var wg sync.WaitGroup
	for i := range conns {
		id := conns[i].ConnectionID()
		wg.Add(3)
		go func() {
			defer wg.Done()
			mgr.AddToGroup("g", id)
		}()
		go func() {
			defer wg.Done()
			mgr.RemoveFromGroup("g", id)
		}()
		go func() {
			defer wg.Done()
			mgr.InvokeGroup("g", "noop", nil)
		}()
	}
	wg.Wait()
}

// mockHubConn is a no-op HubConnection used for concurrency tests.
type mockHubConn struct{ id string }

func (m *mockHubConn) ConnectionID() string                                     { return m.id }
func (m *mockHubConn) Receive() <-chan receiveResult                            { return nil }
func (m *mockHubConn) SendInvocation(string, string, []interface{}) error       { return nil }
func (m *mockHubConn) SendStreamInvocation(string, string, []interface{}) error { return nil }
func (m *mockHubConn) SendInvocationWithStreamIds(string, string, []interface{}, []string) error {
	return nil
}
func (m *mockHubConn) StreamItem(string, interface{}) error         { return nil }
func (m *mockHubConn) Completion(string, interface{}, string) error { return nil }
func (m *mockHubConn) Close(string, bool) error                     { return nil }
func (m *mockHubConn) Ping() error                                  { return nil }
func (m *mockHubConn) LastWriteStamp() time.Time                    { return time.Time{} }
func (m *mockHubConn) Items() *sync.Map                             { return &sync.Map{} }
func (m *mockHubConn) Context() context.Context                     { return context.Background() }
func (m *mockHubConn) Abort()                                       {}

// TestDisconnectedConnectionRemovedFromGroup verifies that OnDisconnected removes
// the connection from all groups so InvokeGroup no longer reaches it.
func TestDisconnectedConnectionRemovedFromGroup(t *testing.T) {
	mgr := newLifeTimeManager(testLogger())

	called1 := make(chan struct{}, 1)
	called2 := make(chan struct{}, 1)
	conn1 := &invokingHubConn{mockHubConn: mockHubConn{id: "c1"}, called: called1}
	conn2 := &invokingHubConn{mockHubConn: mockHubConn{id: "c2"}, called: called2}

	mgr.OnConnected(conn1)
	mgr.OnConnected(conn2)
	mgr.AddToGroup("g", "c1")
	mgr.AddToGroup("g", "c2")

	mgr.OnDisconnected(conn2)

	mgr.InvokeGroup("g", "m", nil)

	select {
	case <-called1:
		// expected: conn1 still in group
	case <-time.After(200 * time.Millisecond):
		assert.Fail(t, "conn1 should have been invoked")
	}
	select {
	case <-called2:
		assert.Fail(t, "disconnected conn2 should not be invoked")
	case <-time.After(50 * time.Millisecond):
		// expected: conn2 was removed from group on disconnect
	}
}

type invokingHubConn struct {
	mockHubConn
	called chan struct{}
}

func (m *invokingHubConn) ConnectionID() string { return m.mockHubConn.id }
func (m *invokingHubConn) SendInvocation(string, string, []interface{}) error {
	select {
	case m.called <- struct{}{}:
	default:
	}
	return nil
}
