package signalr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type jsonHubProtocol struct {
}

func (j *jsonHubProtocol) ReadMessage(buf *bytes.Buffer) (interface{}, error) {
	data, err := parseTextMessageFormat(buf)
	if err != nil {
		return nil, err
	}

	message := hubMessage{}
	err = json.Unmarshal(data, &message)

	if err != nil {
		return nil, err
	}

	switch message.Type {
	case 1, 4:
		invocation := hubInvocationMessage{}
		err = json.Unmarshal(data, &invocation)
		return invocation, err
	default:
		return message, nil
	}
}

func (j *jsonHubProtocol) WriteMessage(message interface{}, writer io.Writer) error {

	// TODO: Reduce the amount of copies

	// We're copying because we want to write complete messages to the underlying writer
	buf := bytes.Buffer{}

	if err := json.NewEncoder(&buf).Encode(message); err != nil {
		return err
	}
	fmt.Printf("Message sent %v", string(buf.Bytes()))

	if err := buf.WriteByte(30); err != nil {
		return err
	}

	_, err := writer.Write(buf.Bytes())
	return err
}

