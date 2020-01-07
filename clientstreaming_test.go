package signalr

import (
	"fmt"
	. "github.com/onsi/ginkgo"
. "github.com/onsi/gomega"
)

var clientStreamingInvocationQueue = make(chan string, 20)

type clientStreamHub struct {
	Hub
}

func (c *clientStreamHub) UploadStream(upload1 <-chan int, factor float64, upload2 <-chan float64) {
	ok1 := true
	ok2 := true
	u1 := 0
	u2 := 0.0
	clientStreamingInvocationQueue <- fmt.Sprintf("f: %v", factor)
	for {
		select {
		case u1, ok1 = <-upload1:
			if ok1 {
				clientStreamingInvocationQueue <- fmt.Sprintf("u1: %v", u1)
			} else if !ok2 {
				clientStreamingInvocationQueue <- "Finished"
				return
			}
		case u2, ok2 = <-upload2:
			if ok2 {
				clientStreamingInvocationQueue <- fmt.Sprintf("u2: %v", u2)
			} else if !ok1 {
				clientStreamingInvocationQueue <- "Finished"
				return
			}
		default:
		}
	}
}


var _ = Describe("ClientStreaming", func() {

	Describe("Simple stream invocation", func() {
		conn := connect(&streamHub{})
		Context("When invoked by the client with streamids", func() {
			It("should be invoked on the server, and receive stream items until the caller sends a completion", func() {
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
})