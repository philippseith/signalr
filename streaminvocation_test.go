package signalr

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var streamInvocationQueue = make(chan string, 20)

type streamHub struct {
	Hub
}

func (s *streamHub) SimpleStream() <-chan int {
	r := make(chan int)
	go func() {
		defer close(r)
		for i := 1; i < 4; i++ {
			r <- i
		}
	}()
	streamInvocationQueue <- "SimpleStream()"
	return r
}

func (s *streamHub) EndlessStream() <-chan int {
	r := make(chan int)
	go func() {
		defer close(r)
		for i := 1; ; i++ {
			r <- i
		}
	}()
	streamInvocationQueue <- "EndlessStream()"
	return r
}

func (s *streamHub) SliceStream() <-chan []int {
	r := make(chan []int)
	go func() {
		defer close(r)
		for i := 1; i < 4; i++ {
			s := make([]int, 2)
			s[0] = i
			s[1] = i * 2
			r <- s
		}
	}()
	streamInvocationQueue <- "SliceStream()"
	return r
}

func (s *streamHub) SimpleInt() int {
	streamInvocationQueue <- "SimpleInt()"
	return -1
}

var _ = Describe("StreamInvocation", func() {

	Describe("Simple stream invocation", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&streamHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("When invoked by the client", func() {
			It("should be invoked on the server, return stream items and a final completion without result", func(done Done) {
				p := &jsonHubProtocol{dbg: testLogger()}
				conn.ClientSend(`{"type":4,"invocationId": "zzz","target":"simplestream"}`)
				Expect(<-streamInvocationQueue).To(Equal("SimpleStream()"))
				for i := 1; i < 4; i++ {
					recv := (<-conn.received).(streamItemMessage)
					Expect(recv).NotTo(BeNil())
					Expect(recv.InvocationID).To(Equal("zzz"))
					var f float64
					Expect(p.UnmarshalArgument(recv.Item, &f)).NotTo(HaveOccurred())
					Expect(f).To(Equal(float64(i)))
				}
				recv := (<-conn.received).(completionMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.InvocationID).To(Equal("zzz"))
				Expect(recv.Result).To(BeNil())
				Expect(recv.Error).To(Equal(""))
				close(done)
			})
		})
	})

	Describe("Slice stream invocation", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&streamHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("When invoked by the client", func() {
			It("should be invoked on the server, return stream items and a final completion without result", func(done Done) {
				protocol := jsonHubProtocol{dbg: testLogger()}
				conn.ClientSend(`{"type":4,"invocationId": "slice","target":"slicestream"}`)
				Expect(<-streamInvocationQueue).To(Equal("SliceStream()"))
				for i := 1; i < 4; i++ {
					recv := (<-conn.received).(streamItemMessage)
					Expect(recv).NotTo(BeNil())
					Expect(recv.InvocationID).To(Equal("slice"))
					exp := make([]int, 0, 2)
					exp = append(exp, i)
					exp = append(exp, i*2)
					var got []int
					Expect(protocol.UnmarshalArgument(recv.Item, &got)).NotTo(HaveOccurred())
					Expect(got).To(Equal(exp))
				}
				recv := (<-conn.received).(completionMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.InvocationID).To(Equal("slice"))
				Expect(recv.Result).To(BeNil())
				Expect(recv.Error).To(Equal(""))
				close(done)
			})
		})
	})

	Describe("Stop simple stream invocation", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&streamHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("When invoked by the client and stop after one result", func() {
			It("should be invoked on the server, return stream one item and a final completion without result", func(done Done) {
				protocol := jsonHubProtocol{dbg: testLogger()}
				conn.ClientSend(`{"type":4,"invocationId": "xxx","target":"endlessstream"}`)
				Expect(<-streamInvocationQueue).To(Equal("EndlessStream()"))
				recv := (<-conn.received).(streamItemMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.InvocationID).To(Equal("xxx"))
				var got int
				Expect(protocol.UnmarshalArgument(recv.Item, &got)).NotTo(HaveOccurred())
				Expect(got).To(Equal(1))
				// stop it
				conn.ClientSend(`{"type":5,"invocationId": "xxx"}`)
			loop:
				for {
					recv := <-conn.received
					Expect(recv).NotTo(BeNil())
					switch recv := recv.(type) {
					case streamItemMessage:
						Expect(recv.InvocationID).To(Equal("xxx"))
					case completionMessage:
						Expect(recv.InvocationID).To(Equal("xxx"))
						Expect(recv.Result).To(BeNil())
						Expect(recv.Error).To(Equal(""))
						break loop
					}
				}
				close(done)
			})
		})
	})

	Describe("Invalid CancelInvocation", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&streamHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("When invoked by the client and receiving an invalid CancelInvocation", func() {
			It("should close the connection with an error", func(done Done) {
				protocol := &jsonHubProtocol{dbg: testLogger()}
				conn.ClientSend(`{"type":4,"invocationId": "xyz","target":"endlessstream"}`)
				Expect(<-streamInvocationQueue).To(Equal("EndlessStream()"))
				recv := (<-conn.received).(streamItemMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.InvocationID).To(Equal("xyz"))
				var got int
				Expect(protocol.UnmarshalArgument(recv.Item, &got)).NotTo(HaveOccurred())
				Expect(got).To(Equal(1))
				// try to stop it, but do not get it right
				conn.ClientSend(`{"type":5,"invocationId":1}`)
			loop:
				for {
					message := <-conn.received
					switch message := message.(type) {
					case closeMessage:
						Expect(message.Error).NotTo(BeNil())
						break loop
					default:
					}
				}
				close(done)
			})
		})
	})

	Describe("Stream invocation of method with no stream result", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&streamHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("When invoked by the client", func() {
			It("should be invoked on the server, return one stream item with the \"no stream\" result and a final completion without result", func(done Done) {
				protocol := &jsonHubProtocol{dbg: testLogger()}
				conn.ClientSend(`{"type":4,"invocationId": "yyy","target":"simpleint"}`)
				Expect(<-streamInvocationQueue).To(Equal("SimpleInt()"))
				sRecv := (<-conn.received).(streamItemMessage)
				Expect(sRecv).NotTo(BeNil())
				Expect(sRecv.InvocationID).To(Equal("yyy"))
				var got int
				Expect(protocol.UnmarshalArgument(sRecv.Item, &got)).NotTo(HaveOccurred())
				Expect(got).To(Equal(-1))
				cRecv := (<-conn.received).(completionMessage)
				Expect(cRecv).NotTo(BeNil())
				Expect(cRecv.InvocationID).To(Equal("yyy"))
				Expect(cRecv.Result).To(BeNil())
				Expect(cRecv.Error).To(Equal(""))
				close(done)
			})
		})
	})

	Describe("invalid messages", func() {
		var server Server
		var conn *testingConnection
		BeforeEach(func(done Done) {
			server, conn = connect(&streamHub{})
			close(done)
		})
		AfterEach(func(done Done) {
			server.cancel()
			close(done)
		})
		Context("When an invalid stream invocation message is sent", func() {
			It("should return a completion with error", func(done Done) {
				conn.ClientSend(`{"type":4}`)
				select {
				case message := <-conn.received:
					completionMessage := message.(completionMessage)
					Expect(completionMessage).NotTo(BeNil())
					Expect(completionMessage.Error).NotTo(BeNil())
				case <-time.After(100 * time.Millisecond):
				}
				close(done)
			})
		})
	})

})
