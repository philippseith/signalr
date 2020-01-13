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

//noinspection GoUnusedParameter
func (c *clientStreamHub) UploadPanic(upload <-chan string) {
	clientStreamingInvocationQueue <- "Panic()"
	panic("Don't panic!")
}

func (c *clientStreamHub) UploadInt(upload <-chan int) {
	clientStreamingInvocationQueue <- "UploadInt()"
	for {
		<-upload
	}
}

//noinspection GoUnusedParameter
func (c *clientStreamHub) UploadHang(upload <-chan int) {
	clientStreamingInvocationQueue <- "UploadHang()"
	// wait forever
	select {}
}

func (c *clientStreamHub) UploadParamTypes(
	u1 <-chan int8, u2 <-chan int16, u3 <-chan int32, u4 <-chan int64,
	u5 <-chan uint, u6 <-chan uint8, u7 <-chan uint16, u8 <-chan uint32, u9 <-chan uint64,
	u10 <-chan float32,
	u11 <-chan string) {
	for count := 0; count < 12; {
		select {
		case <-u1:
			count++
		case <-u2:
			count++
		case <-u3:
			count++
		case <-u4:
			count++
		case <-u5:
			count++
		case <-u6:
			count++
		case <-u7:
			count++
		case <-u8:
			count++
		case <-u9:
			count++
		case <-u10:
			count++
		case <-u11:
			count++ // will be called twice
		case <-time.After(100 * time.Millisecond):
			clientStreamingInvocationQueue <- "timed out"
			return
		}
	}
	clientStreamingInvocationQueue <- "UPT finished"
}

func (c *clientStreamHub) UploadArray(u <-chan []int) {
	for r := range u {
		clientStreamingInvocationQueue <- fmt.Sprintf("received %v", r)
	}
	clientStreamingInvocationQueue <- "UploadArray finished"
}

func (c *clientStreamHub) UploadError(u <-chan error) {
	clientStreamingInvocationQueue <- "UploadError start"
	for range u {
	}
}

func (c *clientStreamHub) UploadErrorArray(u <-chan []error) {
	clientStreamingInvocationQueue <- "UploadErrorArray start"
	for range u {
	}
}

