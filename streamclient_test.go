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
	ch chan string
}

func (c *clientStreamHub) SendResult(result string) {
	if c.ch != nil {
		c.ch <- result
	}
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
			for i := 0; i < 1; i++ {
				j := i
				It(fmt.Sprintf("should end the connection with an error %v", j), func(done Done) {
					client, _, cancel := makeStreamingClientAndServer()
					ch := make(chan string, 1)
					client.PushStreams("UploadInt", ch)
					ch <- fmt.Sprintf("ShouldBeInt %v", j)
					<-client.WaitForState(context.Background(), ClientClosed)
					Expect(client.Err()).To(HaveOccurred())
					cancel()
					close(done)
				}, 2.0)
			}
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

	Describe("Stream invocation with wrong streamId", func() {
		Context("When invoked by the client with streamIds", func() {
			It("should be invoked on the server, and receive stream items until the caller sends a completion. Unknown streamIds should be ignored", func(done Done) {
				hub := &clientStreamHub{ch: make(chan string, 20)}
				server, err := NewServer(context.TODO(), HubFactory(func() HubInterface {
					return hub
				}), testLoggerOption())
				Expect(err).NotTo(HaveOccurred())
				conn := newTestingConnectionForServer()
				go func() { _ = server.Serve(conn) }()

				conn.ClientSend(fmt.Sprintf(
					`{"type":1,"invocationId":"upstream","target":"uploadstreamsmoke","arguments":[%v],"streamIds":["123","456"]}`, 5))
				Expect(<-hub.ch).To(Equal(fmt.Sprintf("f: %v", 5)))
				// close the first stream
				conn.ClientSend(`{"type":3,"invocationId":"123"}`)
				// Send one with correct streamid
				conn.ClientSend(fmt.Sprintf(`{"type":2,"invocationId":"456","item":%v}`, 7.1))
				// close the second stream
				conn.ClientSend(`{"type":3,"invocationId":"456"}`)
			loop:
				for {
					select {
					case <-hub.ch:
						break loop
					case <-time.After(500 * time.Millisecond):
						Fail("timed out")
					}
				}
				// Read finished value from queue
				<-hub.ch
				server.cancel()
				close(done)
			})
		})
	})
	Describe("Stream invocation with wrong count of streamid", func() {

		Context("When invoked by the client with to many streamIds", func() {
			It("should end the connection with an error", func(done Done) {
				server, conn := connect(&clientStreamHub{})
				conn.ClientSend(fmt.Sprintf(`{"type":1,"invocationId":"upstream","target":"uploadstreamsmoke","arguments":[%v],"streamIds":["123","456","789"]}`, 5))
				message := <-conn.received
				Expect(message).To(BeAssignableToTypeOf(completionMessage{}))
				completionMessage := message.(completionMessage)
				Expect(completionMessage.Error).NotTo(BeNil())
				server.cancel()
				close(done)
			}, 2.0)
		})
		Context("When invoked by the client with not enough streamIds", func() {
			It("should end the connection with an error", func(done Done) {
				server, conn := connect(&clientStreamHub{})
				conn.ClientSend(fmt.Sprintf(`{"type":1,"invocationId":"upstream","target":"uploadstreamsmoke","arguments":[%v],"streamIds":["123"]}`, 5))
				message := <-conn.received
				Expect(message).To(BeAssignableToTypeOf(completionMessage{}))
				completionMessage := message.(completionMessage)
				Expect(completionMessage.Error).NotTo(BeNil())
				server.cancel()
				close(done)
			}, 2.0)
		})
	})

	Describe("Panic in invoked stream client func", func() {
		Context("When a func is invoked by the client and panics", func() {
			It("should return a completion with error", func(done Done) {
				client, _, cancel := makeStreamingClientAndServer()
				client.PushStreams("UploadPanic", make(chan int))
				<-client.WaitForState(context.Background(), ClientClosed)
				Expect(client.Err()).To(HaveOccurred())
				cancel()
				close(done)
			})
		})
	})
	Describe("Stream client with all numbertypes of channels", func() {
		Context("When a func is invoked by the client with all number types of channels", func() {
			It("should receive values on all of these types", func(done Done) {
				u1 := make(chan int8, 1)
				u2 := make(chan int16, 1)
				u3 := make(chan int32, 1)
				u4 := make(chan int64, 1)
				u5 := make(chan uint, 1)
				u6 := make(chan uint8, 1)
				u7 := make(chan uint16, 1)
				u8 := make(chan uint32, 1)
				u9 := make(chan uint64, 1)
				u10 := make(chan float32, 1)
				u11 := make(chan float64, 1)
				u12 := make(chan string, 1)
				client, receiver, cancel := makeStreamingClientAndServer()
				client.PushStreams("UploadParamTypes", u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12)
				u1 <- 1
				u2 <- 1
				u3 <- 1
				u4 <- 1
				u5 <- 1
				u6 <- 1
				u7 <- 1
				u8 <- 1
				u9 <- 1
				u10 <- 1.1
				u11 <- 2.1
				u12 <- "Some String"
				Expect(<-receiver.ch).To(Equal("UPT finished"))
				cancel()
				close(done)
			})
		})
	})

	Describe("Stream client with array channel", func() {
		Context("When a func with an array channel is invoked by the client and stream items are send", func() {
			It("should receive values and end after that", func(done Done) {
				client, receiver, cancel := makeStreamingClientAndServer()
				ch := make(chan []int)
				client.PushStreams("UploadArray", ch)
				ch <- []int{1, 2}
				Expect(<-receiver.ch).To(Equal("received [1 2]"))
				close(ch)
				Expect(<-receiver.ch).To(Equal("UploadArray finished"))
				cancel()
				close(done)
			})
		})
	})

	Describe("Client sending invalid streamitems", func() {
		Context("When an invalid streamitem message with missing id and item is sent", func() {
			It("should end the connection with an error", func(done Done) {
				hub := &clientStreamHub{ch: make(chan string, 20)}
				server, err := NewServer(context.TODO(), HubFactory(func() HubInterface {
					return hub
				}), testLoggerOption())
				Expect(err).NotTo(HaveOccurred())
				conn := newTestingConnectionForServer()
				go func() { _ = server.Serve(conn) }()
				conn.ClientSend(`{"type":4,"invocationId": "nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamIds":["ff2","ggg"]}`)
				<-hub.ch
				<-conn.received
				// Send invalid stream item message with missing id and item
				conn.ClientSend(`{"type":2}`)
				message := <-conn.received
				Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
				Expect(message.(closeMessage).Error).NotTo(BeNil())
				server.cancel()
				close(done)
			}, 2.0)
		})
		Context("When an invalid streamitem message with missing item is received", func() {
			It("should end the connection with an error", func(done Done) {
				hub := &clientStreamHub{ch: make(chan string, 20)}
				server, err := NewServer(context.TODO(), HubFactory(func() HubInterface {
					return hub
				}), testLoggerOption())
				Expect(err).NotTo(HaveOccurred())
				conn := newTestingConnectionForServer()
				go func() { _ = server.Serve(conn) }()
				conn.ClientSend(`{"type":4,"invocationId": "nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamIds":["ff3","ggg"]}`)
				<-hub.ch
				<-conn.received
				// Send invalid stream item message with missing item
				conn.ClientSend(`{"type":2,"InvocationId":"iii"}`)
				message := <-conn.received
				Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
				Expect(message.(closeMessage).Error).NotTo(BeNil())
				server.cancel()
				close(done)
			}, 2.0)
		})
		Context("When an invalid streamitem message with wrong itemtype is received", func() {
			It("should end the connection with an error", func(done Done) {
				hub := &clientStreamHub{ch: make(chan string, 20)}
				server, err := NewServer(context.TODO(), HubFactory(func() HubInterface {
					return hub
				}), testLoggerOption())
				Expect(err).NotTo(HaveOccurred())
				conn := newTestingConnectionForServer()
				go func() { _ = server.Serve(conn) }()
				conn.ClientSend(`{"type":4,"invocationId": "nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamIds":["ff1","ggg"]}`)
				<-hub.ch
				<-conn.received
				// Send invalid stream item message
				conn.ClientSend(`{"type":2,"invocationId":"ff1","item":[42]}`)
				message := <-conn.received
				Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
				Expect(message.(closeMessage).Error).NotTo(BeNil())
				server.cancel()
				close(done)
			}, 2.0)
		})
		Context("When an invalid stream item message with invalid invocation id is sent", func() {
			It("should end the connection with an error", func(done Done) {
				hub := &clientStreamHub{ch: make(chan string, 20)}
				server, err := NewServer(context.TODO(), HubFactory(func() HubInterface {
					return hub
				}), testLoggerOption())
				Expect(err).NotTo(HaveOccurred())
				conn := newTestingConnectionForServer()
				go func() { _ = server.Serve(conn) }()
				conn.ClientSend(`{"type":4,"invocationId": "nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamIds":["ff4","ggg"]}`)
				<-hub.ch
				<-conn.received
				// Send invalid stream item message with invalid invocation id
				conn.ClientSend(`{"type":2,"invocationId":1}`)
				message := <-conn.received
				Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
				Expect(message.(closeMessage).Error).NotTo(BeNil())
				server.cancel()
				close(done)
			}, 2.0)
		})
	})

	Describe("Client sending invalid completion", func() {
		Context("When an invalid completion message with missing id is sent", func() {
			It("should end the connection with an error", func(done Done) {
				hub := &clientStreamHub{ch: make(chan string, 20)}
				server, err := NewServer(context.TODO(), HubFactory(func() HubInterface {
					return hub
				}), testLoggerOption())
				Expect(err).NotTo(HaveOccurred())
				conn := newTestingConnectionForServer()
				go func() { _ = server.Serve(conn) }()
				conn.ClientSend(`{"type":4,"invocationId": "nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamIds":["ff5","ggg"]}`)
				<-hub.ch
				<-conn.received
				// Send invalid completion message with missing id
				conn.ClientSend(`{"type":3}`)
				message := <-conn.received
				Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
				Expect(message.(closeMessage).Error).NotTo(BeNil())
				server.cancel()
				close(done)
			}, 2.0)
		})

		Context("When an invalid completion message with unknown id is sent", func() {
			It("should end the connection with an error", func(done Done) {
				hub := &clientStreamHub{ch: make(chan string, 20)}
				server, err := NewServer(context.TODO(), HubFactory(func() HubInterface {
					return hub
				}), testLoggerOption())
				Expect(err).NotTo(HaveOccurred())
				conn := newTestingConnectionForServer()
				go func() { _ = server.Serve(conn) }()
				conn.ClientSend(`{"type":4,"invocationId":"nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamIds":["ff6","ggg"]}`)
				<-hub.ch
				<-conn.received
				// Send invalid completion message with unknown id
				conn.ClientSend(`{"type":3,"invocationId":"qqq"}`)
				message := <-conn.received
				Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
				Expect(message.(closeMessage).Error).NotTo(BeNil())
				server.cancel()
				close(done)
			}, 2.0)
		})

		Context("When an completion message with an result is sent before any stream item was received", func() {
			It("should take the result as stream item and consider the streaming as finished", func(done Done) {
				hub := &clientStreamHub{ch: make(chan string, 20)}
				server, err := NewServer(context.TODO(), HubFactory(func() HubInterface {
					return hub
				}), testLoggerOption())
				Expect(err).NotTo(HaveOccurred())
				conn := newTestingConnectionForServer()
				go func() { _ = server.Serve(conn) }()
				conn.ClientSend(`{"type":1,"invocationId":"UPA","target":"uploadarray","streamIds":["aaa"]}`)
				conn.ClientSend(`{"type":3,"invocationId":"aaa","result":[1,2]}`)
				Expect(<-hub.ch).To(Equal("received [1 2]"))
				Expect(<-hub.ch).To(Equal("UploadArray finished"))
				server.cancel()
				close(done)
			})
		})

		Context("When an invalid completion message is sent", func() {
			It("should close the connection with an error", func(done Done) {
				hub := &clientStreamHub{ch: make(chan string, 20)}
				server, err := NewServer(context.TODO(), HubFactory(func() HubInterface {
					return hub
				}), testLoggerOption())
				Expect(err).NotTo(HaveOccurred())
				conn := newTestingConnectionForServer()
				go func() { _ = server.Serve(conn) }()
				conn.ClientSend(`{"type":1,"invocationId":"UPA","target":"uploadarray","streamIds":["aaa"]}`)
				conn.ClientSend(`{"type":3,"invocationId":1}`)
				message := <-conn.received
				Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
				Expect(message.(closeMessage).Error).NotTo(BeNil())
				server.cancel()
				close(done)
			}, 2.0)
		})

		Context("When an completion message with an result is sent after a stream item was received", func() {
			It("should end the connection with an error", func(done Done) {
				hub := &clientStreamHub{ch: make(chan string, 20)}
				server, err := NewServer(context.TODO(), HubFactory(func() HubInterface {
					return hub
				}), testLoggerOption())
				Expect(err).NotTo(HaveOccurred())
				conn := newTestingConnectionForServer()
				go func() { _ = server.Serve(conn) }()
				conn.ClientSend(`{"type":4,"invocationId":"nnn","target":"uploadstreamsmoke","arguments":[5.0],"streamIds":["fff","ggg"]}`)
				<-hub.ch
				<-conn.received
				// Send stream item
				conn.ClientSend(`{"type":2,"invocationId":"fff","item":1}`)
				<-hub.ch
				<-conn.received
				// Send completion message with result
				conn.ClientSend(`{"type":3,"invocationId":"fff","result":1}`)
				message := <-conn.received
				Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
				Expect(message.(closeMessage).Error).NotTo(BeNil())
				server.cancel()
				close(done)
			})
		})

		Context("When the stream item type could not converted to the hub methods receive channel type", func() {
			It("should end the connection with an error", func(done Done) {
				hub := &clientStreamHub{ch: make(chan string, 20)}
				server, err := NewServer(context.TODO(), HubFactory(func() HubInterface {
					return hub
				}), testLoggerOption())
				Expect(err).NotTo(HaveOccurred())
				conn := newTestingConnectionForServer()
				go func() { _ = server.Serve(conn) }()
				conn.ClientSend(`{"type":4,"invocationId":"nnn","target":"uploaderror","streamIds":["eee"]}`)
				<-hub.ch
				<-conn.received
				// Send stream item
				conn.ClientSend(`{"type":2,"invocationId":"eee","item":1}`)
				message := <-conn.received
				Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
				Expect(message.(closeMessage).Error).NotTo(BeNil())
				server.cancel()
				close(done)
			})
		})

		Context("When the stream item array type could not converted to the hub methods receive channel array type", func() {
			It("should end the connection with an error", func(done Done) {
				hub := &clientStreamHub{ch: make(chan string, 20)}
				server, err := NewServer(context.TODO(), HubFactory(func() HubInterface {
					return hub
				}), testLoggerOption())
				Expect(err).NotTo(HaveOccurred())
				conn := newTestingConnectionForServer()
				go func() { _ = server.Serve(conn) }()
				conn.ClientSend(`{"type":4,"invocationId":"nnn","target":"uploaderrorarray","streamIds":["aeae"]}`)
				<-hub.ch
				<-conn.received
				// Send stream item
				conn.ClientSend(`{"type":2,"invocationId":"aeae","item":[7,8]}`)
				message := <-conn.received
				Expect(message).To(BeAssignableToTypeOf(closeMessage{}))
				Expect(message.(closeMessage).Error).NotTo(BeNil())
				server.cancel()
				close(done)
			})
		})
	})
})
