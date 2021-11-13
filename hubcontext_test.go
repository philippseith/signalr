package signalr

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type contextHub struct {
	Hub
}

var hubContextOnConnectMsg = make(chan string, 1)

func (c *contextHub) OnConnected(connectionID string) {
	hubContextOnConnectMsg <- connectionID
}

func (c *contextHub) CallAll() {
	c.Clients().All().Send("clientFunc")
	hubContextInvocationQueue <- "CallAll()"
}

func (c *contextHub) CallCaller() {
	c.Clients().Caller().Send("clientFunc")
	hubContextInvocationQueue <- "CallCaller()"
}

func (c *contextHub) CallClient(connectionID string) {
	c.Clients().Client(connectionID).Send("clientFunc")
	hubContextInvocationQueue <- "CallClient()"
}

func (c *contextHub) BuildGroup(connectionID1 string, connectionID2 string) {
	c.Groups().AddToGroup("local", connectionID1)
	c.Groups().AddToGroup("local", connectionID2)
	hubContextInvocationQueue <- "BuildGroup()"
}

func (c *contextHub) RemoveFromGroup(connectionID string) {
	c.Groups().RemoveFromGroup("local", connectionID)
	hubContextInvocationQueue <- "RemoveFromGroup()"
}

func (c *contextHub) CallGroup() {
	c.Clients().Group("local").Send("clientFunc")
	hubContextInvocationQueue <- "CallGroup()"
}

func (c *contextHub) AddItem(key string, value interface{}) {
	c.Items().Store(key, value)
	hubContextInvocationQueue <- "AddItem()"
}

func (c *contextHub) GetItem(key string) interface{} {
	hubContextInvocationQueue <- "GetItem()"
	if item, ok := c.Items().Load(key); ok {
		return item
	}
	return nil
}

func (c *contextHub) TestConnectionID() {
	hubContextInvocationQueue <- c.ConnectionID()
}

func (c *contextHub) Abort() {
	hubContextInvocationQueue <- "Abort()"
	c.Hub.Abort()
}

var hubContextInvocationQueue = make(chan string, 10)

