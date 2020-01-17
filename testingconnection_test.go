package signalr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/onsi/ginkgo"
	"io"
	"sync"
	"time"
)

type testingConnection struct {
	timeout      time.Duration
	connectionID string
	srvWriter    io.Writer
	srvReader    io.Reader
	cliWriter    io.Writer
	cliReader    io.Reader
	received     chan interface{}
	cnMutex      sync.Mutex
	connected    bool
	cliSendChan  chan string
	srvSendChan  chan []byte
	failRead     bool
	failWrite    bool
}

var connNum = 0
var cnm sync.Mutex

func (t *testingConnection) SetTimeout(timeout time.Duration) {
	t.timeout = timeout
}

func (t *testingConnection) Timeout() time.Duration {
	return t.timeout
}

func (t *testingConnection) ConnectionID() string {
	if t.connectionID == "" {
		defer cnm.Unlock()
		cnm.Lock()
		connNum++
		t.connectionID = fmt.Sprintf("test%v", connNum)
	}
	return t.connectionID
}

func (t *testingConnection) Read(b []byte) (n int, err error) {
	if t.failRead {
		t.failRead = false
		return 0, errors.New("test fail")
	}
	return t.srvReader.Read(b)
}

func (t *testingConnection) Write(b []byte) (n int, err error) {
	if t.failWrite {
		t.failWrite = false
		return 0, errors.New("test fail")
	}
	t.srvSendChan <- b
	return len(b), nil
}

func (t *testingConnection) Connected() bool {
	t.cnMutex.Lock()
	defer t.cnMutex.Unlock()
	return t.connected
}

func (t *testingConnection) SetConnected(connected bool) {
	t.cnMutex.Lock()
	defer t.cnMutex.Unlock()
	t.connected = connected
}

func newTestingConnection() *testingConnection {
	conn := newTestingConnectionBeforeHandshake()
	// Send initial Handshake
	conn.ClientSend(`{"protocol": "json","version": 1}`)
	conn.SetConnected(true)
	return conn
}

func newTestingConnectionBeforeHandshake() *testingConnection {
	cliReader, srvWriter := io.Pipe()
	srvReader, cliWriter := io.Pipe()
	conn := testingConnection{
		srvWriter:   srvWriter,
		srvReader:   srvReader,
		cliWriter:   cliWriter,
		cliReader:   cliReader,
		received:    make(chan interface{}, 20),
		cliSendChan: make(chan string, 20),
		srvSendChan: make(chan []byte, 20),
	}
	// client receive loop
	go receiveLoop(&conn)()
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

func (t *testingConnection) FailReadOnce() {
	t.failRead = true
}

func (t *testingConnection) FailWriteOnce() {
	t.failWrite = true
}

func (t *testingConnection) ClientSend(message string) {
	t.cliSendChan <- message
}

func (t *testingConnection) ClientReceive() (string, error) {
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

func (t *testingConnection) ReceiveChan() chan interface{} {
	return t.received
}

type clientReceiver interface {
	ClientReceive() (string, error)
	ReceiveChan() chan interface{}
	SetConnected(bool)
}

func receiveLoop(conn clientReceiver) func() {
	return func() {
		defer ginkgo.GinkgoRecover()
		errorHandler := func(err error) { ginkgo.Fail(fmt.Sprintf("received invalid message from server %v", err.Error())) }
		for {
			if message, err := conn.ClientReceive(); err == nil {
				var hubMessage hubMessage
				if err = json.Unmarshal([]byte(message), &hubMessage); err == nil {
					switch hubMessage.Type {
					case 1, 4:
						var invocationMessage invocationMessage
						if err = json.Unmarshal([]byte(message), &invocationMessage); err == nil {
							conn.ReceiveChan() <- invocationMessage
						} else {
							errorHandler(err)
						}
					case 2:
						var streamItemMessage streamItemMessage
						if err = json.Unmarshal([]byte(message), &streamItemMessage); err == nil {
							conn.ReceiveChan() <- streamItemMessage
						} else {
							errorHandler(err)
						}
					case 3:
						var completionMessage completionMessage
						if err = json.Unmarshal([]byte(message), &completionMessage); err == nil {
							conn.ReceiveChan() <- completionMessage
						} else {
							errorHandler(err)
						}
					case 7:
						var closeMessage closeMessage
						if err = json.Unmarshal([]byte(message), &closeMessage); err == nil {
							conn.SetConnected(false)
							conn.ReceiveChan() <- closeMessage
						} else {
							errorHandler(err)
						}
					}
				} else {
					errorHandler(err)
				}
			}
		}
	}
}
