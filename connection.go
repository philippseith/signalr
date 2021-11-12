package signalr

import (
	"context"
	"io"
)

// Connection describes a connection between signalR client and server
type Connection interface {
	io.Reader
	io.Writer
	Context() context.Context
	ConnectionID() string
	SetConnectionID(id string)
}

// TransferMode is either TextTransferMode or BinaryTransferMode
type TransferMode int

// MessageType constants.
const (
	// TextTransferMode is for UTF-8 encoded text messages like JSON.
	TextTransferMode TransferMode = iota + 1
	// BinaryTransferMode is for binary messages like MessagePack.
	BinaryTransferMode
)

// ConnectionWithTransferMode is a Connection with TransferMode (e.g. Websocket)
type ConnectionWithTransferMode interface {
	TransferMode() TransferMode
	SetTransferMode(transferMode TransferMode)
}
