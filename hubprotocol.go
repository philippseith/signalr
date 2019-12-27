package signalr

import (
	"bytes"
	"io"
)

// HubProtocol interface
type HubProtocol interface {
	ReadMessage(buf *bytes.Buffer) (interface{}, bool, error)
	WriteMessage(message interface{}, writer io.Writer) error
	UnmarshalArgument(argument interface{}, value interface{}) error
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
	Result       interface{} `json:"result"`
	Error        string      `json:"error"`
}

type streamItemMessage struct {
	Type         int         `json:"type"`
	InvocationID string      `json:"invocationId"`
	Item         interface{} `json:"item"`
}

type cancelInvocationMessage struct {
	Type         int         `json:"type"`
	InvocationID string      `json:"invocationId"`
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

