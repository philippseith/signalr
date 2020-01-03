package signalr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// JsonHubProtocol is the JSON based SignalR protocol
type JsonHubProtocol struct {
}

// Protocol specific message for correct unmarshaling of Arguments
type jsonInvocationMessage struct {
	Type         int               `json:"type"`
	Target       string            `json:"target"`
	InvocationID string            `json:"invocationId"`
	Arguments    []json.RawMessage `json:"arguments"`
	StreamIds    []string          `json:"streamIds,omitempty"`
}

// UnmarshalArgument unmarshals a json.RawMessage depending of the specified value type into value
func (j *JsonHubProtocol) UnmarshalArgument(argument interface{}, value interface{}) error {
	return json.Unmarshal(argument.(json.RawMessage), value)
}

// ReadMessage reads a JSON message from buf and returns the messgae if the buf contained one completely.
// If buf does not contain the whole message, it returns a nil message and complete false
func (j *JsonHubProtocol) ReadMessage(buf *bytes.Buffer) (m interface{}, complete bool, err error) {
	data, err := parseTextMessageFormat(buf)
	switch {
	case errors.Is(err, io.EOF):
		return nil, false, err
	case err != nil:
		return nil, true, err
	}

	message := hubMessage{}
	err = json.Unmarshal(data, &message)

	if err != nil {
		return nil, true, err
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
		return invocation, true, err
	case 2:
		streamItem := streamItemMessage{}
		err = json.Unmarshal(data, &streamItem)
		return streamItem, true, err
	case 3:
		completion := completionMessage{}
		err := json.Unmarshal(data, &completion)
		return completion, true, err
	case 5:
		invocation := cancelInvocationMessage{}
		err = json.Unmarshal(data, &invocation)
		return invocation, true, err
	default:
		return message, true, nil
	}
}

func parseTextMessageFormat(buf *bytes.Buffer) ([]byte, error) {
	// 30 = ASCII record separator
	data, err := buf.ReadBytes(30)

	if err != nil {
		return data, err
	}
	// Remove the delimeter
	return data[0 : len(data)-1], err
}

// WriteMessage writes a message as JSON to the specified writer
func (j *JsonHubProtocol) WriteMessage(message interface{}, writer io.Writer) error {

	// TODO: Reduce the amount of copies

	// We're copying because we want to write complete messages to the underlying Writer
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
