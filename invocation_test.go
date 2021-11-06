package signalr

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var invocationQueue = make(chan string, 20)

type invocationHub struct {
	Hub
}

func (i *invocationHub) Simple() {
	invocationQueue <- "Simple()"
}

func (i *invocationHub) SimpleInt(value int) int {
	invocationQueue <- fmt.Sprintf("SimpleInt(%v)", value)
	return value + 1
}

func (i *invocationHub) SimpleFloat(value float64) (float64, float64) {
	invocationQueue <- fmt.Sprintf("SimpleFloat(%v)", value)
	return value * 10.0, value * 100.0
}

func (i *invocationHub) SimpleString(value1 string, value2 string) string {
	invocationQueue <- fmt.Sprintf("SimpleString(%v, %v)", value1, value2)
	return strings.ToLower(value1 + value2)
}

func (i *invocationHub) Async() chan bool {
	r := make(chan bool)
	go func() {
		defer close(r)
		r <- true
	}()
	invocationQueue <- "Async()"
	return r
}

func (i *invocationHub) AsyncClosedChan() chan bool {
	r := make(chan bool)
	close(r)
	invocationQueue <- "AsyncClosedChan()"
	return r
}

func (i *invocationHub) Panic() {
	invocationQueue <- "Panic()"
	panic("Don't panic!")
}

