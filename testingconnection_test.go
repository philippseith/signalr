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
	srvWriter io.Writer
	srvReader io.Reader
	cliWriter io.Writer
	cliReader io.Reader
	received  chan interface{}
	ehMutex sync.Mutex
	errorHandler func(error)
	cnMutex sync.Mutex
	connected bool
	sendchan chan string
}

func (t *testingConnection) ConnectionID() string {
	return "test"
}

func (t *testingConnection) Read(b []byte) (n int, err error) {
	return t.srvReader.Read(b)
}

func (t *testingConnection) Write(b []byte) (n int, err error) {
	return t.srvWriter.Write(b)
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
		srvWriter: srvWriter,
		srvReader: srvReader,
		cliWriter: cliWriter,
		cliReader: cliReader,
		errorHandler: func(err error) { Fail(fmt.Sprintf("received invalid message from server %v", err.Error()))},
		received: make(chan interface{}, 0),
		sendchan: make(chan string, 20),
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
							if closeMessage.Error == "" {
								conn.received <- closeMessage
							} else {
								conn.errorHandler(errors.New(closeMessage.Error))
							}
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
	// Send loop
	go func() {
		for {
			_, _ = conn.cliWriter.Write(append([]byte(<-conn.sendchan), 30))
		}
	}()
	return &conn
}

func (t *testingConnection) clientSend(message string) {
	t.sendchan <- message
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
		conn := connect(&Hub{})
		Context("When the connection is closed", func() {
			It("should not answer an invocation", func() {
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