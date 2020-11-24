package signalr

import (
	"errors"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/vmihailenco/msgpack/v5"
	"io"
	"reflect"
)

type MessagePackHubProtocol struct {
	dbg log.Logger
}

func (m *MessagePackHubProtocol) ParseMessage(reader io.Reader) (interface{}, error) {
	d := msgpack.NewDecoder(reader)

	msg, err := d.DecodeSlice()
	if err != nil {
		return nil, err
	}
	if len(msg) == 0 {
		return nil, errors.New("invalid message length 0")
	}
	msgType8, ok := msg[0].(int8)
	if !ok {
		return nil, fmt.Errorf("invalid message. Can not read message type %v", msg[0])
	}
	msgType := int(msgType8)
	switch msgType {
	case 1, 4:
		if len(msg) != 6 {
			return nil, fmt.Errorf("invalid invocationMessage length %v", len(msg))
		}
		invocationMessage := invocationMessage{Type: msgType}
		if msg[2] != nil {
			if invocationMessage.InvocationID, ok = msg[2].(string); !ok {
				return nil, fmt.Errorf("invalid InvocationId %#v", msg[2])
			}
		}
		if invocationMessage.Target, ok = msg[3].(string); !ok {
			return nil, fmt.Errorf("invalid Target %#v", msg[3])
		}
		if invocationMessage.Arguments, ok = msg[4].([]interface{}); !ok {
			return nil, fmt.Errorf("invalid Arguments %#v", msg[4])
		}
		var streamIds []interface{}
		if streamIds, ok = msg[5].([]interface{}); !ok {
			return nil, fmt.Errorf("invalid streamIds %#v", msg[5])
		}
		for _, rawId := range streamIds {
			if streamId, ok := rawId.(string); ok {
				invocationMessage.StreamIds = append(invocationMessage.StreamIds, streamId)
			} else {
				return nil, fmt.Errorf("invalid StreamId %#v", rawId)
			}
		}
		return invocationMessage, nil
	case 2:
		if len(msg) != 4 {
			return nil, fmt.Errorf("invalid streamItemMessage length %v", len(msg))
		}
		streamItemMessage := streamItemMessage{
			Type: 2,
			Item: msg[3],
		}
		if streamItemMessage.InvocationID, ok = msg[2].(string); !ok {
			return nil, fmt.Errorf("invalid InvocationId %#v", msg[2])
		}
		return streamItemMessage, nil
	case 3:
		if len(msg) < 5 {
			return nil, fmt.Errorf("invalid completionMessage length %v", len(msg))
		}
		completionMessage := completionMessage{Type: 3}
		if completionMessage.InvocationID, ok = msg[2].(string); !ok {
			return nil, fmt.Errorf("invalid InvocationId %#v", msg[2])
		}
		resultKind, ok := msg[3].(int8)
		if !ok {
			return nil, fmt.Errorf("invalid resultKind %#v", msg[3])
		}
		switch resultKind {
		case 1:
			if len(msg) < 5 {
				return nil, fmt.Errorf("invalid completionMessage length %v", len(msg))
			}
			if completionMessage.Error, ok = msg[4].(string); !ok {
				return nil, fmt.Errorf("invalid Error %#v", msg[4])
			}
		case 2:
			// OK
		case 3:
			if len(msg) < 5 {
				return nil, fmt.Errorf("invalid completionMessage length %v", len(msg))
			}
			completionMessage.Result = msg[4]
		default:
			return nil, fmt.Errorf("invalid resultKind %v", resultKind)
		}
		return completionMessage, nil
	case 5:
		if len(msg) != 3 {
			return nil, fmt.Errorf("invalid completionMessage length %v", len(msg))
		}
		cancelInvocationMessage := cancelInvocationMessage{Type: 5}
		if cancelInvocationMessage.InvocationID, ok = msg[2].(string); !ok {
			return nil, fmt.Errorf("invalid InvocationId %#v", msg[2])
		}
		return cancelInvocationMessage, nil
	case 6:
		if len(msg) != 1 {
			return nil, fmt.Errorf("invalid pingMessage length %v", len(msg))
		}
		return hubMessage{Type: 6}, nil
	case 7:
		if len(msg) < 2 {
			return nil, fmt.Errorf("invalid pingMessage length %v", len(msg))
		}
		closeMessage := closeMessage{Type: 7}
		if closeMessage.Error, ok = msg[1].(string); !ok {
			return nil, fmt.Errorf("invalid Error %#v", msg[2])
		}
		if len(msg) > 2 {
			if closeMessage.AllowReconnect, ok = msg[2].(bool); !ok {
				return nil, fmt.Errorf("invalid AllowReconnect %#v", msg[2])
			}
		}
		return closeMessage, nil
	}
	return nil, fmt.Errorf("unknown message type %v", msgType)
}