var _ = Describe("Invocation", func() {

	Describe("Simple invocation", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&invocationHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("When invoked by the client", func() {
			It("should be invoked and return a completion", func(done Done) {
				conn.ClientSend(`{"type":1,"invocationId": "123","target":"simple"}`)
				Expect(<-invocationQueue).To(Equal("Simple()"))
				recv := (<-conn.received).(completionMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.InvocationID).To(Equal("123"))
				Expect(recv.Result).To(BeNil())
				Expect(recv.Error).To(Equal(""))
				close(done)
			}, 2.0)
		})
		Context("When invoked by the client two times in one frame", func() {
			It("should be invoked and return a completion", func(done Done) {
				conn.ClientSend(`{"type":1,"invocationId": "123","target":"simple"}`)
				conn.ClientSend(`{"type":1,"invocationId": "123","target":"simple"}`)
				Expect(<-invocationQueue).To(Equal("Simple()"))
				Expect(<-invocationQueue).To(Equal("Simple()"))
				recv := (<-conn.received).(completionMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.InvocationID).To(Equal("123"))
				Expect(recv.Result).To(BeNil())
				Expect(recv.Error).To(Equal(""))
				close(done)
			}, 2.0)
		})
	})

	Describe("Non blocking invocation", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&invocationHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("When invoked by the client", func() {
			It("should be invoked and return no completion", func(done Done) {
				conn.ClientSend(`{"type":1,"target":"simple"}`)
				Expect(<-invocationQueue).To(Equal("Simple()"))
				select {
				case message := <-conn.received:
					if _, ok := message.(completionMessage); ok {
						Fail("received completion ")
					}
				case <-time.After(1000 * time.Millisecond):
				}
				close(done)
			}, 2.0)
		})
	})

	Describe("Invalid invocation", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&invocationHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("When an invalid invocation message is sent", func() {
			It("close the connection with error", func(done Done) {
				// Invalid. invocationId should be a string
				conn.ClientSend(`{"type":1,"invocationId":1}`)
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
				close(done)
			}, 2.0)
		})
	})

	Describe("Invalid json", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&invocationHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("when invalid json is received", func() {
			It("should close the connection with an error", func(done Done) {
				conn.ClientSend(`{"type":1,"invocationId": "4444","target":"simpleint", arguments[CanNotParse]}`)
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
				close(done)
			}, 2.0)
		})
	})

	Describe("SimpleInt invocation", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&invocationHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("When invoked by the client", func() {
			It("should be invoked on the server, get an int and return an int", func(done Done) {
				var value = 314
				conn.ClientSend(fmt.Sprintf(
					`{"type":1,"invocationId": "666","target":"simpleint","arguments":[%v]}`, value))
				Expect(<-invocationQueue).To(Equal(fmt.Sprintf("SimpleInt(%v)", value)))
				recv := (<-conn.received).(completionMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.InvocationID).To(Equal("666"))
				Expect(recv.Result).To(Equal(float64(value + 1))) // json  makes all numbers float64
				Expect(recv.Error).To(Equal(""))
				close(done)
			}, 2.0)
		})
	})

	Describe("SimpleInt invocation with invalid argument", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&invocationHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("When invoked by the client with an invalid argument", func() {
			It("should not be invoked on the server and return an error", func(done Done) {
				conn.ClientSend(
					`{"type":1,"invocationId": "555","target":"simpleint","arguments":["CantParse"]}`)
				recv := (<-conn.received).(completionMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.Error).NotTo(Equal(""))
				Expect(recv.InvocationID).To(Equal("555"))
				close(done)
			}, 2.0)
		})
	})

	Describe("SimpleFloat invocation", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&invocationHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("When invoked by the client", func() {
			It("should be invoked on the server, get a float and return a two floats", func(done Done) {
				var value = 3.1415
				conn.ClientSend(fmt.Sprintf(
					`{"type":1,"invocationId": "8087","target":"simplefloat","arguments":[%v]}`, value))
				Expect(<-invocationQueue).To(Equal(fmt.Sprintf("SimpleFloat(%v)", value)))
				recv := (<-conn.received).(completionMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.InvocationID).To(Equal("8087"))
				Expect(recv.Result).To(Equal([]interface{}{value * 10.0, value * 100.0}))
				Expect(recv.Error).To(Equal(""))
				close(done)
			}, 2.0)
		})
	})

	Describe("SimpleString invocation", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&invocationHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("When invoked by the client", func() {
			It("should be invoked on the server, get two strings and return a string", func(done Done) {
				value1 := "Camel"
				value2 := "Cased"
				conn.ClientSend(fmt.Sprintf(
					`{"type":1,"invocationId": "6502","target":"simplestring","arguments":["%v", "%v"]}`, value1, value2))
				Expect(<-invocationQueue).To(Equal(fmt.Sprintf("SimpleString(%v, %v)", value1, value2)))
				recv := (<-conn.received).(completionMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.InvocationID).To(Equal("6502"))
				Expect(recv.Result).To(Equal(strings.ToLower(value1 + value2)))
				Expect(recv.Error).To(Equal(""))
				close(done)
			}, 2.0)
		})
	})

	Describe("Async invocation", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&invocationHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("When invoked by the client", func() {
			It("should be invoked on the server and return true asynchronously", func(done Done) {
				conn.ClientSend(`{"type":1,"invocationId": "mfg","target":"async"}`)
				Expect(<-invocationQueue).To(Equal("Async()"))
				recv := (<-conn.received).(completionMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.InvocationID).To(Equal("mfg"))
				Expect(recv.Result).To(Equal(true))
				Expect(recv.Error).To(Equal(""))
				close(done)
			}, 2.0)
		})
	})

	Describe("Async invocation with buggy server method which returns a closed channel", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&invocationHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("When invoked by the client", func() {
			It("should be invoked on the server and return an error", func(done Done) {
				conn.ClientSend(`{"type":1,"invocationId": "ouch","target":"asyncclosedchan"}`)
				Expect(<-invocationQueue).To(Equal("AsyncClosedChan()"))
				recv := (<-conn.received).(completionMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.InvocationID).To(Equal("ouch"))
				Expect(recv.Result).To(BeNil())
				Expect(recv.Error).NotTo(BeNil())
				close(done)
			}, 2.0)
		})
	})

	Describe("Panic in invoked func", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&invocationHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})

		Context("When a func is invoked by the client and panics", func() {
			It("should be invoked on the server and return an error but no result", func(done Done) {
				conn.ClientSend(`{"type":1,"invocationId": "???","target":"panic"}`)
				Expect(<-invocationQueue).To(Equal("Panic()"))
				recv := (<-conn.received).(completionMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.InvocationID).To(Equal("???"))
				Expect(recv.Result).To(BeNil())
				Expect(recv.Error).NotTo(Equal(""))
				close(done)
			}, 2.0)
		})
	})

	Describe("Missing method invocation", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&invocationHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("When a missing server method invoked by the client", func() {
			It("should return an error", func(done Done) {
				conn.ClientSend(`{"type":1,"invocationId": "0000","target":"missing"}`)
				recv := (<-conn.received).(completionMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.InvocationID).To(Equal("0000"))
				Expect(recv.Result).To(BeNil())
				Expect(recv.Error).NotTo(BeNil())
				Expect(len(recv.Error)).To(BeNumerically(">", 0))
				close(done)
			}, 2.0)
		})
	})

})