func connectMany() (Server, []*testingConnection, []string) {
	server, err := NewServer(context.TODO(), SimpleHubFactory(&contextHub{}),
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
			_ = server.Serve(conns[i])
		}(i)
	}
	wg.Wait()
	for i := 0; i < 3; i++ {
		// Ensure to return all connection with connected hubs
		connIds = append(connIds, <-hubContextOnConnectMsg)
	}
	return server, conns, connIds
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
		errCh <- fmt.Errorf("received unspected message %v", msg)
	case <-ctx.Done():
	}
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
				Fail("timeout waiting for clients getting results")
			}
		})
	})
	Context("Clients().Client()", func() {
		It("should invoke only the client which was addressed", func(didIt Done) {
			server, conns, _ := connectMany()
			conns[0].ClientSend(fmt.Sprintf(`{"type":1,"invocationId": "123","target":"callclient","arguments":["%v"]}`, conns[2].ConnectionID()))
			done := make(chan bool)
			go func(conns []*testingConnection) {
				msg := <-conns[0].received
				if _, ok := msg.(completionMessage); ok {
					msg = <-conns[0].received
					if _, ok := msg.(completionMessage); ok {
						Fail(fmt.Sprintf("wrong client received %v", msg))
					}
				} else {
					if _, ok := msg.(completionMessage); ok {
						Fail(fmt.Sprintf("wrong client received %v", msg))
					}
				}
			}(conns)
			go func(conns []*testingConnection) {
				msg := <-conns[1].received
				if _, ok := msg.(completionMessage); ok {
					Fail(fmt.Sprintf("wrong client received %v", msg))
				}
			}(conns)
			go func(conns []*testingConnection, done chan bool) {
				defer GinkgoRecover()
				msg := <-conns[2].received
				Expect(msg).To(BeAssignableToTypeOf(invocationMessage{}))
				Expect(strings.ToLower(msg.(invocationMessage).Target)).To(Equal("clientfunc"))
				done <- true
			}(conns, done)
			Expect(<-hubContextInvocationQueue).To(Equal("CallClient()"))
			select {
			case <-done:
				break
			case <-time.After(3000 * time.Millisecond):
				Fail("timed out")
			}
			server.cancel()
			close(didIt)
		}, 4.0)
	})
	Context("Clients().Group()", func() {
		It("should invoke only the clients in the group", func(ditIt Done) {
			server, conns, _ := connectMany()
			conns[0].ClientSend(fmt.Sprintf(`{"type":1,"invocationId": "123","target":"buildgroup","arguments":["%v","%v"]}`, conns[1].ConnectionID(), conns[2].ConnectionID()))
			<-hubContextInvocationQueue
			<-conns[0].received
			conns[0].ClientSend(`{"type":1,"invocationId": "123","target":"callgroup"}`)
			callCount := make(chan int, 1)
			callCount <- 0
			done := make(chan bool)
			go func(conns []*testingConnection) {
				defer GinkgoRecover()
				msg := <-conns[0].received
				if _, ok := msg.(completionMessage); ok {
					msg = <-conns[0].received
					if _, ok := msg.(completionMessage); ok {
						Fail(fmt.Sprintf("wrong client received %v", msg))
					}
				} else {
					if _, ok := msg.(completionMessage); ok {
						Fail(fmt.Sprintf("wrong client received %v", msg))
					}
				}
			}(conns)
			go func(conns []*testingConnection) {
				defer GinkgoRecover()
				msg := <-conns[1].received
				Expect(expectInvocation(msg, callCount, done, 2)).NotTo(HaveOccurred())
			}(conns)
			go func(conns []*testingConnection, done chan bool) {
				defer GinkgoRecover()
				msg := <-conns[2].received
				Expect(expectInvocation(msg, callCount, done, 2)).NotTo(HaveOccurred())
			}(conns, done)
			Expect(<-hubContextInvocationQueue).To(Equal("CallGroup()"))
			select {
			case <-done:
				break
			case <-time.After(3000 * time.Millisecond):
				Fail("timed out")
			}
			server.cancel()
			close(ditIt)
		}, 4.0)
	})
	Context("RemoveFromGroup should remove clients from the group", func() {
		It("should invoke only the clients in the group", func(ditIt Done) {
			server, conns, _ := connectMany()
			conns[0].ClientSend(fmt.Sprintf(`{"type":1,"invocationId": "123","target":"buildgroup","arguments":["%v","%v"]}`, conns[1].ConnectionID(), conns[2].ConnectionID()))
			Expect(<-hubContextInvocationQueue).To(Equal("BuildGroup()"))
			<-conns[0].received
			conns[2].ClientSend(fmt.Sprintf(`{"type":1,"invocationId": "123","target":"removefromgroup","arguments":["%v"]}`, conns[2].ConnectionID()))
			Expect(<-hubContextInvocationQueue).To(Equal("RemoveFromGroup()"))
			<-conns[2].received
			// Now only conns[1] should be invoked
			conns[0].ClientSend(`{"type":1,"invocationId": "123","target":"callgroup"}`)
			done := make(chan bool)
			go func(conns []*testingConnection) {
				defer GinkgoRecover()
				msg := <-conns[0].received
				if _, ok := msg.(completionMessage); ok {
					msg = <-conns[0].received
					if _, ok := msg.(completionMessage); ok {
						Fail(fmt.Sprintf("wrong client received %v", msg))
					}
				} else {
					if _, ok := msg.(completionMessage); ok {
						Fail(fmt.Sprintf("wrong client received %v", msg))
					}
				}
			}(conns)
			go func(conns []*testingConnection) {
				defer GinkgoRecover()
				msg := <-conns[1].received
				callCount := make(chan int, 1)
				callCount <- 0
				Expect(expectInvocation(msg, callCount, done, 1)).NotTo(HaveOccurred())
			}(conns)
			go func(conns []*testingConnection, done chan bool) {
				defer GinkgoRecover()
				msg := <-conns[2].received
				if _, ok := msg.(completionMessage); ok {
					Fail(fmt.Sprintf("wrong client received %v", msg))
				}
			}(conns, done)
			Expect(<-hubContextInvocationQueue).To(Equal("CallGroup()"))
			<-done
			server.cancel()
			close(ditIt)
		}, 4.0)
	})

	Context("Items()", func() {
		It("should hold Items connection wise", func(done Done) {
			server, conns, _ := connectMany()
			conns[0].ClientSend(`{"type":1,"invocationId": "123","target":"additem","arguments":["first",1]}`)
			// Wait for execution
			Expect(<-hubContextInvocationQueue).To(Equal("AddItem()"))
			// Read completion
			<-conns[0].received
			conns[0].ClientSend(`{"type":1,"invocationId": "123","target":"getitem","arguments":["first"]}`)
			// Wait for execution
			Expect(<-hubContextInvocationQueue).To(Equal("GetItem()"))
			msg := <-conns[0].received
			Expect(msg).To(BeAssignableToTypeOf(completionMessage{}))
			Expect(msg.(completionMessage).Result).To(Equal(float64(1)))
			// Ask on other connection
			conns[1].ClientSend(`{"type":1,"invocationId": "123","target":"getitem","arguments":["first"]}`)
			// Wait for execution
			Expect(<-hubContextInvocationQueue).To(Equal("GetItem()"))
			msg = <-conns[1].received
			Expect(msg).To(BeAssignableToTypeOf(completionMessage{}))
			Expect(msg.(completionMessage).Result).To(BeNil())
			server.cancel()
			close(done)
		}, 2.0)
	})

	Context("ConnectionID", func() {
		It("should be the ID of the connection", func() {
			server, conns, _ := connectMany()
			conns[0].ClientSend(`{"type":1,"invocationId": "ABC","target":"testconnectionid"}`)
			id := <-hubContextInvocationQueue
			Expect(strings.Index(id, "test")).To(Equal(0))
			server.cancel()
		})
	})
})

