package signalr

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/go-kit/log"
	"github.com/vmihailenco/msgpack/v5"
)

type messagePackHubProtocol struct {
	dbg log.Logger
}

func (m *messagePackHubProtocol) ParseMessages(reader io.Reader, remainBuf *bytes.Buffer) ([]interface{}, error) {
	frames, err := m.readFrames(reader, remainBuf)
	if err != nil {
		return nil, err
	}
	messages := make([]interface{}, 0)
	for _, frame := range frames {
		message, err := m.parseMessage(bytes.NewBuffer(frame))
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func (m *messagePackHubProtocol) readFrames(reader io.Reader, remainBuf *bytes.Buffer) ([][]byte, error) {
	frames := make([][]byte, 0)
	for {
		// Try to get the frame length
		frameLenBuf := make([]byte, binary.MaxVarintLen32)
		n1, err := remainBuf.Read(frameLenBuf)
		if err != nil && !errors.Is(err, io.EOF) {
			// Some weird other error
			return nil, err
		}
		n2, err := reader.Read(frameLenBuf[n1:])
		if err != nil && !errors.Is(err, io.EOF) {
			// Some weird other error
			return nil, err
		}
		frameLen, lenLen := binary.Uvarint(frameLenBuf[:n1+n2])
		if lenLen == 0 {
			// reader could not supply enough bytes to decode the Uvarint
			// Store the already read bytes in the remainBuf for next iteration
			_, _ = remainBuf.Write(frameLenBuf[:n1+n2])
			return frames, nil
		}
		if lenLen < 0 {
			return nil, fmt.Errorf("messagepack frame length to large")
		}
		// Still wondering why this happens, but it happens!
		if frameLen == 0 {
			// Store the overread bytes for the next iteration
			_, _ = remainBuf.Write(frameLenBuf[lenLen:])
			continue
		}
		// Try getting data until at least one frame is available
		readBuf := make([]byte, frameLen)
		frameBuf := &bytes.Buffer{}
		// Did we read too many bytes when detecting the frameLen?
		_, _ = frameBuf.Write(frameLenBuf[lenLen:])
		// Read the rest of the bytes from the last iteration
		_, _ = frameBuf.ReadFrom(remainBuf)
		for {
			n, err := reader.Read(readBuf)
			if errors.Is(err, io.EOF) {
				// Less than frameLen. Let the caller parse the already read frames and come here again later
				_, _ = remainBuf.ReadFrom(frameBuf)
				return frames, nil
			}
			if err != nil {
				return nil, err
			}
			_, _ = frameBuf.Write(readBuf[:n])
			if frameBuf.Len() == int(frameLen) {
				// Frame completely read. Return it to the caller
				frames = append(frames, frameBuf.Next(int(frameLen)))
				return frames, nil
			}
			if frameBuf.Len() > int(frameLen) {
				// More than frameLen. Append the current frame to the result and start reading the next frame
				frames = append(frames, frameBuf.Next(int(frameLen)))
				_, _ = remainBuf.ReadFrom(frameBuf)
				break
			}
		}
	}
}

func (m *messagePackHubProtocol) parseMessage(buf *bytes.Buffer) (interface{}, error) {
	decoder := msgpack.NewDecoder(buf)
	// Default map decoding expects all maps to have string keys
	decoder.SetMapDecoder(func(decoder *msgpack.Decoder) (interface{}, error) {
		return decoder.DecodeUntypedMap()
	})
	msgLen, err := decoder.DecodeArrayLen()
	if err != nil {
		return nil, err
	}
	msgType, err := decoder.DecodeInt()
	if err != nil {
		return nil, err
	}
	// Ignore Header for all messages, except ping message that has no header
	// see message spec at https://github.com/dotnet/aspnetcore/blob/main/src/SignalR/docs/specs/HubProtocol.md#message-headers
	if msgType != 6 {
		_, err = decoder.DecodeMap()
		if err != nil {
			return nil, err
		}
	}
	switch msgType {
	case 1, 4:
		if msgLen < 5 {
			return nil, fmt.Errorf("invalid invocationMessage length %v", msgLen)
		}
		invocationID, err := m.decodeInvocationID(decoder)
		if err != nil {
			return nil, err
		}
		invocationMessage := invocationMessage{
			Type:         msgType,
			InvocationID: invocationID,
		}
		invocationMessage.Target, err = decoder.DecodeString()
		if err != nil {
			return nil, err
		}
		argLen, err := decoder.DecodeArrayLen()
		if err != nil {
			return nil, err
		}
		for i := 0; i < argLen; i++ {
			argument, err := decoder.DecodeRaw()
			if err != nil {
				return nil, err
			}
			invocationMessage.Arguments = append(invocationMessage.Arguments, argument)
		}
		// StreamIds seem to be optional
		if msgLen == 6 {
			streamIDLen, err := decoder.DecodeArrayLen()
			if err != nil {
				return nil, err
			}
			for i := 0; i < streamIDLen; i++ {
				streamID, err := decoder.DecodeString()
				if err != nil {
					return nil, err
				}
				invocationMessage.StreamIds = append(invocationMessage.StreamIds, streamID)
			}
		}
		return invocationMessage, nil
	case 2:
		if msgLen != 4 {
			return nil, fmt.Errorf("invalid streamItemMessage length %v", msgLen)
		}
		streamItemMessage := streamItemMessage{Type: 2}
		streamItemMessage.InvocationID, err = decoder.DecodeString()
		if err != nil {
			return nil, err
		}
		streamItemMessage.Item, err = decoder.DecodeRaw()
		if err != nil {
			return nil, err
		}
		return streamItemMessage, nil
	case 3:
		if msgLen < 4 {
			return nil, fmt.Errorf("invalid completionMessage length %v", msgLen)
		}
		completionMessage := completionMessage{Type: 3}
		completionMessage.InvocationID, err = decoder.DecodeString()
		if err != nil {
			return nil, err
		}
		resultKind, err := decoder.DecodeInt8()
		if err != nil {
			return nil, err
		}
		switch resultKind {
		case 1: // Error result
			if msgLen < 5 {
				return nil, fmt.Errorf("invalid completionMessage length %v", msgLen)
			}
			completionMessage.Error, err = decoder.DecodeString()
			if err != nil {
				return nil, err
			}
		case 2: // Void result
		case 3: // Non-void result
			if msgLen < 5 {
				return nil, fmt.Errorf("invalid completionMessage length %v", msgLen)
			}
			completionMessage.Result, err = decoder.DecodeRaw()
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("invalid resultKind %v", resultKind)
		}
		return completionMessage, nil
	case 5:
		if msgLen != 3 {
			return nil, fmt.Errorf("invalid cancelInvocationMessage length %v", msgLen)
		}
		cancelInvocationMessage := cancelInvocationMessage{Type: 5}
		cancelInvocationMessage.InvocationID, err = decoder.DecodeString()
		if err != nil {
			return nil, err
		}
		return cancelInvocationMessage, nil
	case 6:
		if msgLen != 1 {
			return nil, fmt.Errorf("invalid pingMessage length %v", msgLen)
		}
		return hubMessage{Type: 6}, nil
	case 7:
		if msgLen < 2 {
			return nil, fmt.Errorf("invalid closeMessage length %v", msgLen)
		}
		closeMessage := closeMessage{Type: 7}
		closeMessage.Error, err = decoder.DecodeString()
		if err != nil {
			return nil, err
		}
		if msgLen > 2 {
			closeMessage.AllowReconnect, err = decoder.DecodeBool()
			if err != nil {
				return nil, err
			}
		}
		return closeMessage, nil
	}
	return msg, nil
}

func (m *messagePackHubProtocol) decodeInvocationID(decoder *msgpack.Decoder) (string, error) {
	rawID, err := decoder.DecodeInterface()
	if err != nil {
		return "", err
	}
	// nil is ok
	if rawID == nil {
		return "", nil
	}
	// Otherwise, it must be string
	invocationID, ok := rawID.(string)
	if !ok {
		return "", fmt.Errorf("invalid InvocationID %#v", rawID)
	}
	return invocationID, nil
}

func (m *messagePackHubProtocol) WriteMessage(message interface{}, writer io.Writer) error {
	// Encode message body
	buf := &bytes.Buffer{}
	encoder := msgpack.NewEncoder(buf)
	// Ensure uppercase/lowercase mapping for struct member names
	encoder.SetCustomStructTag("json")
	switch msg := message.(type) {
	case invocationMessage:
		if err := encodeMsgHeader(encoder, 6, msg.Type); err != nil {
			return err
		}
		if msg.InvocationID == "" {
			if err := encoder.EncodeNil(); err != nil {
				return err
			}
		} else {
			if err := encoder.EncodeString(msg.InvocationID); err != nil {
				return err
			}
		}
		if err := encoder.EncodeString(msg.Target); err != nil {
			return err
		}
		if err := encoder.EncodeArrayLen(len(msg.Arguments)); err != nil {
			return err
		}
		for _, arg := range msg.Arguments {
			if err := encoder.Encode(arg); err != nil {
				return err
			}
		}
		if err := encoder.EncodeArrayLen(len(msg.StreamIds)); err != nil {
			return err
		}
		for _, id := range msg.StreamIds {
			if err := encoder.EncodeString(id); err != nil {
				return err
			}
		}
	case streamItemMessage:
		if err := encodeMsgHeader(encoder, 4, msg.Type); err != nil {
			return err
		}
		if err := encoder.EncodeString(msg.InvocationID); err != nil {
			return err
		}
		if err := encoder.Encode(msg.Item); err != nil {
			return err
		}
	case completionMessage:
		msgLen := 4
		if msg.Result != nil || msg.Error != "" {
			msgLen = 5
		}
		if err := encodeMsgHeader(encoder, msgLen, msg.Type); err != nil {
			return err
		}
		if err := encoder.EncodeString(msg.InvocationID); err != nil {
			return err
		}
		var resultKind int8 = 2
		if msg.Error != "" {
			resultKind = 1
		} else if msg.Result != nil {
			resultKind = 3
		}
		if err := encoder.EncodeInt8(resultKind); err != nil {
			return err
		}
		switch resultKind {
		case 1:
			if err := encoder.EncodeString(msg.Error); err != nil {
				return err
			}
		case 3:
			if err := encoder.Encode(msg.Result); err != nil {
				return err
			}
		}
	case cancelInvocationMessage:
		if err := encodeMsgHeader(encoder, 3, msg.Type); err != nil {
			return err
		}
		if err := encoder.EncodeString(msg.InvocationID); err != nil {
			return err
		}
	case hubMessage:
		if err := encoder.EncodeArrayLen(1); err != nil {
			return err
		}
		if err := encoder.EncodeInt(6); err != nil {
			return err
		}
	case closeMessage:
		if err := encodeMsgHeader(encoder, 3, msg.Type); err != nil {
			return err
		}
		if err := encoder.EncodeString(msg.Error); err != nil {
			return err
		}
		if err := encoder.EncodeBool(msg.AllowReconnect); err != nil {
			return err
		}
	}
	// Build frame with length information
	frameBuf := &bytes.Buffer{}
	lenBuf := make([]byte, binary.MaxVarintLen32)
	lenLen := binary.PutUvarint(lenBuf, uint64(buf.Len()))
	if _, err := frameBuf.Write(lenBuf[:lenLen]); err != nil {
		return err
	}
	_ = m.dbg.Log(evt, "Write", msg, fmt.Sprintf("%#v", message))
	_, _ = frameBuf.ReadFrom(buf)
	_, err := frameBuf.WriteTo(writer)
	return err
}

func encodeMsgHeader(e *msgpack.Encoder, msgLen int, msgType int) (err error) {
	if err = e.EncodeArrayLen(msgLen); err != nil {
		return err
	}
	if err = e.EncodeInt(int64(msgType)); err != nil {
		return err
	}
	headers := make(map[string]interface{})
	if err = e.EncodeMap(headers); err != nil {
		return err
	}
	return nil
}

func (m *messagePackHubProtocol) transferMode() TransferMode {
	return BinaryTransferMode
}

func (m *messagePackHubProtocol) setDebugLogger(dbg StructuredLogger) {
	m.dbg = log.WithPrefix(dbg, "ts", log.DefaultTimestampUTC, "protocol", "MSGP")
}

// UnmarshalArgument unmarshals raw bytes to a destination value. dst is the pointer to the destination value.
func (m *messagePackHubProtocol) UnmarshalArgument(src interface{}, dst interface{}) error {
	rawSrc, ok := src.(msgpack.RawMessage)
	if !ok {
		return fmt.Errorf("invalid source %#v for UnmarshalArgument", src)
	}
	buf := bytes.NewBuffer(rawSrc)
	decoder := msgpack.GetDecoder()
	defer msgpack.PutDecoder(decoder)
	decoder.Reset(buf)
	// Default map decoding expects all maps to have string keys
	decoder.SetMapDecoder(func(decoder *msgpack.Decoder) (interface{}, error) {
		return decoder.DecodeUntypedMap()
	})
	// Ensure uppercase/lowercase mapping for struct member names
	decoder.SetCustomStructTag("json")
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	return nil
}
