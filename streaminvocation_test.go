package signalr

import (
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

var _ = Describe("Streaminvocation", func() {

	Describe("Simple stream invocation", func() {
		conn := connect(&streamHub{})
		Context("When invoked by the client", func() {
			It("should be invoked on the server, return stream items and a final completion without result", func() {
				conn.clientSend(`{"type":4,"invocationId": "zzz","target":"simplestream"}`)
				Expect(<-streamInvocationQueue).To(Equal("SimpleStream()"))
				for i := 1; i < 4; i++ {
					recv := (<-conn.received).(streamItemMessage)
					Expect(recv).NotTo(BeNil())
					Expect(recv.InvocationID).To(Equal("zzz"))
					Expect(recv.Item).To(Equal(float64(i)))
				}
				recv := (<-conn.received).(completionMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.InvocationID).To(Equal("zzz"))
				Expect(recv.Result).To(BeNil())
				Expect(recv.Error).To(Equal(""))
			})
		})
	})

	Describe("Slice stream invocation", func() {
		conn := connect(&streamHub{})
		Context("When invoked by the client", func() {
			It("should be invoked on the server, return stream items and a final completion without result", func() {
				conn.clientSend(`{"type":4,"invocationId": "slice","target":"slicestream"}`)
				Expect(<-streamInvocationQueue).To(Equal("SliceStream()"))
				for i := 1; i < 4; i++ {
					recv := (<-conn.received).(streamItemMessage)
					Expect(recv).NotTo(BeNil())
					Expect(recv.InvocationID).To(Equal("slice"))
					exp := make([]interface{}, 0, 2)
					exp = append(exp, float64(i))
					exp = append(exp, float64(i*2))
					Expect(recv.Item).To(Equal(exp))
				}
				recv := (<-conn.received).(completionMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.InvocationID).To(Equal("slice"))
				Expect(recv.Result).To(BeNil())
				Expect(recv.Error).To(Equal(""))
			})
		})
	})

	Describe("Stop simple stream invocation", func() {
		conn := connect(&streamHub{})
		Context("When invoked by the client and stop after one result", func() {
			It("should be invoked on the server, return stream one item and a final completion without result", func() {
				conn.clientSend(`{"type":4,"invocationId": "xxx","target":"simplestream"}`)
				Expect(<-streamInvocationQueue).To(Equal("SimpleStream()"))
				recv := (<-conn.received).(streamItemMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.InvocationID).To(Equal("xxx"))
				Expect(recv.Item).To(Equal(float64(1)))
				// stop it
				conn.clientSend(`{"type":5,"invocationId": "xxx"}`)
				for {
					recv := <-conn.received
					Expect(recv).NotTo(BeNil())
					switch recv.(type) {
					case streamItemMessage:
						srecv := recv.(streamItemMessage)
						Expect(srecv.InvocationID).To(Equal("xxx"))
					case completionMessage:
						crecv := recv.(completionMessage)
						Expect(crecv.InvocationID).To(Equal("xxx"))
						Expect(crecv.Result).To(BeNil())
						Expect(crecv.Error).To(Equal(""))
						return
					}
				}
			})
		})
	})

	Describe("Stream invocation of method with no stream result", func() {
		conn := connect(&streamHub{})
		Context("When invoked by the client", func() {
			It("should be invoked on the server, return one stream item with the \"no stream\" result and a final completion without result", func() {
				conn.clientSend(`{"type":4,"invocationId": "yyy","target":"simpleint"}`)
				Expect(<-streamInvocationQueue).To(Equal("SimpleInt()"))
				sRecv := (<-conn.received).(streamItemMessage)
				Expect(sRecv).NotTo(BeNil())
				Expect(sRecv.InvocationID).To(Equal("yyy"))
				Expect(sRecv.Item).To(Equal(float64(-1)))
				cRecv := (<-conn.received).(completionMessage)
				Expect(cRecv).NotTo(BeNil())
				Expect(cRecv.InvocationID).To(Equal("yyy"))
				Expect(cRecv.Result).To(BeNil())
				Expect(cRecv.Error).To(Equal(""))
			})
		})
	})


})
