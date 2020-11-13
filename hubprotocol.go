package signalr

import (
	"bytes"
	"io"
)

// HubProtocol interface
// ReadMessage() reads a message from buf and returns the message if the buf contained one completely.
// If buf does not contain the whole message, it returns a nil message and complete false
// WriteMessage writes a message to the specified writer
// UnmarshalArgument() unmarshals a raw message depending of the specified value type into value
type HubProtocol interface {
	ReadMessage(buf *bytes.Buffer) (interface{}, bool, error)
	WriteMessage(message interface{}, writer io.Writer) error
	UnmarshalArgument(argument interface{}, value interface{}) error
	setDebugLogger(dbg StructuredLogger)
}

// Protocol
type hubMessage struct {
	Type int `json:"type" msg:"type"`
}

type invocationMessage struct {
	Type         int           `json:"type" msg:"type"`
	Target       string        `json:"target" msg:"target"`
	InvocationID string        `json:"invocationId,omitempty" msg:"invocationId,omitempty"`
	Arguments    []interface{} `json:"arguments" msg:"arguments"`
	StreamIds    []string      `json:"streamIds,omitempty" msg:"streamIds,omitempty"`
}

type completionMessage struct {
	Type         int         `json:"type" msg:"type"`
	InvocationID string      `json:"invocationId" msg:"invocationId"`
	Result       interface{} `json:"result,omitempty" msg:"result,omitempty"`
	Error        string      `json:"error,omitempty" msg:"error,omitempty"`
}

type streamItemMessage struct {
	Type         int         `json:"type" msg:"type"`
	InvocationID string      `json:"invocationId" msg:"invocationId"`
	Item         interface{} `json:"item" msg:"item"`
}

type cancelInvocationMessage struct {
	Type         int    `json:"type" msg:"type"`
	InvocationID string `json:"invocationId" msg:"invocationId"`
}

type closeMessage struct {
	Type           int    `json:"type" msg:"type"`
	Error          string `json:"error" msg:"error"`
	AllowReconnect bool   `json:"allowReconnect" msg:"allowReconnect"`
}

type handshakeRequest struct {
	Protocol string `json:"protocol" msg:"protocol"`
	Version  int    `json:"version" msg:"version"`
}

type handshakeResponse struct {
	Error string `json:"error,omitempty" msg:"error,omitempty"`
}
