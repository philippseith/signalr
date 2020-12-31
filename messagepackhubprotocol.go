package signalr

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/vmihailenco/msgpack/v5"
	"io"
)

type messagePackHubProtocol struct {
	dbg log.Logger
}

func (m *messagePackHubProtocol) ParseMessages(reader io.Reader, remainBuf *bytes.Buffer) ([]interface{}, error) {
	buf := bytes.Buffer{}
	_, _ = buf.ReadFrom(remainBuf)
	decoder := msgpack.GetDecoder()
	defer msgpack.PutDecoder(decoder)
	decoder.Reset(&buf)
	// Default map decoding expects all maps to have string keys
	decoder.SetMapDecoder(func(decoder *msgpack.Decoder) (interface{}, error) {
		return decoder.DecodeUntypedMap()
	})
	p := make([]byte, 1<<15)
	notYetDecoded := make([]byte, 0)
	for {
		buf.Write(notYetDecoded)
		n, err := reader.Read(p)
		if err != nil {
			return nil, err
		}
		_, _ = buf.Write(p[:n])
		message, err := m.parseMessage(decoder)
		if err != nil {
			if errors.Is(err, io.EOF) {
				// Could not decode because bytes are missing. We need to read additional content
				notYetDecoded = append(notYetDecoded, p[:n]...)
				continue
			}
			// Could not decode because it is garbage
			return nil, err
		}
		// Could decode. Store remaining buf content
		_, _ = remainBuf.ReadFrom(&buf)
		return []interface{}{message}, nil
	}
}

func (m *messagePackHubProtocol) parseMessage(decoder *msgpack.Decoder) (interface{}, error) {
	msgLen, err := decoder.DecodeArrayLen()
	if err != nil {
		return nil, err
	}
	msgType8, err := decoder.DecodeInt8()
	if err != nil {
		return nil, err
	}
	// Ignore Header
	_, err = decoder.DecodeMap()
	if err != nil {
		return nil, err
	}
	msgType := int(msgType8)
	switch msgType {
	case 1, 4:
		if msgLen != 6 {
			return nil, fmt.Errorf("invalid invocationMessage length %v", msgLen)
		}
		invocationMessage := invocationMessage{Type: msgType}
		invocationMessage.InvocationID, err = decoder.DecodeString()
		if err != nil {
			return nil, err
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
		streamIdLen, err := decoder.DecodeArrayLen()
		if err != nil {
			return nil, err
		}
		for i := 0; i < streamIdLen; i++ {
			streamId, err := decoder.DecodeString()
			if err != nil {
				return nil, err
			}
			invocationMessage.StreamIds = append(invocationMessage.StreamIds, streamId)
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
			return nil, fmt.Errorf("invalid pingMessage length %v", msgLen)
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

func (m *messagePackHubProtocol) WriteMessage(message interface{}, writer io.Writer) (err error) {
	var buf bytes.Buffer
	encoder := msgpack.NewEncoder(&buf)
	switch msg := message.(type) {
	case invocationMessage:
		if err = encodeMsgHeader(encoder, 6, msg.Type); err != nil {
			return err
		}
		if msg.InvocationID == "" {
			if err = encoder.EncodeNil(); err != nil {
				return err
			}
		} else {
			if err = encoder.EncodeString(msg.InvocationID); err != nil {
				return err
			}
		}
		if err = encoder.EncodeString(msg.Target); err != nil {
			return err
		}
		if err = encoder.EncodeArrayLen(len(msg.Arguments)); err != nil {
			return err
		}
		for _, arg := range msg.Arguments {
			if err = encoder.Encode(arg); err != nil {
				return err
			}
		}
		if err = encoder.EncodeArrayLen(len(msg.StreamIds)); err != nil {
			return err
		}
		for _, id := range msg.StreamIds {
			if err = encoder.EncodeString(id); err != nil {
				return err
			}
		}
	case streamItemMessage:
		if err = encodeMsgHeader(encoder, 4, msg.Type); err != nil {
			return err
		}
		if err = encoder.EncodeString(msg.InvocationID); err != nil {
			return err
		}
		if err = encoder.Encode(msg.Item); err != nil {
			return err
		}
	case completionMessage:
		msgLen := 4
		if msg.Result != nil || msg.Error != "" {
			msgLen = 5
		}
		if err = encodeMsgHeader(encoder, msgLen, msg.Type); err != nil {
			return err
		}
		if err = encoder.EncodeString(msg.InvocationID); err != nil {
			return err
		}
		var resultKind int8 = 2
		if msg.Error != "" {
			resultKind = 1
		} else if msg.Result != nil {
			resultKind = 3
		}
		if err = encoder.EncodeInt8(resultKind); err != nil {
			return err
		}
		switch resultKind {
		case 1:
			if err = encoder.EncodeString(msg.Error); err != nil {
				return err
			}
		case 3:
			if err = encoder.Encode(msg.Result); err != nil {
				return err
			}
		}
	case cancelInvocationMessage:
		if err = encodeMsgHeader(encoder, 3, msg.Type); err != nil {
			return err
		}
		if err = encoder.EncodeString(msg.InvocationID); err != nil {
			return err
		}
	case hubMessage:
		if err = encoder.EncodeInt8(int8(6)); err != nil {
			return err
		}
	case closeMessage:
		if err = encodeMsgHeader(encoder, 3, msg.Type); err != nil {
			return err
		}
		if err = encoder.EncodeString(msg.Error); err != nil {
			return err
		}
		if err = encoder.EncodeBool(msg.AllowReconnect); err != nil {
			return err
		}
	}
	_, err = buf.WriteTo(writer)
	return err
}

func encodeMsgHeader(e *msgpack.Encoder, msgLen int, msgType int) (err error) {
	if err = e.EncodeArrayLen(msgLen); err != nil {
		return err
	}
	if err = e.EncodeInt8(int8(msgType)); err != nil {
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
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	return nil
}
