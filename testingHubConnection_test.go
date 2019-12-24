package signalr_test

import (
	"bytes"
	"io"
	"github.com/philippseith/signalr"
)

type testingHubConnection struct {
	signalr.HubConnectionBase
	cliWriter io.Writer
	cliReader io.Reader
}

func newTestingHubConnection() *testingHubConnection {
	cliReader, srvWriter := io.Pipe()
	srvReader, cliWriter := io.Pipe()
	return &testingHubConnection{
		HubConnectionBase: signalr.HubConnectionBase{
			ConnectionID: "TestID",
			Protocol:     &signalr.JsonHubProtocol{},
			Connected:    0,
			Writer:       srvWriter,
			Reader:       srvReader,
		},
		cliWriter: cliWriter,
		cliReader: cliReader,
	}
}

func (t *testingHubConnection) clientSend(message string) (int, error) {
	return t.cliWriter.Write([]byte(message))
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
			return message, nil
		}
	}
}



