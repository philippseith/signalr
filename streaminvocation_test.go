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

var _ = Describe("Streaminvocation", func() {

	Describe("Simple stream invocation", func() {
		conn := connect(&streamHub{})
		Context("When invoked by the client", func() {
			It("should be invoked on the server, return stream items and a final completion without result", func() {
				_, err := conn.clientSend(`{"type":4,"invocationId": "zzz","target":"simplestream"}`)
				Expect(err).To(BeNil())
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

	Describe("Stop simple stream invocation", func() {
		conn := connect(&streamHub{})
		Context("When invoked by the client and stop after one result", func() {
			It("should be invoked on the server, return stream one item and a final completion without result", func() {
				_, err := conn.clientSend(`{"type":4,"invocationId": "xxx","target":"simplestream"}`)
				Expect(err).To(BeNil())
				Expect(<-streamInvocationQueue).To(Equal("SimpleStream()"))
				recv := (<-conn.received).(streamItemMessage)
				Expect(recv).NotTo(BeNil())
				Expect(recv.InvocationID).To(Equal("xxx"))
				Expect(recv.Item).To(Equal(float64(1)))
				// stop it
				_, err = conn.clientSend(`{"type":5,"invocationId": "xxx"}`)
				Expect(err).To(BeNil())
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
})
