package signalr

import (
	"bytes"
	"github.com/go-kit/kit/log"
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
	SetDebugLogger(dbg log.Logger)
}

// Protocol
type hubMessage struct {
	Type int `json:"type"`
}

type invocationMessage struct {
	Type         int
	Target       string
	InvocationID string
	Arguments    []interface{}
	StreamIds    []string
}

type sendOnlyHubInvocationMessage struct {
	Type      int           `json:"type"`
	Target    string        `json:"target"`
	Arguments []interface{} `json:"arguments"`
}

type completionMessage struct {
	Type         int         `json:"type"`
	InvocationID string      `json:"invocationId"`
	Result       interface{} `json:"result,omitempty"`
	Error        string      `json:"error,omitempty"`
}

type streamItemMessage struct {
	Type         int         `json:"type"`
	InvocationID string      `json:"invocationId"`
	Item         interface{} `json:"item"`
}

type cancelInvocationMessage struct {
	Type         int    `json:"type"`
	InvocationID string `json:"invocationId"`
}

type closeMessage struct {
	Type           int    `json:"type"`
	Error          string `json:"error"`
	AllowReconnect bool   `json:"allowReconnect"`
}

type handshakeRequest struct {
	Protocol string `json:"Protocol"`
	Version  int    `json:"version"`
}
