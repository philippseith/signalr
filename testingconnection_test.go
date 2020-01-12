package signalr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io"
	"sync"
	"time"
)

type testingConnection struct {
	connectionID string
	srvWriter    io.Writer
	srvReader    io.Reader
	cliWriter    io.Writer
	cliReader    io.Reader
	received     chan interface{}
	ehMutex      sync.Mutex
	errorHandler func(error)
	cnMutex      sync.Mutex
	connected    bool
	cliSendChan  chan string
	srvSendChan  chan []byte
}

var connNum = 0

func (t *testingConnection) ConnectionID() string {
	if t.connectionID == "" {
		connNum++
		t.connectionID = fmt.Sprintf("test%v", connNum)
	}
	return t.connectionID
}

func (t *testingConnection) Read(b []byte) (n int, err error) {
	return t.srvReader.Read(b)
}

func (t *testingConnection) Write(b []byte) (n int, err error) {
	t.srvSendChan <- b
	return len(b), nil
}

func (t *testingConnection) SetReceiveErrorHandler(errorHandler func(error)) {
	t.ehMutex.Lock()
	defer t.ehMutex.Unlock()
	t.errorHandler = errorHandler
}

func (t *testingConnection) callErrorHandler(err error) {
	t.ehMutex.Lock()
	defer t.ehMutex.Unlock()
	t.errorHandler(err)
}

func (t *testingConnection) Connected() bool {
	t.cnMutex.Lock()
	defer t.cnMutex.Unlock()
	return t.connected
}

func (t *testingConnection) setConnected(connected bool) {
	t.cnMutex.Lock()
	defer t.cnMutex.Unlock()
	t.connected = connected
}

func newTestingConnection() *testingConnection {
	cliReader, srvWriter := io.Pipe()
	srvReader, cliWriter := io.Pipe()
	conn := testingConnection{
		srvWriter:    srvWriter,
		srvReader:    srvReader,
		cliWriter:    cliWriter,
		cliReader:    cliReader,
		errorHandler: func(err error) { Fail(fmt.Sprintf("received invalid message from server %v", err.Error())) },
		received:     make(chan interface{}, 20),
		cliSendChan:  make(chan string, 20),
		srvSendChan:  make(chan []byte, 20),
	}
	// Send initial Handshake
	conn.clientSend(`{"protocol": "json","version": 1}`)
	conn.setConnected(true)

	// Receive loop
	go func() {
		defer GinkgoRecover()
		for {
			if message, err := conn.clientReceive(); err == nil {
				var hubMessage hubMessage
				if err = json.Unmarshal([]byte(message), &hubMessage); err == nil {
					switch hubMessage.Type {
					case 1, 4:
						var invocationMessage invocationMessage
						if err = json.Unmarshal([]byte(message), &invocationMessage); err == nil {
							conn.received <- invocationMessage
						} else {
							conn.errorHandler(err)
						}
					case 2:
						var streamItemMessage streamItemMessage
						if err = json.Unmarshal([]byte(message), &streamItemMessage); err == nil {
							conn.received <- streamItemMessage
						} else {
							conn.errorHandler(err)
						}
					case 3:
						var completionMessage completionMessage
						if err = json.Unmarshal([]byte(message), &completionMessage); err == nil {
							conn.received <- completionMessage
						} else {
							conn.errorHandler(err)
						}
					case 7:
						var closeMessage closeMessage
						if err = json.Unmarshal([]byte(message), &closeMessage); err == nil {
							conn.setConnected(false)
							if closeMessage.Error != "" {
								// Most time, a closeMessage with error is not whats expected by the tests
								conn.errorHandler(errors.New(closeMessage.Error))
							}
							conn.received <- closeMessage
						} else {
							conn.errorHandler(err)
						}
					}
				} else {
					conn.errorHandler(err)
				}
			}
		}
	}()
	// client send loop
	go func() {
		for {
			_, _ = conn.cliWriter.Write(append([]byte(<-conn.cliSendChan), 30))
		}
	}()
	// server send loop
	go func() {
		for {
			_, _ = conn.srvWriter.Write(<-conn.srvSendChan)
		}
	}()
	return &conn
}

func (t *testingConnection) clientSend(message string) {
	t.cliSendChan <- message
}

func (t *testingConnection) clientReceive() (string, error) {
	var buf bytes.Buffer
	var data = make([]byte, 1<<15) // 32K
	var n int
	for {
		if message, err := buf.ReadString(30); err != nil {
			buf.Write(data[:n])
			if n, err = t.cliReader.Read(data); err == nil {
				buf.Write(data[:n])
			} else {
				return "", err
			}
		} else {
			return message[:len(message)-1], nil
		}
	}
}

var _ = Describe("Connection", func() {

	Describe("Connection closed", func() {
		Context("When the connection is closed", func() {
			It("should not answer an invocation", func() {
				conn := connect(&Hub{})
				conn.clientSend(`{"type":7}`)
				conn.clientSend(`{"type":1,"invocationId": "123","target":"simple"}`)
				// When the connection is closed, the server should either send a closeMessage or nothing at all
				select {
				case message := <-conn.received:
					Expect(message.(closeMessage)).NotTo(BeNil())
				case <-time.After(100 * time.Millisecond):
				}
			})
		})
	})
})

var _ = Describe("Protocol", func() {

	Describe("Invalid messages", func() {
		Context("When a message with invalid id is sent", func() {
			It("should close the connection with an error", func() {
				conn := connect(&Hub{})
				// Disable error handling
				conn.SetReceiveErrorHandler(func(err error) {})
				conn.clientSend(`{"type":99}`)
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
})
