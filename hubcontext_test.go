package signalr

import (
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
		go server.Run(conns[i])
		// Ensure to return all connection with connected hubs
		<-hubContextOnConnectMsg
	}

	return conns
}

var _ = Describe("HubContext", func() {
	Context("Clients().All()", func() {
		It("should invoke all clients", func() {
			conns := connectMany()
			conns[0].clientSend(`{"type":1,"invocationId": "123","target":"callall"}`)
			callCount := make(chan int, 1)
			callCount <- 0
			done := make(chan bool)
			go func(conns []*testingConnection, callCount chan int, done chan bool) {
				msg := <-conns[0].received
				if _, ok := msg.(completionMessage); ok {
					msg = <-conns[0].received
					expectInvocation(msg, callCount, done)
				} else {
					expectInvocation(msg, callCount, done)
				}
			}(conns, callCount, done)
			go func(conns []*testingConnection, callCount chan int, done chan bool) {
				msg := <-conns[1].received
				expectInvocation(msg, callCount, done)
			}(conns, callCount, done)
			go func(conns []*testingConnection, callCount chan int, done chan bool) {
				msg := <-conns[2].received
				expectInvocation(msg, callCount, done)
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
			conns[0].clientSend(`{"type":1,"invocationId": "123","target":"callcaller"}`)
			done := make(chan bool)
			go func(conns []*testingConnection, done chan bool) {
				msg := <-conns[0].received
				if _, ok := msg.(completionMessage); ok {
					msg = <-conns[0].received
					Expect(msg).To(BeAssignableToTypeOf(invocationMessage{}))
					Expect(strings.ToLower(msg.(invocationMessage).Target)).To(Equal("clientfunc"))
					done <- true
				} else {
					Expect(msg).To(BeAssignableToTypeOf(invocationMessage{}))
					Expect(strings.ToLower(msg.(invocationMessage).Target)).To(Equal("clientfunc"))
					done <- true
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
			conns[0].clientSend(fmt.Sprintf(`{"type":1,"invocationId": "123","target":"callclient","arguments":["%v"]}`, conns[2].ConnectionID()))
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

})

func expectInvocation(msg interface{}, callCount chan int, done chan bool) {
	Expect(msg).To(BeAssignableToTypeOf(invocationMessage{}))
	Expect(strings.ToLower(msg.(invocationMessage).Target)).To(Equal("clientfunc"))
	d := <-callCount
	d++
	if d == 3 {
		done <- true
	} else {
		callCount <- d
	}
}
