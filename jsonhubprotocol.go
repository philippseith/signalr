package signalr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-kit/kit/log"
	"io"
	"reflect"
)

// jsonHubProtocol is the JSON based SignalR protocol
type jsonHubProtocol struct {
	dbg log.Logger
}

// Protocol specific messages for correct unmarshaling of arguments or results.
// jsonInvocationMessage is only used in ParseMessages, not in WriteMessage
//easyjson:json
type jsonInvocationMessage struct {
	Type         int               `json:"type"`
	Target       string            `json:"target"`
	InvocationID string            `json:"invocationId"`
	Arguments    []json.RawMessage `json:"arguments"`
	StreamIds    []string          `json:"streamIds,omitempty"`
}

//easyjson:json
type jsonStreamItemMessage struct {
	Type         int             `json:"type"`
	InvocationID string          `json:"invocationId"`
	Item         json.RawMessage `json:"item"`
}

//easyjson:json
type jsonCompletionMessage struct {
	Type         int             `json:"type"`
	InvocationID string          `json:"invocationId"`
	Result       json.RawMessage `json:"result,omitempty"`
	Error        string          `json:"error,omitempty"`
}

type jsonError struct {
	raw string
	err error
}

func (j *jsonError) Error() string {
	return fmt.Sprintf("%v (source: %v)", j.err, j.raw)
}

// UnmarshalArgument unmarshals a json.RawMessage depending of the specified value type into value
func (j *jsonHubProtocol) UnmarshalArgument(src interface{}, dst interface{}) error {
	rawSrc, ok := src.(json.RawMessage)
	if !ok {
		return fmt.Errorf("invalid source %#v for UnmarshalArgument", src)
	}
	if err := json.Unmarshal(rawSrc, dst); err != nil {
		return &jsonError{string(rawSrc), err}
	}
	_ = j.dbg.Log(evt, "UnmarshalArgument",
		"argument", string(rawSrc),
		"value", fmt.Sprintf("%v", reflect.ValueOf(dst).Elem()))
	return nil
}

// ReadMessage reads a JSON message from buf and returns the message if the buf contained one completely.
// If buf does not contain the whole message, it returns a nil message and complete false
func (j *jsonHubProtocol) ParseMessages(reader io.Reader, remainBuf *bytes.Buffer) (messages []interface{}, err error) {
	texts, err := parseTextMessageFormat(reader, remainBuf)
	if err != nil {
		return nil, err
	}
	message := hubMessage{}
	messages = make([]interface{}, 0)
	for _, text := range texts {
		err = message.UnmarshalJSON(text)
		_ = j.dbg.Log(evt, "read", msg, string(text))
		if err != nil {
			return nil, &jsonError{string(text), err}
		}
		typedMessage, err := j.parseMessage(message.Type, text)
		if err != nil {
			return nil, err
		}
		// No specific type (aka Ping), use hubMessage
		if typedMessage == nil {
			typedMessage = message
		}
		messages = append(messages, typedMessage)
	}
	return messages, nil
}

func (j *jsonHubProtocol) parseMessage(messageType int, text []byte) (message interface{}, err error) {
	switch messageType {
	case 1, 4:
		jsonInvocation := jsonInvocationMessage{}
		if err = json.Unmarshal(text, &jsonInvocation); err != nil {
			err = &jsonError{string(text), err}
		}
		arguments := make([]interface{}, len(jsonInvocation.Arguments))
		for i, a := range jsonInvocation.Arguments {
			arguments[i] = a
		}
		return invocationMessage{
			Type:         jsonInvocation.Type,
			Target:       jsonInvocation.Target,
			InvocationID: jsonInvocation.InvocationID,
			Arguments:    arguments,
			StreamIds:    jsonInvocation.StreamIds,
		}, err
	case 2:
		jsonStreamItem := jsonStreamItemMessage{}
		if err = json.Unmarshal(text, &jsonStreamItem); err != nil {
			err = &jsonError{string(text), err}
		}
		return streamItemMessage{
			Type:         jsonStreamItem.Type,
			InvocationID: jsonStreamItem.InvocationID,
			Item:         jsonStreamItem.Item,
		}, err
	case 3:
		jsonCompletion := jsonCompletionMessage{}
		if err = json.Unmarshal(text, &jsonCompletion); err != nil {
			err = &jsonError{string(text), err}
		}
		completion := completionMessage{
			Type:         jsonCompletion.Type,
			InvocationID: jsonCompletion.InvocationID,
			Error:        jsonCompletion.Error,
		}
		// Only assign Result when non nil. setting interface{} Result to (json.RawMessage)(nil)
		// will produce a value which can not compared to nil even if it is pointing towards nil!
		// See https://www.calhoun.io/when-nil-isnt-equal-to-nil/ for explanation
		if jsonCompletion.Result != nil {
			completion.Result = jsonCompletion.Result
		}
		return completion, err
	case 5:
		invocation := cancelInvocationMessage{}
		if err = invocation.UnmarshalJSON(text); err != nil {
			err = &jsonError{string(text), err}
		}
		return invocation, err
	case 7:
		cm := closeMessage{}
		if err = cm.UnmarshalJSON(text); err != nil {
			err = &jsonError{string(text), err}
		}
		return cm, err
	default:
		return nil, nil
	}
}

func parseTextMessageFormat(reader io.Reader, remainBuf *bytes.Buffer) ([][]byte, error) {
	p := make([]byte, 1<<15)
	texts := make([][]byte, 0)
	buf := bytes.Buffer{}
	for {
		_, _ = buf.ReadFrom(remainBuf)
		n, err := reader.Read(p)
		if err != nil {
			return nil, err
		}
		_, _ = buf.Write(p[:n])
		for {
			text, err := buf.ReadBytes(0x1e)
			if errors.Is(err, io.EOF) {
				_, _ = remainBuf.Write(text)
				if len(texts) > 0 {
					return texts, nil
				}
				break
			}
			texts = append(texts, text[:len(text)-1])
		}
	}
}

// WriteMessage writes a message as JSON to the specified writer
func (j *jsonHubProtocol) WriteMessage(message interface{}, writer io.Writer) error {
	if marshaler, ok := message.(json.Marshaler); ok {
		b, err := marshaler.MarshalJSON()
		if err != nil {
			return err
		}
		b = append(b, 0x1e)
		_ = j.dbg.Log(evt, "write", msg, string(b))
		_, err = writer.Write(b)
		return err
	}
	return fmt.Errorf("%#v does not implement json.Marshaler", message)
}

func (j *jsonHubProtocol) setDebugLogger(dbg StructuredLogger) {
	j.dbg = log.WithPrefix(dbg, "ts", log.DefaultTimestampUTC, "protocol", "JSON")
}