func (m *MessagePackHubProtocol) WriteMessage(message interface{}, writer io.Writer) (err error) {
	e := msgpack.NewEncoder(writer)
	switch msg := message.(type) {
	case invocationMessage:
		if err = encodeMsgHeader(e, 6, msg.Type); err != nil {
			return err
		}
		if msg.InvocationID == "" {
			if err = e.EncodeNil(); err != nil {
				return err
			}
		} else {
			if err = e.EncodeString(msg.InvocationID); err != nil {
				return err
			}
		}
		if err = e.EncodeString(msg.Target); err != nil {
			return err
		}
		if err = e.EncodeArrayLen(len(msg.Arguments)); err != nil {
			return err
		}
		for _, arg := range msg.Arguments {
			if err = e.Encode(arg); err != nil {
				return err
			}
		}
		if err = e.EncodeArrayLen(len(msg.StreamIds)); err != nil {
			return err
		}
		for _, id := range msg.StreamIds {
			if err = e.EncodeString(id); err != nil {
				return err
			}
		}
	case streamItemMessage:
		if err = encodeMsgHeader(e, 4, msg.Type); err != nil {
			return err
		}
		if err = e.EncodeString(msg.InvocationID); err != nil {
			return err
		}
		if err = e.Encode(msg.Item); err != nil {
			return err
		}
	case completionMessage:
		msgLen := 4
		if msg.Result != nil || msg.Error != "" {
			msgLen = 5
		}
		if err = encodeMsgHeader(e, msgLen, msg.Type); err != nil {
			return err
		}
		if err = e.EncodeString(msg.InvocationID); err != nil {
			return err
		}
		var resultKind int8 = 2
		if msg.Error != "" {
			resultKind = 1
		} else if msg.Result != nil {
			resultKind = 3
		}
		if err = e.EncodeInt8(resultKind); err != nil {
			return err
		}
		switch resultKind {
		case 1:
			if err = e.EncodeString(msg.Error); err != nil {
				return err
			}
		case 3:
			if err = e.Encode(msg.Result); err != nil {
				return err
			}
		}
	case cancelInvocationMessage:
		if err = encodeMsgHeader(e, 3, msg.Type); err != nil {
			return err
		}
		if err = e.EncodeString(msg.InvocationID); err != nil {
			return err
		}
	case hubMessage:
		if err = e.EncodeInt8(int8(6)); err != nil {
			return err
		}
	case closeMessage:
		if err = encodeMsgHeader(e, 3, msg.Type); err != nil {
			return err
		}
		if err = e.EncodeString(msg.Error); err != nil {
			return err
		}
		if err = e.EncodeBool(msg.AllowReconnect); err != nil {
			return err
		}
	}
	return nil
}

func encodeMsgHeader(e *msgpack.Encoder, msgLen int, msgType int) (err error) {
	if err = e.EncodeArrayLen(msgLen); err != nil {
		return err
	}
	if err = e.EncodeInt8(int8(msgType)); err != nil {
		return err
	}
	if err = e.EncodeMapLen(0); err != nil {
		return err
	}
	return nil
}

func (m *MessagePackHubProtocol) UnmarshalArgument(argument interface{}, value interface{}) error {
	v := reflect.Indirect(reflect.ValueOf(value))
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(reflect.ValueOf(argument).Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(reflect.ValueOf(argument).Uint())
	case reflect.String:
		v.Set(reflect.ValueOf(argument))
	case reflect.Array, reflect.Slice:
	}
	return nil
}

func (m *MessagePackHubProtocol) setDebugLogger(dbg StructuredLogger) {
	m.dbg = log.WithPrefix(dbg, "ts", log.DefaultTimestampUTC, "protocol", "MSGP")
}
