package signalr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/go-kit/log"
)

// jsonHubProtocol is the JSON based SignalR protocol
type jsonHubProtocol struct {
	dbg log.Logger
}

// Protocol specific messages for correct unmarshaling of arguments or results.
// jsonInvocationMessage is only used in ParseMessages, not in WriteMessage
type jsonInvocationMessage struct {
	Type         int               `json:"type"`
	Target       string            `json:"target"`
	InvocationID string            `json:"invocationId"`
	Arguments    []json.RawMessage `json:"arguments"`
	StreamIds    []string          `json:"streamIds,omitempty"`
}

type jsonStreamItemMessage struct {
	Type         int             `json:"type"`
	InvocationID string          `json:"invocationId"`
	Item         json.RawMessage `json:"item"`
}

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

// UnmarshalArgument unmarshals a json.RawMessage depending on the specified value type into value
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

// ParseMessages reads all messages from the reader and puts the remaining bytes into remainBuf
func (j *jsonHubProtocol) ParseMessages(reader io.Reader, remainBuf *bytes.Buffer) (messages []interface{}, err error) {
	frames, err := readJSONFrames(reader, remainBuf)
	if err != nil {
		return nil, err
	}
	message := hubMessage{}
	messages = make([]interface{}, 0)
	for _, frame := range frames {
		err = json.Unmarshal(frame, &message)
		_ = j.dbg.Log(evt, "read", msg, string(frame))
		if err != nil {
			return nil, &jsonError{string(frame), err}
		}
		typedMessage, err := j.parseMessage(message.Type, frame)
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
		if err = json.Unmarshal(text, &invocation); err != nil {
			err = &jsonError{string(text), err}
		}
		return invocation, err
	case 7:
		cm := closeMessage{}
		if err = json.Unmarshal(text, &cm); err != nil {
			err = &jsonError{string(text), err}
		}
		return cm, err
	default:
		return nil, nil
	}
}

// readJSONFrames reads all complete frames (delimited by 0x1e) from the reader and puts the remaining bytes into remainBuf
func readJSONFrames(reader io.Reader, remainBuf *bytes.Buffer) ([][]byte, error) {
	p := make([]byte, 1<<15)
	buf := &bytes.Buffer{}
	_, _ = buf.ReadFrom(remainBuf)
	// Try getting data until at least one frame is available
	for {
		n, err := reader.Read(p)
		// Some reader implementations return io.EOF additionally to n=0 if no data could be read
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if n > 0 {
			_, _ = buf.Write(p[:n])
			frames, err := parseJSONFrames(buf)
			if err != nil {
				return nil, err
			}
			if len(frames) > 0 {
				_, _ = remainBuf.ReadFrom(buf)
				return frames, nil
			}
		}
	}
}

func parseJSONFrames(buf *bytes.Buffer) ([][]byte, error) {
	frames := make([][]byte, 0)
	for {
		frame, err := buf.ReadBytes(0x1e)
		if errors.Is(err, io.EOF) {
			// Restore incomplete frame in buffer
			_, _ = buf.Write(frame)
			break
		}
		if err != nil {
			return nil, err
		}
		frames = append(frames, frame[:len(frame)-1])
	}
	return frames, nil
}

// WriteMessage writes a message as JSON to the specified writer
func (j *jsonHubProtocol) WriteMessage(message interface{}, writer io.Writer) error {
	var b []byte
	var err error
	if marshaler, ok := message.(json.Marshaler); ok {
		b, err = marshaler.MarshalJSON()
	} else {
		b, err = json.Marshal(message)
	}
	if err != nil {
		return err
	}
	b = append(b, 0x1e)
	_ = j.dbg.Log(evt, "write", msg, string(b))
	_, err = writer.Write(b)
	return err
}

func (j *jsonHubProtocol) transferMode() TransferMode {
	return TextTransferMode
}

func (j *jsonHubProtocol) setDebugLogger(dbg StructuredLogger) {
	j.dbg = log.WithPrefix(dbg, "ts", log.DefaultTimestampUTC, "protocol", "JSON")
}
