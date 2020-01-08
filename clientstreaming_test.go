package signalr

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"strings"
	"time"
)

var clientStreamingInvocationQueue = make(chan string, 20)

type clientStreamHub struct {
	Hub
}

func (c *clientStreamHub) UploadStreamSmoke(upload1 <-chan int, factor float64, upload2 <-chan float64) {
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
			}
		case u2, ok2 = <-upload2:
			if ok2 {
				clientStreamingInvocationQueue <- fmt.Sprintf("u2: %v", u2)
			}
		default:
		}
		if !ok1 && !ok2 {
			clientStreamingInvocationQueue <- "Finished"
			return
		}
	}
}

var _ = Describe("ClientStreaming", func() {

	Describe("Simple stream invocation", func() {
		conn := connect(&clientStreamHub{})
		Context("When invoked by the client with streamids", func() {
			It("should be invoked on the server, and receive stream items until the caller sends a completion", func() {
				conn.clientSend(fmt.Sprintf(
					`{"type":1,"invocationId":"upstream","target":"uploadstreamsmoke","arguments":[%v],"streamids":["123","456"]}`, 5))
				Expect(<-clientStreamingInvocationQueue).To(Equal(fmt.Sprintf("f: %v", 5)))
				go func() {
					go func() {
						for i := 0; i < 10; i++ {
							conn.clientSend(fmt.Sprintf(`{"type":2,"invocationid":"123","item":%v}`, i))
							time.Sleep(100 * time.Millisecond)
						}
						conn.clientSend(`{"type":3,"invocationid":"123"}`)
					}()
					go func() {
						for i := 5; i < 10; i++ {
							conn.clientSend(fmt.Sprintf(`{"type":2,"invocationid":"456","item":%v}`, float64(i)*7.1))
							time.Sleep(200 * time.Millisecond)
						}
						conn.clientSend(`{"type":3,"invocationid":"456"}`)
					}()
				}()
				u1 := 0
				u2 := 5
			loop:
				for {
					select {
					case r := <-clientStreamingInvocationQueue:
						switch {
						case strings.HasPrefix(r, "u1"):
							Expect(r).To(Equal(fmt.Sprintf("u1: %v", u1)))
							u1++
						case strings.HasPrefix(r, "u2"):
							Expect(r).To(Equal(fmt.Sprintf("u2: %v", float64(u2)*7.1)))
							u2++
						case r == "Finished":
							Expect(u1).To(Equal(10))
							Expect(u2).To(Equal(10))
							break loop
						}
					default:
						if !conn.Connected() {
							Fail("server disconnected")
						}
					}
				}
			})
		})
	})

	PDescribe("Faulty stream invocation", func() {
		conn := connect(&clientStreamHub{})
		Context("When invoked by the client with streamids", func() {
			It("should be invoked on the server, and receive stream items until the caller sends a completion. Unknown streamids should be ignored", func() {
				conn.clientSend(fmt.Sprintf(
					`{"type":1,"invocationId":"upstream","target":"uploadstreamsmoke","arguments":[%v],"streamids":["123","456"]}`, 5))
				Expect(<-clientStreamingInvocationQueue).To(Equal(fmt.Sprintf("f: %v", 5)))
				// close the first stream
				conn.clientSend(`{"type":3,"invocationid":"123"}`)
				// Send one with correct streamid and one with invalid streamid
				conn.clientSend(fmt.Sprintf(`{"type":2,"invocationid":"456","item":%v}`, 7.1))
				conn.clientSend(fmt.Sprintf(`{"type":2,"invocationid":"WRONG_ID","item":%v}`, 8.3))

				conn.clientSend(`{"type":3,"invocationid":"456"}`)
				for r := range clientStreamingInvocationQueue {
					Expect(r).NotTo(Equal(fmt.Sprintf("u2: %v",8.3)))
				}
			})
		})
	})

})
