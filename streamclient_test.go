package signalr

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type clientStreamHub struct {
	Hub
}

func (c *clientStreamHub) SendResult(result string) {
	c.Hub.Clients().All().Send("OnResult", result)
}

func (c *clientStreamHub) UploadStreamSmoke(upload1 <-chan int, factor float64, upload2 <-chan float64) {
	ok1 := true
	ok2 := true
	u1 := 0
	u2 := 0.0
	c.SendResult(fmt.Sprintf("f: %v", factor))
	for {
		select {
		case u1, ok1 = <-upload1:
			if ok1 {
				c.SendResult(fmt.Sprintf("u1: %v", u1))
			}
		case u2, ok2 = <-upload2:
			if ok2 {
				c.SendResult(fmt.Sprintf("u2: %v", u2))
			}
		case <-c.Hub.Context().Done():
			return
		}
		if !ok1 && !ok2 {
			c.SendResult("Finished")
			return
		}
	}
}

//noinspection GoUnusedParameter
func (c *clientStreamHub) UploadPanic(upload <-chan string) {
	c.SendResult("Panic()")
	panic("Don't panic!")
}

func (c *clientStreamHub) UploadInt(upload <-chan int) {
	c.SendResult("UploadInt()")
	for {
		<-upload
	}
}

//noinspection GoUnusedParameter
func (c *clientStreamHub) UploadHang(upload <-chan int) {
	c.SendResult("UploadHang()")
	// wait forever
	select {}
}

func (c *clientStreamHub) UploadParamTypes(
	u1 <-chan int8, u2 <-chan int16, u3 <-chan int32, u4 <-chan int64,
	u5 <-chan uint, u6 <-chan uint8, u7 <-chan uint16, u8 <-chan uint32, u9 <-chan uint64,
	u10 <-chan float32, u11 <-chan float64,
	u12 <-chan string) {
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
			count++
		case <-u12:
			count++
		case <-time.After(100 * time.Millisecond):
			c.SendResult("timed out")
			return
		}
	}
	c.SendResult("UPT finished")
}

func (c *clientStreamHub) UploadArray(u <-chan []int) {
	for r := range u {
		c.SendResult(fmt.Sprintf("received %v", r))
	}
	c.SendResult("UploadArray finished")
}

func (c *clientStreamHub) UploadError(u <-chan error) {
	c.SendResult("UploadError start")
	for range u {
	}
}

func (c *clientStreamHub) UploadErrorArray(u <-chan []error) {
	c.SendResult("UploadErrorArray start")
	for range u {
	}
}

type resultReceiver struct {
	ch chan string
}

func (r *resultReceiver) OnResult(result string) {
	r.ch <- result
}

func makeStreamingClientAndServer(options ...func(Party) error) (Client, *resultReceiver, context.CancelFunc) {
	cliConn, srvConn := newClientServerConnections()
	ctx, cancel := context.WithCancel(context.Background())
	if options == nil {
		options = make([]func(Party) error, 0)
	}
	options = append(options, SimpleHubFactory(&clientStreamHub{}), testLoggerOption())
	server, _ := NewServer(ctx, options...)
	go func() { _ = server.Serve(srvConn) }()
	receiver := &resultReceiver{ch: make(chan string, 1)}
	client, _ := NewClient(ctx, WithConnection(cliConn), WithReceiver(receiver), testLoggerOption())
	client.Start()
	return client, receiver, cancel
}

