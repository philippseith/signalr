package signalr

import (
	"bytes"
	"encoding/json"
	"io"
)

// HubProtocol interface
type HubProtocol interface {
	ReadMessage(buf *bytes.Buffer) (interface{}, error)
	WriteMessage(message interface{}, writer io.Writer) error
}

// Protocol
type hubMessage struct {
	Type int `json:"type"`
}

type hubInvocationMessage struct {
	Type         int               `json:"type"`
	Target       string            `json:"target"`
	InvocationID string            `json:"invocationId"`
	Arguments    []json.RawMessage `json:"arguments"`
	StreamIds    []string          `json:"streamIds,omitempty"`
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
	Protocol string `json:"protocol"`
	Version  int    `json:"version"`
}

