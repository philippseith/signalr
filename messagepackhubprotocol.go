package signalr

import (
	"github.com/vmihailenco/msgpack/v4"
	"io"
)

type MessagePackHubProtocol struct {
}

func (m MessagePackHubProtocol) ParseMessage(buf io.Reader) (message interface{}, err error) {
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
