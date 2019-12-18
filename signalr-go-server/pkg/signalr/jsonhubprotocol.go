package signalr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type jsonHubProtocol struct {
}

// protocol specific message for correct unmarshaling Arguments
type jsonInvocationMessage struct {
	Type         int               `json:"type"`
	Target       string            `json:"target"`
	InvocationID string            `json:"invocationId"`
	Arguments    []json.RawMessage `json:"arguments"`
	StreamIds    []string          `json:"streamIds,omitempty"`
}

func (j *jsonHubProtocol) UnmarshalArgument(argument interface{}, value interface{}) error {
	return json.Unmarshal(argument.(json.RawMessage), value)
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
		jsonInvocation := jsonInvocationMessage{}
		err = json.Unmarshal(data, &jsonInvocation)
		arguments := make([]interface{}, len(jsonInvocation.Arguments))
		for i, a := range jsonInvocation.Arguments {
			arguments[i] = a
		}
		invocation := invocationMessage{
			Type:         jsonInvocation.Type,
			Target:       jsonInvocation.Target,
			InvocationID: jsonInvocation.InvocationID,
			Arguments:    arguments,
			StreamIds:    jsonInvocation.StreamIds,
		}
		return invocation, err
	case 2:
		streamItem := streamItemMessage{}
		err = json.Unmarshal(data, &streamItem)
		return streamItem, err
	case 3:
		completion := completionMessage{}
		err := json.Unmarshal(data, &completion)
		return completion, err
	case 5:
		invocation := cancelInvocationMessage{}
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



