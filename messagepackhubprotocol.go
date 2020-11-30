package signalr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/vmihailenco/msgpack/v5"
	"io"
	"strconv"
)

type messagePackHubProtocol struct {
	dbg log.Logger
}

func (m *messagePackHubProtocol) ParseMessages(reader io.Reader, remainBuf *bytes.Buffer) ([]interface{}, error) {
	buf := bytes.Buffer{}
	_, _ = buf.ReadFrom(remainBuf)
	d := msgpack.NewDecoder(&buf)
	p := make([]byte, 1<<15)
	notYetDecoded := make([]byte, 0)
	for {
		buf.Write(notYetDecoded)
		n, err := reader.Read(p)
		if err != nil {
			return nil, err
		}
		_, _ = buf.Write(p[:n])
		msg, err := d.DecodeSlice()
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
		// Decode slice contents
		msgType8, ok := msg[0].(int8)
		if !ok {
			return nil, fmt.Errorf("invalid message. Can not read message type %v", msg[0])
		}
		message, err := m.parseMessage(int(msgType8), msg)
		if err != nil {
			return nil, err
		}
		return []interface{}{message}, nil
	}
}

func (m *messagePackHubProtocol) parseMessage(msgType int, msg []interface{}) (message interface{}, err error) {
	var ok bool
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

func (m *messagePackHubProtocol) WriteMessage(message interface{}, writer io.Writer) (err error) {
	var buf bytes.Buffer
	e := msgpack.NewEncoder(&buf)
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
	if err = e.EncodeMapLen(0); err != nil {
		return err
	}
	return nil
}

func (m *messagePackHubProtocol) setDebugLogger(dbg StructuredLogger) {
	m.dbg = log.WithPrefix(dbg, "ts", log.DefaultTimestampUTC, "protocol", "MSGP")
}

// UnmarshalArgument copies the value of a basic type to another basic type.
// dst must be a pointer to the destination instance.
// Copying string to numeric supports the numeric display types supported by strconv.ParseInt()
func (m *messagePackHubProtocol) UnmarshalArgument(src, dst interface{}) error {
	switch s := src.(type) {
	case string:
		return copyFromString(s, dst)
	case float32:
		copyFromFloat32(s, dst)
	case float64:
		copyFromFloat64(s, dst)
	case int8:
		copyFromInt8(s, dst)
	case int16:
		copyFromInt16(s, dst)
	case int32:
		copyFromInt32(s, dst)
	case int64:
		copyFromInt64(s, dst)
	case int:
		copyFromInt(s, dst)
	case uint8:
		copyFromUint8(s, dst)
	case uint16:
		copyFromUint16(s, dst)
	case uint32:
		copyFromUint32(s, dst)
	case uint64:
		copyFromUint64(s, dst)
	case uint:
		copyFromUint(s, dst)
	case []string:
		return copyFromStringSlice(s, dst)
	case []float32:
		copyFromFloat32Slice(s, dst)
	case []float64:
		copyFromFloat64Slice(s, dst)
	case []int8:
		copyFromInt8Slice(s, dst)
	case []int16:
		copyFromInt16Slice(s, dst)
	case []int32:
		copyFromInt32Slice(s, dst)
	case []int64:
		copyFromInt64Slice(s, dst)
	case []int:
		copyFromIntSlice(s, dst)
	case []uint8:
		copyFromUint8Slice(s, dst)
	case []uint16:
		copyFromUint16Slice(s, dst)
	case []uint32:
		copyFromUint32Slice(s, dst)
	case []uint64:
		copyFromUint64Slice(s, dst)
	case []uint:
		copyFromUintSlice(s, dst)
	}
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func copyFromString(s string, dst interface{}) error {
	switch d := dst.(type) {
	case *string:
		*d = s
		return nil
	case *float32:
		f, err := strconv.ParseFloat(s, 32)
		if err != nil {
			i, err := strconv.ParseInt(s, 0, 64)
			if err != nil {
				return err
			}
			*d = float32(i)
		} else {
			*d = float32(f)
		}
		return nil
	case *float64:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			i, err := strconv.ParseInt(s, 0, 64)
			if err != nil {
				return err
			}
			*d = float64(i)
		} else {
			*d = f
		}
		return nil
	case *int8:
		i, err := strconv.ParseInt(s, 0, 8)
		if err != nil {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return err
			}
			*d = int8(f)
		} else {
			*d = int8(i)
		}
		return nil
	case *int16:
		i, err := strconv.ParseInt(s, 0, 16)
		if err != nil {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return err
			}
			*d = int16(f)
		} else {
			*d = int16(i)
		}
		return nil
	case *int32:
		i, err := strconv.ParseInt(s, 0, 32)
		if err != nil {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return err
			}
			*d = int32(f)
		} else {
			*d = int32(i)
		}
		return nil
	case *int64:
		i, err := strconv.ParseInt(s, 0, 64)
		if err != nil {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return err
			}
			*d = int64(f)
		} else {
			*d = i
		}
		return nil
	case *int:
		i, err := strconv.ParseInt(s, 0, 64)
		if err != nil {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return err
			}
			*d = int(f)
		} else {
			*d = int(i)
		}
		return nil
	case *uint8:
		i, err := strconv.ParseUint(s, 0, 8)
		if err != nil {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return err
			}
			*d = uint8(f)
		} else {
			*d = uint8(i)
		}
		return nil
	case *uint16:
		i, err := strconv.ParseUint(s, 0, 16)
		if err != nil {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return err
			}
			*d = uint16(f)
		} else {
			*d = uint16(i)
		}
		return nil
	case *uint32:
		i, err := strconv.ParseUint(s, 0, 32)
		if err != nil {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return err
			}
			*d = uint32(f)
		} else {
			*d = uint32(i)
		}
		return nil
	case *uint64:
		i, err := strconv.ParseUint(s, 0, 64)
		if err != nil {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return err
			}
			*d = uint64(f)
		} else {
			*d = i
		}
		return nil
	case *uint:
		i, err := strconv.ParseUint(s, 0, 64)
		if err != nil {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return err
			}
			*d = uint(f)
		} else {
			*d = uint(i)
		}
		return nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func copyFromFloat32(s float32, dst interface{}) {
	switch d := dst.(type) {
	case *string:
		*d = fmt.Sprint(s)
	case *float32:
		*d = s
	case *float64:
		*d = float64(s)
	case *int8:
		*d = int8(s)
	case *int16:
		*d = int16(s)
	case *int32:
		*d = int32(s)
	case *int64:
		*d = int64(s)
	case *int:
		*d = int(s)
	case *uint8:
		*d = uint8(s)
	case *uint16:
		*d = uint16(s)
	case *uint32:
		*d = uint32(s)
	case *uint64:
		*d = uint64(s)
	case *uint:
		*d = uint(s)
	}
}

