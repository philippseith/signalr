package signalr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/mailru/easyjson"
	"github.com/mailru/easyjson/jwriter"
	"io"
	"reflect"
)

// JSONHubProtocol is the JSON based SignalR protocol
type JSONHubProtocol struct {
	dbg        log.Logger
	easyWriter jwriter.Writer
}

// Protocol specific message for correct unmarshaling of Arguments
type jsonInvocationMessage struct {
	Type         int               `json:"type"`
	Target       string            `json:"target"`
	InvocationID string            `json:"invocationId"`
	Arguments    []json.RawMessage `json:"arguments"`
	StreamIds    []string          `json:"streamIds,omitempty"`
}

type jsonError struct {
	raw string
	err error
}

func (j *jsonError) Error() string {
	return fmt.Sprintf("%v (source: %v)", j.err, j.raw)
}

// UnmarshalArgument unmarshals a json.RawMessage depending of the specified value type into value
func (j *JSONHubProtocol) UnmarshalArgument(argument interface{}, value interface{}) error {
	if err := json.Unmarshal(argument.(json.RawMessage), value); err != nil {
		return &jsonError{string(argument.(json.RawMessage)), err}
	}
	_ = j.dbg.Log(evt, "UnmarshalArgument",
		"argument", string(argument.(json.RawMessage)),
		"value", fmt.Sprintf("%v", reflect.ValueOf(value).Elem()))
	return nil
}

// ReadMessage reads a JSON message from buf and returns the message if the buf contained one completely.
// If buf does not contain the whole message, it returns a nil message and complete false
func (j *JSONHubProtocol) ReadMessage(buf *bytes.Buffer) (m interface{}, complete bool, err error) {
	data, err := parseTextMessageFormat(buf)
	switch {
	case errors.Is(err, io.EOF):
		return nil, false, err
		// Other errors never happen, because parseTextMessageFormat will only return err
		// from bytes.Buffer.ReadBytes() which is always io.EOF or nil
	}

	message := hubMessage{}
	err = message.UnmarshalJSON(data)
	_ = j.dbg.Log(evt, "read", msg, string(data))
	if err != nil {
		return nil, true, &jsonError{string(data), err}
	}

	switch message.Type {
	case 1, 4:
		jsonInvocation := jsonInvocationMessage{}
		if err = json.Unmarshal(data, &jsonInvocation); err != nil {
			err = &jsonError{string(data), err}
		}
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
		if err = streamItem.UnmarshalJSON(data); err != nil {
			err = &jsonError{string(data), err}
		}
		return streamItem, true, err
	case 3:
		completion := completionMessage{}
		if err = completion.UnmarshalJSON(data); err != nil {
			err = &jsonError{string(data), err}
		}
		return completion, true, err
	case 5:
		invocation := cancelInvocationMessage{}
		if err = invocation.UnmarshalJSON(data); err != nil {
			err = &jsonError{string(data), err}
		}
		return invocation, true, err
	case 7:
		cm := closeMessage{}
		if err = cm.UnmarshalJSON(data); err != nil {
			err = &jsonError{string(data), err}
		}
		return cm, true, err
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
	// Remove the delimiter
	return data[0 : len(data)-1], err
}

// WriteMessage writes a message as JSON to the specified writer
func (j *JSONHubProtocol) WriteMessage(message interface{}, writer io.Writer) error {
	if em, ok := message.(easyjson.Marshaler); ok {
		em.MarshalEasyJSON(&j.easyWriter)
		j.easyWriter.RawByte(30)
		b := j.easyWriter.Buffer.BuildBytes()
		_ = j.dbg.Log(evt, "write", msg, string(b))
		_, err := writer.Write(b)
		return err
	}
	buf := bytes.Buffer{}
	if marshaler, ok := message.(json.Marshaler); ok {
		b, err := marshaler.MarshalJSON()
		if err != nil {
			return err
		}
		_, _ = buf.Write(b) // err is always nil
	} else if err := json.NewEncoder(&buf).Encode(message); err != nil {
		// Don't know when this will happen, presumably never
		return err
	}
	_ = buf.WriteByte(30) // bytes.Buffer.WriteByte() returns always nil
	_ = j.dbg.Log(evt, "write", msg, buf.String())
	_, err := writer.Write(buf.Bytes())
	return err
}

func (j *JSONHubProtocol) setDebugLogger(dbg StructuredLogger) {
	j.dbg = log.WithPrefix(dbg, "ts", log.DefaultTimestampUTC, "protocol", "JSON")
}
