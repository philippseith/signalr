package signalr

import (
	"context"
	"fmt"
	"github.com/go-kit/kit/log"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"os"
	"strings"
	"time"
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

func connectMany() []*testingConnection {
	server, err := NewServer(SimpleHubFactory(&contextHub{}),
		Logger(log.NewLogfmtLogger(os.Stderr), false))
	if err != nil {
		Fail(err.Error())
		return nil
	}
	conns := make([]*testingConnection, 3)
	for i := 0; i < 3; i++ {
		conns[i] = newTestingConnection()
		go server.Run(context.TODO(), conns[i])
		// Ensure to return all connection with connected hubs
		<-hubContextOnConnectMsg
	}

	return conns
}

var _ = Describe("HubContext", func() {
	Context("Clients().All()", func() {
		It("should invoke all clients", func() {
			conns := connectMany()
			conns[0].ClientSend(`{"type":1,"invocationId": "123","target":"callall"}`)
			callCount := make(chan int, 1)
			callCount <- 0
			done := make(chan bool)
			go func(conns []*testingConnection, callCount chan int, done chan bool) {
				msg := <-conns[0].received
				if _, ok := msg.(completionMessage); ok {
					msg = <-conns[0].received
					expectInvocation(msg, callCount, done, 3)
				} else {
					expectInvocation(msg, callCount, done, 3)
				}
			}(conns, callCount, done)
			go func(conns []*testingConnection, callCount chan int, done chan bool) {
				msg := <-conns[1].received
				expectInvocation(msg, callCount, done, 3)
			}(conns, callCount, done)
			go func(conns []*testingConnection, callCount chan int, done chan bool) {
				msg := <-conns[2].received
				expectInvocation(msg, callCount, done, 3)
			}(conns, callCount, done)
			Expect(<-hubContextInvocationQueue).To(Equal("CallAll()"))
			select {
			case <-done:
				break
			case <-time.After(3000 * time.Millisecond):
				Fail("timed out")
			}
		})
	})

	Context("Clients().Caller()", func() {
		It("should invoke only the caller", func() {
			conns := connectMany()
			conns[0].ClientSend(`{"type":1,"invocationId": "123","target":"callcaller"}`)
			done := make(chan bool)
			callCount := make(chan int, 1)
			callCount <- 0
			go func(conns []*testingConnection, done chan bool) {
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
		})
	})

	Context("Clients().Client()", func() {
		It("should invoke only the client which was addressed", func() {
			conns := connectMany()
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
		})
	})

	Context("Clients().Group()", func() {
		It("should invoke only the clients in the group", func() {
			conns := connectMany()
			conns[0].ClientSend(fmt.Sprintf(`{"type":1,"invocationId": "123","target":"buildgroup","arguments":["%v","%v"]}`, conns[1].ConnectionID(), conns[2].ConnectionID()))
			<-hubContextInvocationQueue
			<-conns[0].received
			conns[0].ClientSend(`{"type":1,"invocationId": "123","target":"callgroup"}`)
			callCount := make(chan int, 1)
			callCount <- 0
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
				expectInvocation(msg, callCount, done, 2)
			}(conns)
			go func(conns []*testingConnection, done chan bool) {
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
		})
	})

	Context("RemoveFromGroup should remove clients from the group", func() {
		It("should invoke only the clients in the group", func() {
			conns := connectMany()
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
				expectInvocation(msg, callCount, done, 1)
			}(conns)
			go func(conns []*testingConnection, done chan bool) {
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
		})
	})

	Context("Items()", func() {
		It("should hold Items connection wise", func() {
			conns := connectMany()
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
		})
	})

	Context("Abort()", func() {
		It("should abort the connection of the current caller", func() {
			conn0 := connect(&contextHub{})
			conn1 := connect(&contextHub{})
			conn0.ClientSend(`{"type":1,"invocationId": "ab0ab0","target":"abort"}`)
			// Wait for execution
			Expect(<-hubContextInvocationQueue).To(Equal("Abort()"))
			// Abort should close
			msg := <-conn0.received
			Expect(msg).To(BeAssignableToTypeOf(closeMessage{}))
			Expect(msg.(closeMessage).Error).NotTo(BeNil())
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

		})
	})
})

func expectInvocation(msg interface{}, callCount chan int, done chan bool, doneCount int) {
	Expect(msg).To(BeAssignableToTypeOf(invocationMessage{}))
	Expect(strings.ToLower(msg.(invocationMessage).Target)).To(Equal("clientfunc"))
	d := <-callCount
	d++
	if d == doneCount {
		done <- true
	} else {
		callCount <- d
	}
}