func copyFromFloat64(s float64, dst interface{}) {
	switch d := dst.(type) {
	case *string:
		*d = fmt.Sprint(s)
	case *float32:
		*d = float32(s)
	case *float64:
		*d = s
	case *int8:
		*d = int8(s)
	case *int16:
		*d = int16(s)
	case *int32:
		*d = int32(s)
	case *int64:
		*d = int64(s)
	case *int:
		*d = int(s)
	case *uint8:
		*d = uint8(s)
	case *uint16:
		*d = uint16(s)
	case *uint32:
		*d = uint32(s)
	case *uint64:
		*d = uint64(s)
	case *uint:
		*d = uint(s)
	}
}

func copyFromInt8(s int8, dst interface{}) {
	switch d := dst.(type) {
	case *string:
		*d = fmt.Sprint(s)
	case *float32:
		*d = float32(s)
	case *float64:
		*d = float64(s)
	case *int8:
		*d = s
	case *int16:
		*d = int16(s)
	case *int32:
		*d = int32(s)
	case *int64:
		*d = int64(s)
	case *int:
		*d = int(s)
	case *uint8:
		*d = uint8(s)
	case *uint16:
		*d = uint16(s)
	case *uint32:
		*d = uint32(s)
	case *uint64:
		*d = uint64(s)
	case *uint:
		*d = uint(s)
	}
}

func copyFromInt16(s int16, dst interface{}) {
	switch d := dst.(type) {
	case *string:
		*d = fmt.Sprint(s)
	case *float32:
		*d = float32(s)
	case *float64:
		*d = float64(s)
	case *int8:
		*d = int8(s)
	case *int16:
		*d = s
	case *int32:
		*d = int32(s)
	case *int64:
		*d = int64(s)
	case *int:
		*d = int(s)
	case *uint8:
		*d = uint8(s)
	case *uint16:
		*d = uint16(s)
	case *uint32:
		*d = uint32(s)
	case *uint64:
		*d = uint64(s)
	case *uint:
		*d = uint(s)
	}
}

