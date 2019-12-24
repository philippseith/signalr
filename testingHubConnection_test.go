package signalr

import (
	"bytes"
	"io"
)

type testingHubConnection struct {
	baseHubConnection
	cliWriter io.Writer
	cliReader io.Reader
}

func newTestingHubConnection() *testingHubConnection {
	cliReader, srvWriter := io.Pipe()
	srvReader, cliWriter := io.Pipe()
	return &testingHubConnection{
		baseHubConnection: baseHubConnection{
			connectionID: "TestID",
			protocol:     &jsonHubProtocol{},
			connected:    0,
			writer:       srvWriter,
			reader:       srvReader,
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



