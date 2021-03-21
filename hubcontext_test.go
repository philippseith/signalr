package signalr

import (
	"context"
	"fmt"
	"strings"
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

func (c *contextHub) Abort() {
	hubContextInvocationQueue <- "Abort()"
	c.context.Abort()
}

var hubContextInvocationQueue = make(chan string, 10)

func connectMany() (Server, []*testingConnection) {
	server, err := NewServer(context.TODO(), SimpleHubFactory(&contextHub{}),
		testLoggerOption())
	if err != nil {
		Fail(err.Error())
		return nil, nil
	}
	conns := make([]*testingConnection, 3)
	for i := 0; i < 3; i++ {
		conns[i] = newTestingConnectionForServer()
		go server.Serve(conns[i])
		// Ensure to return all connection with connected hubs
		<-hubContextOnConnectMsg
	}

	return server, conns
}

var _ = Describe("HubContext", func() {
	var server Server
	var conns []*testingConnection
	BeforeEach(func() {
		server, conns = connectMany()
	})
	AfterEach(func() {
		server.cancel()
	})
	Context("Clients().All()", func() {
		It("should invoke all clients", func(didIt Done) {
			conns[0].ClientSend(`{"type":1,"invocationId": "123","target":"callall"}`)
			callCount := make(chan int, 1)
			callCount <- 0
			done := make(chan bool)
			go func(conns []*testingConnection, callCount chan int, done chan bool) {
				defer GinkgoRecover()
				msg := <-conns[0].received
				if _, ok := msg.(completionMessage); ok {
					msg = <-conns[0].received
					expectInvocation(msg, callCount, done, 3)
				} else {
					expectInvocation(msg, callCount, done, 3)
				}
			}(conns, callCount, done)
			go func(conns []*testingConnection, callCount chan int, done chan bool) {
				defer GinkgoRecover()
				msg := <-conns[1].received
				if _, ok := msg.(completionMessage); ok {
					msg = <-conns[1].received
					expectInvocation(msg, callCount, done, 3)
				} else {
					expectInvocation(msg, callCount, done, 3)
				}
			}(conns, callCount, done)
			go func(conns []*testingConnection, callCount chan int, done chan bool) {
				defer GinkgoRecover()
				msg := <-conns[2].received
				if _, ok := msg.(completionMessage); ok {
					msg = <-conns[2].received
					expectInvocation(msg, callCount, done, 3)
				} else {
					expectInvocation(msg, callCount, done, 3)
				}
			}(conns, callCount, done)
			Expect(<-hubContextInvocationQueue).To(Equal("CallAll()"))
			select {
			case <-done:
				break
			case <-time.After(3000 * time.Millisecond):
				Fail("timed out")
			}
			close(didIt)
		}, 5.0)
	})
})

var _ = Describe("HubContext", func() {
	var server Server
	var conns []*testingConnection
	BeforeEach(func(done Done) {
		server, conns = connectMany()
		close(done)
	})
	AfterEach(func(done Done) {
		server.cancel()
		close(done)
	})

	Context("Clients().Caller()", func() {
		It("should invoke only the caller", func(didIt Done) {
			conns[0].ClientSend(`{"type":1,"invocationId": "123","target":"callcaller"}`)
			done := make(chan bool)
			callCount := make(chan int, 1)
			callCount <- 0
			go func(conns []*testingConnection, done chan bool) {
				defer GinkgoRecover()
				msg := <-conns[0].received
				if _, ok := msg.(completionMessage); ok {
					msg = <-conns[0].received
					expectInvocation(msg, callCount, done, 1)
				} else {
					expectInvocation(msg, callCount, done, 1)
				}
			}(conns, done)
			go func(conns []*testingConnection) {
				msg := <-conns[1].received
				if _, ok := msg.(completionMessage); ok {
					Fail(fmt.Sprintf("non caller received %v", msg))
				}
			}(conns)
			go func(conns []*testingConnection) {
				msg := <-conns[2].received
				if _, ok := msg.(completionMessage); ok {
					Fail(fmt.Sprintf("non caller received %v", msg))
				}
			}(conns)
			Expect(<-hubContextInvocationQueue).To(Equal("CallCaller()"))
			select {
			case <-done:
				break
			case <-time.After(3000 * time.Millisecond):
				Fail("timed out")
			}
			close(didIt)
		}, 4.0)
	})

	Context("Clients().Client()", func() {
		It("should invoke only the client which was addressed", func(didIt Done) {
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
			close(didIt)
		}, 4.0)
	})

	Context("Clients().Group()", func() {
		It("should invoke only the clients in the group", func(ditIt Done) {
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
				expectInvocation(msg, callCount, done, 2)
			}(conns)
			go func(conns []*testingConnection, done chan bool) {
				defer GinkgoRecover()
				msg := <-conns[2].received
				expectInvocation(msg, callCount, done, 2)
			}(conns, done)
			Expect(<-hubContextInvocationQueue).To(Equal("CallGroup()"))
			select {
			case <-done:
				break
			case <-time.After(3000 * time.Millisecond):
				Fail("timed out")
			}
			close(ditIt)
		}, 4.0)
	})

	Context("RemoveFromGroup should remove clients from the group", func() {
		It("should invoke only the clients in the group", func(ditIt Done) {
			conns[0].ClientSend(fmt.Sprintf(`{"type":1,"invocationId": "123","target":"buildgroup","arguments":["%v","%v"]}`, conns[1].ConnectionID(), conns[2].ConnectionID()))
			Expect(<-hubContextInvocationQueue).To(Equal("BuildGroup()"))
			<-conns[0].received
			conns[2].ClientSend(fmt.Sprintf(`{"type":1,"invocationId": "123","target":"removefromgroup","arguments":["%v"]}`, conns[2].ConnectionID()))
			Expect(<-hubContextInvocationQueue).To(Equal("RemoveFromGroup()"))
			<-conns[2].received
			// Now only conns[1] should be invoked
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
				expectInvocation(msg, callCount, done, 1)
			}(conns)
			go func(conns []*testingConnection, done chan bool) {
				defer GinkgoRecover()
				msg := <-conns[2].received
				if _, ok := msg.(completionMessage); ok {
					Fail(fmt.Sprintf("wrong client received %v", msg))
				}
			}(conns, done)
			Expect(<-hubContextInvocationQueue).To(Equal("CallGroup()"))
			select {
			case <-done:
				break
			case <-time.After(3000 * time.Millisecond):
				Fail("timed out")
			}
			close(ditIt)
		}, 4.0)
	})

	Context("Items()", func() {
		It("should hold Items connection wise", func(done Done) {
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
			close(done)
		}, 2.0)
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
		go server.Serve(conn0)
		conn1 := newTestingConnectionForServer()
		go server.Serve(conn1)
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

func expectInvocation(msg interface{}, callCount chan int, done chan bool, doneCount int) {
	Expect(msg).To(BeAssignableToTypeOf(invocationMessage{}), fmt.Sprintf("Expected invocationMessage, got %T %#v", msg, msg))
	Expect(strings.ToLower(msg.(invocationMessage).Target)).To(Equal("clientfunc"))
	d := <-callCount
	d++
	if d == doneCount {
		done <- true
	} else {
		callCount <- d
	}
}