func copyFromInt32(s int32, dst interface{}) {
	switch d := dst.(type) {
	case *string:
		*d = fmt.Sprint(s)
	case *float32:
		*d = float32(s)
	case *float64:
		*d = float64(s)
	case *int8:
		*d = int8(s)
	case *int16:
		*d = int16(s)
	case *int32:
		*d = s
	case *int64:
		*d = int64(s)
	case *int:
		*d = int(s)
	case *uint8:
		*d = uint8(s)
	case *uint16:
		*d = uint16(s)
	case *uint32:
		*d = uint32(s)
	case *uint64:
		*d = uint64(s)
	case *uint:
		*d = uint(s)
	}
}

func copyFromInt64(s int64, dst interface{}) {
	switch d := dst.(type) {
	case *string:
		*d = fmt.Sprint(s)
	case *float32:
		*d = float32(s)
	case *float64:
		*d = float64(s)
	case *int8:
		*d = int8(s)
	case *int16:
		*d = int16(s)
	case *int32:
		*d = int32(s)
	case *int64:
		*d = s
	case *int:
		*d = int(s)
	case *uint8:
		*d = uint8(s)
	case *uint16:
		*d = uint16(s)
	case *uint32:
		*d = uint32(s)
	case *uint64:
		*d = uint64(s)
	case *uint:
		*d = uint(s)
	}
}

func copyFromInt(s int, dst interface{}) {
	switch d := dst.(type) {
	case *string:
		*d = fmt.Sprint(s)
	case *float32:
		*d = float32(s)
	case *float64:
		*d = float64(s)
	case *int8:
		*d = int8(s)
	case *int16:
		*d = int16(s)
	case *int32:
		*d = int32(s)
	case *int64:
		*d = int64(s)
	case *int:
		*d = s
	case *uint8:
		*d = uint8(s)
	case *uint16:
		*d = uint16(s)
	case *uint32:
		*d = uint32(s)
	case *uint64:
		*d = uint64(s)
	case *uint:
		*d = uint(s)
	}
}

func copyFromUint8(s uint8, dst interface{}) {
	switch d := dst.(type) {
	case *string:
		*d = fmt.Sprint(s)
	case *float32:
		*d = float32(s)
	case *float64:
		*d = float64(s)
	case *int8:
		*d = int8(s)
	case *int16:
		*d = int16(s)
	case *int32:
		*d = int32(s)
	case *int64:
		*d = int64(s)
	case *int:
		*d = int(s)
	case *uint8:
		*d = s
	case *uint16:
		*d = uint16(s)
	case *uint32:
		*d = uint32(s)
	case *uint64:
		*d = uint64(s)
	case *uint:
		*d = uint(s)
	}
}

func copyFromUint16(s uint16, dst interface{}) {
	switch d := dst.(type) {
	case *string:
		*d = fmt.Sprint(s)
	case *float32:
		*d = float32(s)
	case *float64:
		*d = float64(s)
	case *int8:
		*d = int8(s)
	case *int16:
		*d = int16(s)
	case *int32:
		*d = int32(s)
	case *int64:
		*d = int64(s)
	case *int:
		*d = int(s)
	case *uint8:
		*d = uint8(s)
	case *uint16:
		*d = s
	case *uint32:
		*d = uint32(s)
	case *uint64:
		*d = uint64(s)
	case *uint:
		*d = uint(s)
	}
}

func copyFromUint32(s uint32, dst interface{}) {
	switch d := dst.(type) {
	case *string:
		*d = fmt.Sprint(s)
	case *float32:
		*d = float32(s)
	case *float64:
		*d = float64(s)
	case *int8:
		*d = int8(s)
	case *int16:
		*d = int16(s)
	case *int32:
		*d = int32(s)
	case *int64:
		*d = int64(s)
	case *int:
		*d = int(s)
	case *uint8:
		*d = uint8(s)
	case *uint16:
		*d = uint16(s)
	case *uint32:
		*d = s
	case *uint64:
		*d = uint64(s)
	case *uint:
		*d = uint(s)
	}
}