var _ = Describe("ClientStreaming", func() {

	Describe("Simple stream invocation", func() {
		Context("When invoked by the client with streamIds", func() {
			It("should be invoked on the server, and receive stream items until the caller sends a completion", func(done Done) {
				client, receiver, cancel := makeStreamingClientAndServer()
				ch1 := make(chan int, 1)
				ch2 := make(chan float64, 1)
				client.PushStreams("UploadStreamSmoke", ch1, 5, ch2)
				go func() {
					go func() {
						for i := 0; i < 10; i++ {
							ch1 <- i
							time.Sleep(100 * time.Millisecond)
						}
					}()
					go func() {
						for i := 5; i < 10; i++ {
							ch2 <- float64(i) * 7.1
							time.Sleep(200 * time.Millisecond)
						}
					}()
				}()
				u1 := 0
				u2 := 5
			loop:
				for {
					select {
					case r := <-receiver.ch:
						switch {
						case strings.HasPrefix(r, "u1"):
							Expect(r).To(Equal(fmt.Sprintf("u1: %v", u1)))
							u1++
							if u1 == 10 {
								close(ch1)
							}
						case strings.HasPrefix(r, "u2"):
							Expect(r).To(Equal(fmt.Sprintf("u2: %v", float64(u2)*7.1)))
							u2++
							if u2 == 10 {
								close(ch2)
							}
						case r == "Finished":
							break loop
						}
					}
				}
				cancel()
				close(done)
			}, 2.0)
		})
	})

	Describe("Stream invocation", func() {
		Context(" When stream item type that could not be converted to the receiving hub methods channel type is sent", func() {
			It("should end the connection with an error", func(done Done) {
				client, _, cancel := makeStreamingClientAndServer()
				ch := make(chan string, 1)
				client.PushStreams("UploadInt", ch)
				ch <- "ShouldBeInt"
				<-client.WaitForState(context.Background(), ClientClosed)
				Expect(client.Err()).To(HaveOccurred())
				cancel()
				close(done)
			}, 2.0)
		})
	})

	Describe("Stream invocation to the StreamBufferCapacity", func() {
		Context(" When hub method is called as streaming receiver but does not handle channel input", func() {
			It("should send a completion with error, but wait at least ChanReceiveTimeout", func(done Done) {
				client, _, cancel := makeStreamingClientAndServer(ChanReceiveTimeout(500*time.Millisecond),
					StreamBufferCapacity(5))
				<-client.WaitForState(context.Background(), ClientConnected)
				ch := make(chan int, 1)
				client.PushStreams("UploadHang", ch)
				// connect() sets StreamBufferCapacity to 5, so 6 messages should be send to make it hang
				for i := 0; i < 6; i++ {
					ch <- i
				}
				sent := time.Now()
				<-client.WaitForState(context.Background(), ClientClosed)
				Expect(client.Err()).To(HaveOccurred())
				// ChanReceiveTimeout 200 ms should be over
				Expect(time.Now().UnixNano()).To(BeNumerically(">", sent.Add(500*time.Millisecond).UnixNano()))
				cancel()
				close(done)
			})
		})
	})
	/*
		Describe("Stream invocation with wrong streamId", func() {
			Context("When invoked by the client with streamIds", func() {
				It("should be invoked on the server, and receive stream items until the caller sends a completion. Unknown streamIds should be ignored", func(done Done) {
					server, conn := connect(&clientStreamHub{})
					conn.ClientSend(fmt.Sprintf(
						`{"type":1,"invocationId":"upstream","target":"uploadstreamsmoke","arguments":[%v],"streamIds":["123","456"]}`, 5))
					Expect(<-clientStreamingInvocationQueue).To(Equal(fmt.Sprintf("f: %v", 5)))
					// close the first stream
					conn.ClientSend(`{"type":3,"invocationId":"123"}`)
					// Send one with correct streamid
					conn.ClientSend(fmt.Sprintf(`{"type":2,"invocationId":"456","item":%v}`, 7.1))
					// close the second stream
					conn.ClientSend(`{"type":3,"invocationId":"456"}`)
				loop:
					for {
						select {
						case <-clientStreamingInvocationQueue:
							break loop
						case <-time.After(500 * time.Millisecond):
							Fail("timed out")
						}
					}
					// Read finished value from queue
					<-clientStreamingInvocationQueue
					server.cancel()
					close(done)
				})
			})
		})

		Describe("Stream invocation with wrong count of streamid", func() {
			var server Server
			var conn *testingConnection
			BeforeEach(func(done Done) {
				server, conn = connect(&clientStreamHub{})
				close(done)
			})
			AfterEach(func(done Done) {
				server.cancel()
				close(done)
			})
			Context("When invoked by the client with to many streamIds", func() {
				It("should end the connection with an error", func(done Done) {
					conn.ClientSend(fmt.Sprintf(`{"type":1,"invocationId":"upstream","target":"uploadstreamsmoke","arguments":[%v],"streamIds":["123","456","789"]}`, 5))
					message := <-conn.received
					Expect(message).To(BeAssignableToTypeOf(completionMessage{}))
					completionMessage := message.(completionMessage)
					Expect(completionMessage.Error).NotTo(BeNil())
					close(done)
				}, 2.0)
			})
			Context("When invoked by the client with not enough streamIds", func() {
				It("should end the connection with an error", func(done Done) {
					conn.ClientSend(fmt.Sprintf(`{"type":1,"invocationId":"upstream","target":"uploadstreamsmoke","arguments":[%v],"streamIds":["123"]}`, 5))
					message := <-conn.received
					Expect(message).To(BeAssignableToTypeOf(completionMessage{}))
					completionMessage := message.(completionMessage)
					Expect(completionMessage.Error).NotTo(BeNil())
					close(done)
				}, 2.0)
			})
		})

		Describe("Panic in invoked stream client func", func() {
			Context("When a func is invoked by the client and panics", func() {
				It("should return a completion with error", func(done Done) {
					server, conn := connect(&clientStreamHub{})
					conn.ClientSend(`{"type":1,"invocationId":"upstreampanic","target":"uploadpanic","streamIds":["123"]}`)
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
					server.cancel()
					close(done)
				})
			})
		})

		Describe("Stream client with all numbertypes of channels", func() {
			var server Server
			var conn *testingConnection
			BeforeEach(func(done Done) {
				server, conn = connect(&clientStreamHub{})
				close(done)
			})
			AfterEach(func(done Done) {
				server.cancel()
				close(done)
			})
			Context("When a func is invoked by the client with all number types of channels", func() {
				It("should receive values on all of these types", func(done Done) {
					conn.ClientSend(`{"type":1,"invocationId":"UPT","target":"uploadparamtypes","streamIds":["1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"]}`)
					conn.ClientSend(`{"type":2,"invocationId":"1","item":1}`)
					conn.ClientSend(`{"type":2,"invocationId":"2","item":1}`)
					conn.ClientSend(`{"type":2,"invocationId":"3","item":1}`)
					conn.ClientSend(`{"type":2,"invocationId":"4","item":1}`)
					conn.ClientSend(`{"type":2,"invocationId":"5","item":1}`)
					conn.ClientSend(`{"type":2,"invocationId":"6","item":1}`)
					conn.ClientSend(`{"type":2,"invocationId":"7","item":1}`)
					conn.ClientSend(`{"type":2,"invocationId":"8","item":1}`)
					conn.ClientSend(`{"type":2,"invocationId":"9","item":1}`)
					conn.ClientSend(`{"type":2,"invocationId":"10","item":1.1}`)
					conn.ClientSend(`{"type":2,"invocationId":"11","item":2.1}`)
					conn.ClientSend(`{"type":2,"invocationId":"12","item":"Some String"}`)
					r := <-clientStreamingInvocationQueue
					Expect(r).To(Equal("UPT finished"))
					close(done)
				})
			})
		})

		Describe("Stream client with array channel", func() {
			var server Server
			var conn *testingConnection
			BeforeEach(func(done Done) {
				server, conn = connect(&clientStreamHub{})
				close(done)
			})
			AfterEach(func(done Done) {
				server.cancel()
				close(done)
			})
			Context("When a func with an array channel is invoked by the client and stream items are send", func() {
				It("should receive values and end after that", func(done Done) {
					conn.ClientSend(`{"type":1,"invocationId":"UPA","target":"uploadarray","streamIds":["aaa"]}`)
					conn.ClientSend(`{"type":2,"invocationId":"aaa","item":[1,2]}`)
					conn.ClientSend(`{"type":3,"invocationId":"aaa"}`)
					Expect(<-clientStreamingInvocationQueue).To(Equal("received [1 2]"))
					Expect(<-clientStreamingInvocationQueue).To(Equal("UploadArray finished"))
					close(done)
				})
			})
		})

		Describe("Client sending invalid streamitems", func() {
			var server Server
			var conn *testingConnection
			BeforeEach(func(done Done) {
				server, conn = connect(&clientStreamHub{})
				close(done)
			})
			AfterEach(func(done Done) {
				server.cancel()
				close(done)
			})
			Context("When an invalid streamitem message with missing id and item is sent", func() {
				It("should end the connection with an error", func(done Done) {
					conn.ClientSend(`{"type":4,"invocationId": "nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamIds":["ff2","ggg"]}`)
					<-clientStreamingInvocationQueue
					// Send invalid stream item message with missing id and item
					conn.ClientSend(`{"type":2}`)
					message := <-conn.received
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
					close(done)
				}, 2.0)
			})
			Context("When an invalid streamitem message with missing item is received", func() {
				var server Server
				var conn *testingConnection
				BeforeEach(func(done Done) {
					server, conn = connect(&clientStreamHub{})
					close(done)
				})
				AfterEach(func(done Done) {
					server.cancel()
					close(done)
				})
				It("should end the connection with an error", func(done Done) {
					conn.ClientSend(`{"type":4,"invocationId": "nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamIds":["ff3","ggg"]}`)
					<-clientStreamingInvocationQueue
					// Send invalid stream item message with missing item
					conn.ClientSend(`{"type":2,"InvocationId":"iii"}`)
					message := <-conn.received
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
					close(done)
				}, 2.0)
			})
			Context("When an invalid streamitem message with wrong itemtype is received", func() {
				var server Server
				var conn *testingConnection
				BeforeEach(func(done Done) {
					server, conn = connect(&clientStreamHub{})
					close(done)
				})
				AfterEach(func(done Done) {
					server.cancel()
					close(done)
				})
				It("should end the connection with an error", func(done Done) {
					conn.ClientSend(`{"type":4,"invocationId": "nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamIds":["ff1","ggg"]}`)
					<-clientStreamingInvocationQueue
					// Send invalid stream item message
					conn.ClientSend(`{"type":2,"invocationId":"ff1","item":[42]}`)
					message := <-conn.received
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
					close(done)
				}, 2.0)
			})
			Context("When an invalid stream item message with invalid invocation id is sent", func() {
				var server Server
				var conn *testingConnection
				BeforeEach(func(done Done) {
					server, conn = connect(&clientStreamHub{})
					close(done)
				})
				AfterEach(func(done Done) {
					server.cancel()
					close(done)
				})
				It("should end the connection with an error", func(done Done) {
					conn.ClientSend(`{"type":4,"invocationId": "nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamIds":["ff4","ggg"]}`)
					<-clientStreamingInvocationQueue
					// Send invalid stream item message with invalid invocation id
					conn.ClientSend(`{"type":2,"invocationId":1}`)
					message := <-conn.received
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
					close(done)
				}, 2.0)
			})
		})

		Describe("Client sending invalid completion", func() {
			var server Server
			var conn *testingConnection
			BeforeEach(func() {
				server, conn = connect(&clientStreamHub{})
			})
			AfterEach(func() {
				server.cancel()
			})
			Context("When an invalid completion message with missing id is sent", func() {
				It("should end the connection with an error", func(done Done) {
					conn.ClientSend(`{"type":4,"invocationId": "nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamIds":["ff5","ggg"]}`)
					<-clientStreamingInvocationQueue
					// Send invalid completion message with missing id
					conn.ClientSend(`{"type":3}`)
					message := <-conn.received
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
					close(done)
				}, 2.0)
			})

			Context("When an invalid completion message with unknown id is sent", func() {
				It("should end the connection with an error", func(done Done) {
					conn.ClientSend(`{"type":4,"invocationId":"nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamIds":["ff6","ggg"]}`)
					<-clientStreamingInvocationQueue
					// Send invalid completion message with unknown id
					conn.ClientSend(`{"type":3,"invocationId":"qqq"}`)
					message := <-conn.received
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
					close(done)
				}, 2.0)
			})

			Context("When an completion message with an result is sent before any stream item was received", func() {
				It("should take the result as stream item and consider the streaming as finished", func(done Done) {
					conn.ClientSend(`{"type":1,"invocationId":"UPA","target":"uploadarray","streamIds":["aaa"]}`)
					conn.ClientSend(`{"type":3,"invocationId":"aaa","result":[1,2]}`)
					Expect(<-clientStreamingInvocationQueue).To(Equal("received [1 2]"))
					Expect(<-clientStreamingInvocationQueue).To(Equal("UploadArray finished"))
					close(done)
				})
			})

			Context("When an invalid completion message is sent", func() {
				It("should close the connection with an error", func(done Done) {
					conn.ClientSend(`{"type":1,"invocationId":"UPA","target":"uploadarray","streamIds":["aaa"]}`)
					conn.ClientSend(`{"type":3,"invocationId":1}`)
					message := <-conn.received
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
					close(done)
				}, 2.0)
			})

			Context("When an completion message with an result is sent after a stream item was received", func() {
				It("should end the connection with an error", func(done Done) {
					conn.ClientSend(`{"type":4,"invocationId":"nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamIds":["fff","ggg"]}`)
					<-clientStreamingInvocationQueue
					// Send stream item
					conn.ClientSend(`{"type":2,"invocationId":"fff","item":1}`)
					<-clientStreamingInvocationQueue
					// Send completion message with result
					conn.ClientSend(`{"type":3,"invocationId":"fff","result":1}`)
					message := <-conn.received
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
					close(done)
				})
			})

			Context("When the stream item type could not converted to the hub methods receive channel type", func() {
				It("should end the connection with an error", func(done Done) {
					conn.ClientSend(`{"type":4,"invocationId":"nnn","target":"uploaderror","streamIds":["eee"]}`)
					<-clientStreamingInvocationQueue
					// Send stream item
					conn.ClientSend(`{"type":2,"invocationId":"eee","item":1}`)
					message := <-conn.received
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
					close(done)
				})
			})

			Context("When the stream item array type could not converted to the hub methods receive channel array type", func() {
				It("should end the connection with an error", func(done Done) {
					conn.ClientSend(`{"type":4,"invocationId":"nnn","target":"uploaderrorarray","streamIds":["aeae"]}`)
					<-clientStreamingInvocationQueue
					// Send stream item
					conn.ClientSend(`{"type":2,"invocationId":"aeae","item":[7,8]}`)
					message := <-conn.received
					Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
					Expect(message.(closeMessage).Error).NotTo(BeNil())
					close(done)
				})
			})
		})
	*/
})
