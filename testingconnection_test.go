package signalr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/onsi/ginkgo"
	"io"
)

type testingConnection struct {
	srvWriter io.Writer
	srvReader io.Reader
	cliWriter io.Writer
	cliReader io.Reader
	received  chan interface{}
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

func newTestingConnection() *testingConnection {
	cliReader, srvWriter := io.Pipe()
	srvReader, cliWriter := io.Pipe()
	conn := testingConnection{
		srvWriter: srvWriter,
		srvReader: srvReader,
		cliWriter: cliWriter,
		cliReader: cliReader,
	}
	// Send initial Handshake
	go func() {
		if _, err := conn.clientSend(`{"protocol": "json","version": 1}`); err != nil {
			ginkgo.Fail(fmt.Sprint(err))
		}
	}()
	conn.received = make(chan interface{}, 0)
	go func() {
		for {
			if message, err := conn.clientReceive(); err == nil {
				var hubMessage hubMessage
				if err = json.Unmarshal([]byte(message), &hubMessage); err == nil {
					switch hubMessage.Type {
					case 2:
						var streamItemMessage streamItemMessage
						if err = json.Unmarshal([]byte(message), &streamItemMessage); err == nil {
							conn.received <- streamItemMessage
						}
					case 3:
						var completionMessage completionMessage
						if err = json.Unmarshal([]byte(message), &completionMessage); err == nil {
							conn.received <- completionMessage
						}
					}
				}
			}
		}
	}()
	return &conn
}

func (t *testingConnection) clientSend(message string) (int, error) {
	return t.cliWriter.Write(append([]byte(message), 30))
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
			} else{
				return "", err
			}
		} else {
			return message[:len(message)-1], nil
		}
	}
}