func copyFromUint64(s uint64, dst interface{}) {
	switch d := dst.(type) {
	case *string:
		*d = fmt.Sprint(s)
	case *float32:
		*d = float32(s)
	case *float64:
		*d = float64(s)
	case *int8:
		*d = int8(s)
	case *int16:
		*d = int16(s)
	case *int32:
		*d = int32(s)
	case *int64:
		*d = int64(s)
	case *int:
		*d = int(s)
	case *uint8:
		*d = uint8(s)
	case *uint16:
		*d = uint16(s)
	case *uint32:
		*d = uint32(s)
	case *uint64:
		*d = s
	case *uint:
		*d = uint(s)
	}
}

func copyFromUint(s uint, dst interface{}) {
	switch d := dst.(type) {
	case *string:
		*d = fmt.Sprint(s)
	case *float32:
		*d = float32(s)
	case *float64:
		*d = float64(s)
	case *int8:
		*d = int8(s)
	case *int16:
		*d = int16(s)
	case *int32:
		*d = int32(s)
	case *int64:
		*d = int64(s)
	case *int:
		*d = int(s)
	case *uint8:
		*d = uint8(s)
	case *uint16:
		*d = uint16(s)
	case *uint32:
		*d = uint32(s)
	case *uint64:
		*d = uint64(s)
	case *uint:
		*d = s
	}
}

