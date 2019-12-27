package signalr_test

import (
	"bytes"
	"encoding/json"
	"github.com/philippseith/signalr"
	"io"
)

type testingHubConnection struct {
	signalr.HubConnectionBase
	cliWriter io.Writer
	cliReader io.Reader
	received  chan interface{}
}

type hubMessage struct {
	Type         int    `json:"type"`
	InvocationID string `json:"invocationId"`
}

type completionMessage struct {
	Type         int         `json:"type"`
	InvocationID string      `json:"invocationId"`
	Result       interface{} `json:"result"`
	Error        string      `json:"error"`
}

type streamItemMessage struct {
	Type         int         `json:"type"`
	InvocationID string      `json:"invocationId"`
	Item         interface{} `json:"item"`
}


func newTestingHubConnection() *testingHubConnection {
	cliReader, srvWriter := io.Pipe()
	srvReader, cliWriter := io.Pipe()
	conn := &testingHubConnection{
		HubConnectionBase: signalr.HubConnectionBase{
			ConnectionID: "TestID",
			Protocol:     &signalr.JsonHubProtocol{},
			Connected:    1,
			Writer:       srvWriter,
			Reader:       srvReader,
		},
		cliWriter: cliWriter,
		cliReader: cliReader,
	}
	conn.received = make(chan interface{}, 20)
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
	return conn
}

func (t *testingHubConnection) clientSend(message string) (int, error) {
	return t.cliWriter.Write(append([]byte(message), 30))
}

func (t *testingHubConnection) clientReceive() (string, error) {
	var buf bytes.Buffer
	var data = make([]byte, 1<<15) // 32K
	var n int
	for {
		if message, err := buf.ReadString(30); err != nil {
			buf.Write(data[:n])
			if n, err = t.cliReader.Read(data); err != nil {
				return "", err
			} else {
				buf.Write(data[:n])
			}
		} else {
			return message[:len(message) - 1], nil
		}
	}
}