var _ = Describe("ClientStreaming", func() {

	Describe("Simple stream invocation", func() {
		Context("When invoked by the client with streamids", func() {
			It("should be invoked on the server, and receive stream items until the caller sends a completion", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(fmt.Sprintf(
					`{"type":1,"invocationId":"upstream","target":"uploadstreamsmoke","arguments":[%v],"streamids":["123","456"]}`, 5))
				Expect(<-clientStreamingInvocationQueue).To(Equal(fmt.Sprintf("f: %v", 5)))
				go func() {
					go func() {
						for i := 0; i < 10; i++ {
							conn.ClientSend(fmt.Sprintf(`{"type":2,"invocationid":"123","item":%v}`, i))
							time.Sleep(100 * time.Millisecond)
						}
						conn.ClientSend(`{"type":3,"invocationid":"123"}`)
					}()
					go func() {
						for i := 5; i < 10; i++ {
							conn.ClientSend(fmt.Sprintf(`{"type":2,"invocationid":"456","item":%v}`, float64(i)*7.1))
							time.Sleep(200 * time.Millisecond)
						}
						conn.ClientSend(`{"type":3,"invocationid":"456"}`)
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

	Describe("Stream invocation", func() {
		Context(" When stream item type that could not be converted to the receiving hub methods channel type is sent", func() {
			It("should end the connection with an error", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(`{"type":1,"invocationId":"upstream","target":"uploadint","streamids":["abc"]}`)
				Expect(<-clientStreamingInvocationQueue).To(Equal("UploadInt()"))
				conn.ClientSend(`{"type":2,"invocationid":"abc","item":"ShouldBeInt"}`)
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
				case <-time.After(500 * time.Millisecond):
					Fail("timed out")
				}
			})
		})
	})

	Describe("Stream invocation", func() {
		Context(" When hub method is called as streaming receiver but does not handle channel input", func() {
			It("should send a completion with error", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(`{"type":1,"invocationId":"upstream","target":"uploadhang","streamids":["hang"]}`)
				Expect(<-clientStreamingInvocationQueue).To(Equal("UploadHang()"))
				conn.ClientSend(`{"type":2,"invocationid":"hang","item":1}`)
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(completionMessage{}))
					Expect(message.(completionMessage).Error).NotTo(BeNil())
				case <-time.After(500 * time.Millisecond):
					Fail("timed out")
				}
			})
		})
	})

	Describe("Stream invocation with wrong streamid", func() {
		Context("When invoked by the client with streamids", func() {
			It("should be invoked on the server, and receive stream items until the caller sends a completion. Unknown streamids should be ignored", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(fmt.Sprintf(
					`{"type":1,"invocationId":"upstream","target":"uploadstreamsmoke","arguments":[%v],"streamids":["123","456"]}`, 5))
				Expect(<-clientStreamingInvocationQueue).To(Equal(fmt.Sprintf("f: %v", 5)))
				// close the first stream
				conn.ClientSend(`{"type":3,"invocationid":"123"}`)
				// Send one with correct streamid
				conn.ClientSend(fmt.Sprintf(`{"type":2,"invocationid":"456","item":%v}`, 7.1))
				// close the second stream
				conn.ClientSend(`{"type":3,"invocationid":"456"}`)
			loop:
				for {
					select {
					case <-clientStreamingInvocationQueue:
						break loop
					case <-time.After(100 * time.Millisecond):
						Fail("timed out")
					}
				}
				// Read finished value from queue
				<-clientStreamingInvocationQueue
			})
		})
	})

	Describe("Stream invocation with wrong count of streamid", func() {

		Context("When invoked by the client with to many streamids", func() {
			It("should end the connection with an error", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(fmt.Sprintf(`{"type":1,"invocationId":"upstream","target":"uploadstreamsmoke","arguments":[%v],"streamids":["123","456","789"]}`, 5))
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(completionMessage{}))
					completionMessage := message.(completionMessage)
					Expect(completionMessage.Error).NotTo(BeNil())
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
			})
		})
		Context("When invoked by the client with not enough streamids", func() {
			It("should end the connection with an error", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(fmt.Sprintf(`{"type":1,"invocationId":"upstream","target":"uploadstreamsmoke","arguments":[%v],"streamids":["123"]}`, 5))
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(completionMessage{}))
					completionMessage := message.(completionMessage)
					Expect(completionMessage.Error).NotTo(BeNil())
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
			})
		})
	})

	Describe("Panic in invoked stream client func", func() {
		Context("When a func is invoked by the client and panics", func() {
			It("should return a completion with error", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(`{"type":1,"invocationId":"upstreampanic","target":"uploadpanic","streamids":["123"]}`)
				wasInvoked := false
				gotMsg := false
			loop:
				for {
					select {
					case r := <-clientStreamingInvocationQueue:
						Expect(r).To(Equal("Panic()"))
						wasInvoked = true
						if wasInvoked && gotMsg {
							break loop
						}
					case message := <-conn.received:
						completionMessage, ok := message.(completionMessage)
						Expect(ok).To(BeTrue())
						Expect(completionMessage.Error).NotTo(Equal(""))
						gotMsg = true
						if wasInvoked && gotMsg {
							break loop
						}
					case <-time.After(100 * time.Millisecond):
						Fail("timed out")
						break loop
					}
				}
			})
		})
	})

	Describe("Stream client with all numbertypes of channels", func() {
		Context("When a func is invoked by the client with all number types of channels", func() {
			It("should receive values on all of these types", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(`{"type":1,"invocationId":"UPT","target":"uploadparamtypes","streamids":["1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"]}`)
				conn.ClientSend(`{"type":2,"invocationid":"1","item":1}`)
				conn.ClientSend(`{"type":2,"invocationid":"2","item":1}`)
				conn.ClientSend(`{"type":2,"invocationid":"3","item":1}`)
				conn.ClientSend(`{"type":2,"invocationid":"4","item":1}`)
				conn.ClientSend(`{"type":2,"invocationid":"5","item":1}`)
				conn.ClientSend(`{"type":2,"invocationid":"6","item":1}`)
				conn.ClientSend(`{"type":2,"invocationid":"7","item":1}`)
				conn.ClientSend(`{"type":2,"invocationid":"8","item":1}`)
				conn.ClientSend(`{"type":2,"invocationid":"9","item":1}`)
				conn.ClientSend(`{"type":2,"invocationid":"10","item":1.1}`)
				conn.ClientSend(`{"type":2,"invocationid":"11","item":2.1}`)
				conn.ClientSend(`{"type":2,"invocationid":"11","item":"Some String"}`)
				select {
				case r := <-clientStreamingInvocationQueue:
					Expect(r).To(Equal("UPT finished"))
				case <-time.After(100 * time.Millisecond):
					Fail("timed out")
				}
			})
		})
	})

	Describe("Stream client with array channel", func() {
		Context("When a func with an array channel is invoked by the client and stream items are send", func() {
			It("should receive values and end after that", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(`{"type":1,"invocationId":"UPA","target":"uploadarray","streamids":["aaa"]}`)
				conn.ClientSend(`{"type":2,"invocationId":"aaa","item":[1,2]}`)
				conn.ClientSend(`{"type":3,"invocationId":"aaa"}`)
				Expect(<-clientStreamingInvocationQueue).To(Equal("received [1 2]"))
				Expect(<-clientStreamingInvocationQueue).To(Equal("UploadArray finished"))
			})
		})
	})

	Describe("Client sending invalid streamitems", func() {
		Context("When an invalid streamitem message with missing id and item is sent", func() {
			It("should end the connection with an error", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(`{"type":4,"invocationId": "nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamids":["fff","ggg"]}`)
				<-clientStreamingInvocationQueue
				// Send invalid stream item message with missing id and item
				conn.ClientSend(`{"type":2}`)
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
			})
		})
		Context("When an invalid streamitem message with missing item is received", func() {
			It("should end the connection with an error", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(`{"type":4,"invocationId": "nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamids":["fff","ggg"]}`)
				<-clientStreamingInvocationQueue
				// Send invalid stream item message with missing item
				conn.ClientSend(`{"type":2,"InvocationId":"iii"}`)
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
			})
		})
		Context("When an invalid streamitem message with wrong itemtype is received", func() {
			It("should end the connection with an error", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(`{"type":4,"invocationId": "nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamids":["fff","ggg"]}`)
				<-clientStreamingInvocationQueue
				// Send invalid stream item message
				conn.ClientSend(`{"type":2,"invocationid":"fff","item":[42]}`)
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
			})
		})
		Context("When an invalid streamitem message with invalid invocation id is sent", func() {
			It("should end the connection with an error", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(`{"type":4,"invocationId": "nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamids":["fff","ggg"]}`)
				<-clientStreamingInvocationQueue
				// Send invalid stream item message with invalid invocation id
				conn.ClientSend(`{"type":2,"invocationId":1}`)
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
			})
		})
	})

	Describe("Client sending invalid completion", func() {
		Context("When an invalid completion message with missing id is sent", func() {
			It("should end the connection with an error", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(`{"type":4,"invocationId": "nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamids":["fff","ggg"]}`)
				<-clientStreamingInvocationQueue
				// Send invalid completion message with missing id
				conn.ClientSend(`{"type":3}`)
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
			})
		})

		Context("When an invalid completion message with unknown id is sent", func() {
			It("should end the connection with an error", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(`{"type":4,"invocationId":"nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamids":["fff","ggg"]}`)
				<-clientStreamingInvocationQueue
				// Send invalid completion message with unknown id
				conn.ClientSend(`{"type":3,"invocationId":"qqq"}`)
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
				case <-time.After(1000 * time.Millisecond):
					Fail("timed out")
				}
			})
		})

		Context("When an completion message with an result is sent before any stream item was received", func() {
			It("should take the result as stream item and consider the streaming as finished", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(`{"type":1,"invocationId":"UPA","target":"uploadarray","streamids":["aaa"]}`)
				conn.ClientSend(`{"type":3,"invocationId":"aaa","result":[1,2]}`)
				Expect(<-clientStreamingInvocationQueue).To(Equal("received [1 2]"))
				Expect(<-clientStreamingInvocationQueue).To(Equal("UploadArray finished"))
			})
		})

		Context("When an invalid completion message is sent", func() {
			It("should close the connection with an error", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(`{"type":1,"invocationId":"UPA","target":"uploadarray","streamids":["aaa"]}`)
				conn.ClientSend(`{"type":3,"invocationId":1}`)
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
				case <-time.After(100 * time.Millisecond):
					Fail("timed out")
				}
			})
		})

		Context("When an completion message with an result is sent after a stream item was received", func() {
			It("should end the connection with an error", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(`{"type":4,"invocationId":"nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamids":["fff","ggg"]}`)
				<-clientStreamingInvocationQueue
				// Send stream item
				conn.ClientSend(`{"type":2,"invocationId":"fff","item":1}`)
				// Send completion message with result
				conn.ClientSend(`{"type":3,"invocationId":"fff","result":1}`)
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
				case <-time.After(100 * time.Millisecond):
					Fail("timed out")
				}
			})
		})

		Context("When the stream item type could not converted to the hub methods receive channel type", func() {
			It("should end the connection with an error", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(`{"type":4,"invocationId":"nnn","target":"uploaderror","streamids":["eee"]}`)
				<-clientStreamingInvocationQueue
				// Send stream item
				conn.ClientSend(`{"type":2,"invocationId":"eee","item":1}`)
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
				case <-time.After(100 * time.Millisecond):
					Fail("timed out")
				}
			})
		})

		Context("When the stream item array type could not converted to the hub methods receive channel array type", func() {
			It("should end the connection with an error", func() {
				conn := connect(&clientStreamHub{})
				conn.ClientSend(`{"type":4,"invocationId":"nnn","target":"uploaderrorarray","streamids":["aeae"]}`)
				<-clientStreamingInvocationQueue
				// Send stream item
				conn.ClientSend(`{"type":2,"invocationId":"aeae","item":[7,8]}`)
				select {
				case message := <-conn.received:
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
				case <-time.After(100 * time.Millisecond):
					Fail("timed out")
				}
			})
		})
	})
})