func copyFromStringSlice(s []string, dst interface{}) error {
	switch d := dst.(type) {
	case *[]string:
		*d = s
		return nil
	case *[]float32:
		f := make([]float32, len(s))
		for _, se := range s {
			fe, err := strconv.ParseFloat(se, 32)
			if err != nil {
				return err
			}
			f = append(f, float32(fe))
		}
		*d = f
		return nil
	case *[]float64:
		f := make([]float64, len(s))
		for _, se := range s {
			fe, err := strconv.ParseFloat(se, 64)
			if err != nil {
				return err
			}
			f = append(f, fe)
		}
		*d = f
		return nil
	case *[]int8:
		i := make([]int8, len(s))
		for _, se := range s {
			ie, err := strconv.ParseInt(se, 0, 8)
			if err != nil {
				return err
			}
			i = append(i, int8(ie))
		}
		*d = i
		return nil
	case *[]int16:
		i := make([]int16, len(s))
		for _, se := range s {
			ie, err := strconv.ParseInt(se, 0, 16)
			if err != nil {
				return err
			}
			i = append(i, int16(ie))
		}
		*d = i
		return nil
	case *[]int32:
		i := make([]int32, len(s))
		for _, se := range s {
			ie, err := strconv.ParseInt(se, 0, 32)
			if err != nil {
				return err
			}
			i = append(i, int32(ie))
		}
		*d = i
		return nil
	case *[]int64:
		i := make([]int64, len(s))
		for _, se := range s {
			ie, err := strconv.ParseInt(se, 0, 64)
			if err != nil {
				return err
			}
			i = append(i, ie)
		}
		*d = i
		return nil
	case *[]int:
		i := make([]int, len(s))
		for _, se := range s {
			ie, err := strconv.ParseInt(se, 0, 64)
			if err != nil {
				return err
			}
			i = append(i, int(ie))
		}
		*d = i
		return nil
	case *[]uint8:
		i := make([]uint8, len(s))
		for _, se := range s {
			ie, err := strconv.ParseUint(se, 0, 8)
			if err != nil {
				return err
			}
			i = append(i, uint8(ie))
		}
		*d = i
		return nil
	case *[]uint16:
		i := make([]uint16, len(s))
		for _, se := range s {
			ie, err := strconv.ParseUint(se, 0, 16)
			if err != nil {
				return err
			}
			i = append(i, uint16(ie))
		}
		*d = i
		return nil
	case *[]uint32:
		i := make([]uint32, len(s))
		for _, se := range s {
			ie, err := strconv.ParseUint(se, 0, 32)
			if err != nil {
				return err
			}
			i = append(i, uint32(ie))
		}
		*d = i
		return nil
	case *[]uint64:
		i := make([]uint64, len(s))
		for _, se := range s {
			ie, err := strconv.ParseUint(se, 0, 64)
			if err != nil {
				return err
			}
			i = append(i, ie)
		}
		*d = i
		return nil
	case *[]uint:
		i := make([]uint, len(s))
		for _, se := range s {
			ie, err := strconv.ParseUint(se, 0, 64)
			if err != nil {
				return err
			}
			i = append(i, uint(ie))
		}
		*d = i
		return nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
func copyFromFloat32Slice(s []float32, dst interface{}) {
	switch d := dst.(type) {
	case *[]string:
		t := make([]string, len(s))
		for _, se := range s {
			t = append(t, fmt.Sprint(se))
		}
		*d = t
	case *[]float32:
		*d = s
	case *[]float64:
		f := make([]float64, len(s))
		for _, se := range s {
			f = append(f, float64(se))
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for _, se := range s {
			i = append(i, int8(se))
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for _, se := range s {
			i = append(i, int16(se))
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for _, se := range s {
			i = append(i, int32(se))
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for _, se := range s {
			i = append(i, int64(se))
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for _, se := range s {
			i = append(i, int(se))
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for _, se := range s {
			i = append(i, uint8(se))
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for _, se := range s {
			i = append(i, uint16(se))
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for _, se := range s {
			i = append(i, uint32(se))
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for _, se := range s {
			i = append(i, uint64(se))
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for _, se := range s {
			i = append(i, uint(se))
		}
		*d = i
	}
}
func copyFromFloat64Slice(s []float64, dst interface{}) {
	switch d := dst.(type) {
	case *[]string:
		t := make([]string, len(s))
		for _, se := range s {
			t = append(t, fmt.Sprint(se))
		}
		*d = t
	case *[]float32:
		f := make([]float32, len(s))
		for _, se := range s {
			f = append(f, float32(se))
		}
		*d = f
	case *[]float64:
		*d = s
	case *[]int8:
		i := make([]int8, len(s))
		for _, se := range s {
			i = append(i, int8(se))
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for _, se := range s {
			i = append(i, int16(se))
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for _, se := range s {
			i = append(i, int32(se))
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for _, se := range s {
			i = append(i, int64(se))
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for _, se := range s {
			i = append(i, int(se))
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for _, se := range s {
			i = append(i, uint8(se))
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for _, se := range s {
			i = append(i, uint16(se))
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for _, se := range s {
			i = append(i, uint32(se))
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for _, se := range s {
			i = append(i, uint64(se))
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for _, se := range s {
			i = append(i, uint(se))
		}
		*d = i
	}
}

func copyFromInt8Slice(s []int8, dst interface{}) {
	switch d := dst.(type) {
	case *[]string:
		t := make([]string, len(s))
		for _, se := range s {
			t = append(t, fmt.Sprint(se))
		}
		*d = t
	case *[]float32:
		f := make([]float32, len(s))
		for _, se := range s {
			f = append(f, float32(se))
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for _, se := range s {
			f = append(f, float64(se))
		}
		*d = f
	case *[]int8:
		*d = s
	case *[]int16:
		i := make([]int16, len(s))
		for _, se := range s {
			i = append(i, int16(se))
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for _, se := range s {
			i = append(i, int32(se))
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for _, se := range s {
			i = append(i, int64(se))
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for _, se := range s {
			i = append(i, int(se))
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for _, se := range s {
			i = append(i, uint8(se))
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for _, se := range s {
			i = append(i, uint16(se))
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for _, se := range s {
			i = append(i, uint32(se))
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for _, se := range s {
			i = append(i, uint64(se))
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for _, se := range s {
			i = append(i, uint(se))
		}
		*d = i
	}
}

func copyFromInt16Slice(s []int16, dst interface{}) {
	switch d := dst.(type) {
	case *[]string:
		t := make([]string, len(s))
		for _, se := range s {
			t = append(t, fmt.Sprint(se))
		}
		*d = t
	case *[]float32:
		f := make([]float32, len(s))
		for _, se := range s {
			f = append(f, float32(se))
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for _, se := range s {
			f = append(f, float64(se))
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for _, se := range s {
			i = append(i, int8(se))
		}
		*d = i
	case *[]int16:
		*d = s
	case *[]int32:
		i := make([]int32, len(s))
		for _, se := range s {
			i = append(i, int32(se))
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for _, se := range s {
			i = append(i, int64(se))
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for _, se := range s {
			i = append(i, int(se))
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for _, se := range s {
			i = append(i, uint8(se))
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for _, se := range s {
			i = append(i, uint16(se))
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for _, se := range s {
			i = append(i, uint32(se))
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for _, se := range s {
			i = append(i, uint64(se))
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for _, se := range s {
			i = append(i, uint(se))
		}
		*d = i
	}
}

func copyFromInt32Slice(s []int32, dst interface{}) {
	switch d := dst.(type) {
	case *[]string:
		t := make([]string, len(s))
		for _, se := range s {
			t = append(t, fmt.Sprint(se))
		}
		*d = t
	case *[]float32:
		f := make([]float32, len(s))
		for _, se := range s {
			f = append(f, float32(se))
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for _, se := range s {
			f = append(f, float64(se))
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for _, se := range s {
			i = append(i, int8(se))
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for _, se := range s {
			i = append(i, int16(se))
		}
		*d = i
	case *[]int32:
		*d = s
	case *[]int64:
		i := make([]int64, len(s))
		for _, se := range s {
			i = append(i, int64(se))
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for _, se := range s {
			i = append(i, int(se))
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for _, se := range s {
			i = append(i, uint8(se))
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for _, se := range s {
			i = append(i, uint16(se))
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for _, se := range s {
			i = append(i, uint32(se))
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for _, se := range s {
			i = append(i, uint64(se))
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for _, se := range s {
			i = append(i, uint(se))
		}
		*d = i
	}
}

func copyFromInt64Slice(s []int64, dst interface{}) {
	switch d := dst.(type) {
	case *[]string:
		t := make([]string, len(s))
		for _, se := range s {
			t = append(t, fmt.Sprint(se))
		}
		*d = t
	case *[]float32:
		f := make([]float32, len(s))
		for _, se := range s {
			f = append(f, float32(se))
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for _, se := range s {
			f = append(f, float64(se))
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for _, se := range s {
			i = append(i, int8(se))
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for _, se := range s {
			i = append(i, int16(se))
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for _, se := range s {
			i = append(i, int32(se))
		}
		*d = i
	case *[]int64:
		*d = s
	case *[]int:
		i := make([]int, len(s))
		for _, se := range s {
			i = append(i, int(se))
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for _, se := range s {
			i = append(i, uint8(se))
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for _, se := range s {
			i = append(i, uint16(se))
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for _, se := range s {
			i = append(i, uint32(se))
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for _, se := range s {
			i = append(i, uint64(se))
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for _, se := range s {
			i = append(i, uint(se))
		}
		*d = i
	}
}

func copyFromIntSlice(s []int, dst interface{}) {
	switch d := dst.(type) {
	case *[]string:
		t := make([]string, len(s))
		for _, se := range s {
			t = append(t, fmt.Sprint(se))
		}
		*d = t
	case *[]float32:
		f := make([]float32, len(s))
		for _, se := range s {
			f = append(f, float32(se))
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for _, se := range s {
			f = append(f, float64(se))
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for _, se := range s {
			i = append(i, int8(se))
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for _, se := range s {
			i = append(i, int16(se))
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for _, se := range s {
			i = append(i, int32(se))
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for _, se := range s {
			i = append(i, int64(se))
		}
		*d = i
	case *[]int:
		*d = s
	case *[]uint8:
		i := make([]uint8, len(s))
		for _, se := range s {
			i = append(i, uint8(se))
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for _, se := range s {
			i = append(i, uint16(se))
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for _, se := range s {
			i = append(i, uint32(se))
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for _, se := range s {
			i = append(i, uint64(se))
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for _, se := range s {
			i = append(i, uint(se))
		}
		*d = i
	}
}

func copyFromUint8Slice(s []uint8, dst interface{}) {
	switch d := dst.(type) {
	case *[]string:
		t := make([]string, len(s))
		for _, se := range s {
			t = append(t, fmt.Sprint(se))
		}
		*d = t
	case *[]float32:
		f := make([]float32, len(s))
		for _, se := range s {
			f = append(f, float32(se))
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for _, se := range s {
			f = append(f, float64(se))
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for _, se := range s {
			i = append(i, int8(se))
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for _, se := range s {
			i = append(i, int16(se))
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for _, se := range s {
			i = append(i, int32(se))
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for _, se := range s {
			i = append(i, int64(se))
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for _, se := range s {
			i = append(i, int(se))
		}
		*d = i
	case *[]uint8:
		*d = s
	case *[]uint16:
		i := make([]uint16, len(s))
		for _, se := range s {
			i = append(i, uint16(se))
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for _, se := range s {
			i = append(i, uint32(se))
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for _, se := range s {
			i = append(i, uint64(se))
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for _, se := range s {
			i = append(i, uint(se))
		}
		*d = i
	}
}

func copyFromUint16Slice(s []uint16, dst interface{}) {
	switch d := dst.(type) {
	case *[]string:
		t := make([]string, len(s))
		for _, se := range s {
			t = append(t, fmt.Sprint(se))
		}
		*d = t
	case *[]float32:
		f := make([]float32, len(s))
		for _, se := range s {
			f = append(f, float32(se))
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for _, se := range s {
			f = append(f, float64(se))
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for _, se := range s {
			i = append(i, int8(se))
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for _, se := range s {
			i = append(i, int16(se))
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for _, se := range s {
			i = append(i, int32(se))
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for _, se := range s {
			i = append(i, int64(se))
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for _, se := range s {
			i = append(i, int(se))
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for _, se := range s {
			i = append(i, uint8(se))
		}
		*d = i
	case *[]uint16:
		*d = s
	case *[]uint32:
		i := make([]uint32, len(s))
		for _, se := range s {
			i = append(i, uint32(se))
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for _, se := range s {
			i = append(i, uint64(se))
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for _, se := range s {
			i = append(i, uint(se))
		}
		*d = i
	}
}

func copyFromUint32Slice(s []uint32, dst interface{}) {
	switch d := dst.(type) {
	case *[]string:
		t := make([]string, len(s))
		for _, se := range s {
			t = append(t, fmt.Sprint(se))
		}
		*d = t
	case *[]float32:
		f := make([]float32, len(s))
		for _, se := range s {
			f = append(f, float32(se))
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for _, se := range s {
			f = append(f, float64(se))
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for _, se := range s {
			i = append(i, int8(se))
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for _, se := range s {
			i = append(i, int16(se))
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for _, se := range s {
			i = append(i, int32(se))
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for _, se := range s {
			i = append(i, int64(se))
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for _, se := range s {
			i = append(i, int(se))
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for _, se := range s {
			i = append(i, uint8(se))
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for _, se := range s {
			i = append(i, uint16(se))
		}
		*d = i
	case *[]uint32:
		*d = s
	case *[]uint64:
		i := make([]uint64, len(s))
		for _, se := range s {
			i = append(i, uint64(se))
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for _, se := range s {
			i = append(i, uint(se))
		}
		*d = i
	}
}

func copyFromUint64Slice(s []uint64, dst interface{}) {
	switch d := dst.(type) {
	case *[]string:
		t := make([]string, len(s))
		for _, se := range s {
			t = append(t, fmt.Sprint(se))
		}
		*d = t
	case *[]float32:
		f := make([]float32, len(s))
		for _, se := range s {
			f = append(f, float32(se))
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for _, se := range s {
			f = append(f, float64(se))
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for _, se := range s {
			i = append(i, int8(se))
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for _, se := range s {
			i = append(i, int16(se))
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for _, se := range s {
			i = append(i, int32(se))
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for _, se := range s {
			i = append(i, int64(se))
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for _, se := range s {
			i = append(i, int(se))
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for _, se := range s {
			i = append(i, uint8(se))
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for _, se := range s {
			i = append(i, uint16(se))
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for _, se := range s {
			i = append(i, uint32(se))
		}
		*d = i
	case *[]uint64:
		*d = s
	case *[]uint:
		i := make([]uint, len(s))
		for _, se := range s {
			i = append(i, uint(se))
		}
		*d = i
	}
}

func copyFromUintSlice(s []uint, dst interface{}) {
	switch d := dst.(type) {
	case *[]string:
		t := make([]string, len(s))
		for _, se := range s {
			t = append(t, fmt.Sprint(se))
		}
		*d = t
	case *[]float32:
		f := make([]float32, len(s))
		for _, se := range s {
			f = append(f, float32(se))
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for _, se := range s {
			f = append(f, float64(se))
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for _, se := range s {
			i = append(i, int8(se))
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for _, se := range s {
			i = append(i, int16(se))
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for _, se := range s {
			i = append(i, int32(se))
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for _, se := range s {
			i = append(i, int64(se))
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for _, se := range s {
			i = append(i, int(se))
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for _, se := range s {
			i = append(i, uint8(se))
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for _, se := range s {
			i = append(i, uint16(se))
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for _, se := range s {
			i = append(i, uint32(se))
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for _, se := range s {
			i = append(i, uint64(se))
		}
		*d = i
	case *[]uint:
		*d = s
	}
}