var _ = Describe("Abort()", func() {
	It("should abort the connection of the current caller", func(done Done) {
		server, err := NewServer(context.TODO(), SimpleHubFactory(&contextHub{}),
			testLoggerOption(),
			ChanReceiveTimeout(200*time.Millisecond),
			StreamBufferCapacity(5))
		Expect(err).NotTo(HaveOccurred())
		conn0 := newTestingConnectionForServer()
		go func() { _ = server.Serve(conn0) }()
		conn1 := newTestingConnectionForServer()
		go func() { _ = server.Serve(conn1) }()
		conn0.ClientSend(`{"type":1,"invocationId": "ab0ab0","target":"abort"}`)
		// Wait for execution
		Expect(<-hubContextInvocationQueue).To(Equal("Abort()"))
		// We get the completion and the close message, the order depends on server timing
		msg := <-conn0.received
		_, isClose := msg.(closeMessage)
		Expect(isClose).To(Equal(true))
		// This connection should not work anymore
		conn0.ClientSend(`{"type":1,"invocationId": "ab123","target":"additem","arguments":["first",2]}`)
		select {
		case <-conn0.received:
			Fail("closed connection still receives messages")
		case <-time.After(10 * time.Millisecond):
			// OK
		}
		// Other connections should still work
		conn1.ClientSend(`{"type":1,"invocationId": "ab123","target":"additem","arguments":["first",2]}`)
		// Wait for execution
		Expect(<-hubContextInvocationQueue).To(Equal("AddItem()"))
		// Read completion
		<-conn1.received
		conn1.ClientSend(`{"type":1,"invocationId": "ab123","target":"getitem","arguments":["first"]}`)
		// Wait for execution
		Expect(<-hubContextInvocationQueue).To(Equal("GetItem()"))
		msg = <-conn1.received
		Expect(msg).To(BeAssignableToTypeOf(completionMessage{}))
		Expect(msg.(completionMessage).Result).To(Equal(float64(2)))
		server.cancel()
		close(done)
	}, 10.0)
})

func expectInvocation(msg interface{}, callCount chan int, done chan bool, doneCount int) error {
	defer func() {
		d := <-callCount
		d++
		if d == doneCount {
			done <- true
		} else {
			callCount <- d
		}
	}()
	if ivMsg, ok := msg.(invocationMessage); !ok {
		return fmt.Errorf("expected invocationMessage, got %T %#v", msg, msg)
	} else {
		if strings.ToLower(ivMsg.Target) != "clientfunc" {
			return fmt.Errorf("expected clientfunc, got got %#v", ivMsg)
		}
	}
	return nil
}
