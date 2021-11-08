package signalr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/onsi/ginkgo"
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
	failRead     string
	failWrite    string
	failMx       sync.Mutex
}

func (t *testingConnection) Context() context.Context {
	return context.TODO()
}

var connNum = 0
var connNumMx sync.Mutex

func (t *testingConnection) SetTimeout(timeout time.Duration) {
	t.timeout = timeout
}

func (t *testingConnection) Timeout() time.Duration {
	return t.timeout
}

func (t *testingConnection) ConnectionID() string {
	connNumMx.Lock()
	defer connNumMx.Unlock()
	if t.connectionID == "" {
		connNum++
		t.connectionID = fmt.Sprintf("test%v", connNum)
	}
	return t.connectionID
}

func (t *testingConnection) SetConnectionID(id string) {
	t.connectionID = id
}

func (t *testingConnection) Read(b []byte) (n int, err error) {
	if fr := t.FailRead(); fr != "" {
		defer func() { t.SetFailRead("") }()
		return 0, errors.New(fr)
	}
	timer := make(<-chan time.Time)
	if t.Timeout() > 0 {
		timer = time.After(t.Timeout())
	}
	nch := make(chan int)
	go func() {
		n, _ := t.srvReader.Read(b)
		nch <- n
	}()
	select {
	case n := <-nch:
		return n, nil
	case <-timer:
		return 0, fmt.Errorf("timeout %v", t.Timeout())
	}
}

func (t *testingConnection) Write(b []byte) (n int, err error) {
	if fw := t.FailWrite(); fw != "" {
		defer func() { t.SetFailWrite("") }()
		return 0, errors.New(fw)
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

func (t *testingConnection) FailRead() string {
	defer t.failMx.Unlock()
	t.failMx.Lock()
	return t.failRead
}

func (t *testingConnection) FailWrite() string {
	defer t.failMx.Unlock()
	t.failMx.Lock()
	return t.failWrite
}

func (t *testingConnection) SetFailRead(fail string) {
	defer t.failMx.Unlock()
	t.failMx.Lock()
	t.failRead = fail
}

func (t *testingConnection) SetFailWrite(fail string) {
	defer t.failMx.Unlock()
	t.failMx.Lock()
	t.failWrite = fail
}

// newTestingConnectionForServer builds a testingConnection with an sent (but not yet received) handshake for testing a server
func newTestingConnectionForServer() *testingConnection {
	conn := newTestingConnection()
	// client receive loop
	go receiveLoop(conn)()
	// Send initial Handshake
	conn.ClientSend(`{"protocol": "json","version": 1}`)
	conn.SetConnected(true)
	return conn
}

func newTestingConnection() *testingConnection {
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
		timeout:     time.Second * 5,
	}
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

func (t *testingConnection) ClientSend(message string) {
	t.cliSendChan <- message
}

func (t *testingConnection) ClientReceive() (string, error) {
	var buf bytes.Buffer
	var data = make([]byte, 1<<15) // 32K
	var nn int
	for {
		if message, err := buf.ReadString(30); err != nil {
			buf.Write(data[:nn])
			if n, err := t.cliReader.Read(data[nn:]); err == nil {
				buf.Write(data[nn : nn+n])
				nn = nn + n
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
						var jsonStreamItemMessage jsonStreamItemMessage
						if err = json.Unmarshal([]byte(message), &jsonStreamItemMessage); err == nil {

							conn.ReceiveChan() <- streamItemMessage{
								Type:         jsonStreamItemMessage.Type,
								InvocationID: jsonStreamItemMessage.InvocationID,
								Item:         jsonStreamItemMessage.Item,
							}
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
