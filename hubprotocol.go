package signalr

import (
	"bytes"
	"io"
)

// hubProtocol interface
// ParseMessages() parses messages from an io.Reader and stores unparsed bytes in remainBuf.
// If buf does not contain the whole message, it returns a nil message and complete false
// WriteMessage writes a message to the specified writer
// UnmarshalArgument() unmarshals a raw message depending of the specified value type into a destination value
type hubProtocol interface {
	ParseMessages(reader io.Reader, remainBuf *bytes.Buffer) ([]interface{}, error)
	WriteMessage(message interface{}, writer io.Writer) error
	UnmarshalArgument(src interface{}, dst interface{}) error
	setDebugLogger(dbg StructuredLogger)
	transferMode() TransferMode
}

//easyjson:json
type hubMessage struct {
	Type int `json:"type"`
}

// easyjson:json
type invocationMessage struct {
	Type         int           `json:"type"`
	Target       string        `json:"target"`
	InvocationID string        `json:"invocationId,omitempty"`
	Arguments    []interface{} `json:"arguments"`
	StreamIds    []string      `json:"streamIds,omitempty"`
}

//easyjson:json
type completionMessage struct {
	Type         int         `json:"type"`
	InvocationID string      `json:"invocationId"`
	Result       interface{} `json:"result,omitempty"`
	Error        string      `json:"error,omitempty"`
}

//easyjson:json
type streamItemMessage struct {
	Type         int         `json:"type"`
	InvocationID string      `json:"invocationId"`
	Item         interface{} `json:"item"`
}

//easyjson:json
type cancelInvocationMessage struct {
	Type         int    `json:"type"`
	InvocationID string `json:"invocationId"`
}

//easyjson:json
type closeMessage struct {
	Type           int    `json:"type"`
	Error          string `json:"error"`
	AllowReconnect bool   `json:"allowReconnect"`
}

//easyjson:json
type handshakeRequest struct {
	Protocol string `json:"protocol"`
	Version  int    `json:"version"`
}

//easyjson:json
type handshakeResponse struct {
	Error string `json:"error,omitempty"`
}
