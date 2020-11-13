package signalr

import (
	"bytes"
	"github.com/vmihailenco/msgpack/v4"
	"io"
)

type MessagePackHubProtocol struct {
}

func (m MessagePackHubProtocol) ReadMessage(buf *bytes.Buffer) (message interface{}, complete bool, err error) {
	d := msgpack.NewDecoder(buf)
	_, err = d.DecodeInt()
	panic("implement me")
}

func (m MessagePackHubProtocol) WriteMessage(message interface{}, writer io.Writer) error {
	panic("implement me")
}

func (m MessagePackHubProtocol) UnmarshalArgument(argument interface{}, value interface{}) error {
	panic("implement me")
}

func (m MessagePackHubProtocol) setDebugLogger(dbg StructuredLogger) {
	panic("implement me")
}
