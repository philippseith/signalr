package signalr

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/vmihailenco/msgpack/v5"
	"io"
	"reflect"
)

type messagePackHubProtocol struct {
	dbg log.Logger
}

func (m *messagePackHubProtocol) ParseMessages(reader io.Reader, remainBuf *bytes.Buffer) ([]interface{}, error) {
	buf := bytes.Buffer{}
	_, _ = buf.ReadFrom(remainBuf)
	decoder := msgpack.NewDecoder(&buf)
	p := make([]byte, 1<<15)
	notYetDecoded := make([]byte, 0)
	for {
		buf.Write(notYetDecoded)
		n, err := reader.Read(p)
		if err != nil {
			return nil, err
		}
		_, _ = buf.Write(p[:n])
		msg, err := decoder.DecodeSlice()
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
func (m *messagePackHubProtocol) UnmarshalArgument(src, dst interface{}) error {
	// If dst point towards an interface{} value, assigning is easy
	if dstPtr, ok := dst.(*interface{}); ok {
		*dstPtr = src
		return nil
	}
	switch s := src.(type) {
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
	default:
		rDst := reflect.ValueOf(dst)
		if rDst.Kind() != reflect.Ptr {
			return fmt.Errorf("dst ist not a pointer but %T", dst)
		}
		if reflect.TypeOf(src).AssignableTo(rDst.Elem().Type()) {
			rDst.Elem().Set(reflect.ValueOf(src))
		}
	}
	return nil
}

func copyFromFloat32(s float32, dst interface{}) {
	switch d := dst.(type) {
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

func copyFromFloat32Slice(s []float32, dst interface{}) {
	switch d := dst.(type) {
	case *[]float32:
		*d = s
	case *[]float64:
		f := make([]float64, len(s))
		for j, se := range s {
			f[j] = float64(se)
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for j, se := range s {
			i[j] = int8(se)
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for j, se := range s {
			i[j] = int16(se)
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for j, se := range s {
			i[j] = int32(se)
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for j, se := range s {
			i[j] = int64(se)
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for j, se := range s {
			i[j] = int(se)
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for j, se := range s {
			i[j] = uint8(se)
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for j, se := range s {
			i[j] = uint16(se)
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for j, se := range s {
			i[j] = uint32(se)
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for j, se := range s {
			i[j] = uint64(se)
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for j, se := range s {
			i[j] = uint(se)
		}
		*d = i
	}
}

func copyFromFloat64Slice(s []float64, dst interface{}) {
	switch d := dst.(type) {
	case *[]float32:
		f := make([]float32, len(s))
		for j, se := range s {
			f[j] = float32(se)
		}
		*d = f
	case *[]float64:
		*d = s
	case *[]int8:
		i := make([]int8, len(s))
		for j, se := range s {
			i[j] = int8(se)
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for j, se := range s {
			i[j] = int16(se)
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for j, se := range s {
			i[j] = int32(se)
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for j, se := range s {
			i[j] = int64(se)
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for j, se := range s {
			i[j] = int(se)
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for j, se := range s {
			i[j] = uint8(se)
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for j, se := range s {
			i[j] = uint16(se)
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for j, se := range s {
			i[j] = uint32(se)
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for j, se := range s {
			i[j] = uint64(se)
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for j, se := range s {
			i[j] = uint(se)
		}
		*d = i
	}
}

func copyFromInt8Slice(s []int8, dst interface{}) {
	switch d := dst.(type) {
	case *[]float32:
		f := make([]float32, len(s))
		for j, se := range s {
			f[j] = float32(se)
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for j, se := range s {
			f[j] = float64(se)
		}
		*d = f
	case *[]int8:
		*d = s
	case *[]int16:
		i := make([]int16, len(s))
		for j, se := range s {
			i[j] = int16(se)
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for j, se := range s {
			i[j] = int32(se)
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for j, se := range s {
			i[j] = int64(se)
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for j, se := range s {
			i[j] = int(se)
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for j, se := range s {
			i[j] = uint8(se)
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for j, se := range s {
			i[j] = uint16(se)
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for j, se := range s {
			i[j] = uint32(se)
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for j, se := range s {
			i[j] = uint64(se)
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for j, se := range s {
			i[j] = uint(se)
		}
		*d = i
	}
}

func copyFromInt16Slice(s []int16, dst interface{}) {
	switch d := dst.(type) {
	case *[]float32:
		f := make([]float32, len(s))
		for j, se := range s {
			f[j] = float32(se)
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for j, se := range s {
			f[j] = float64(se)
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for j, se := range s {
			i[j] = int8(se)
		}
		*d = i
	case *[]int16:
		*d = s
	case *[]int32:
		i := make([]int32, len(s))
		for j, se := range s {
			i[j] = int32(se)
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for j, se := range s {
			i[j] = int64(se)
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for j, se := range s {
			i[j] = int(se)
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for j, se := range s {
			i[j] = uint8(se)
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for j, se := range s {
			i[j] = uint16(se)
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for j, se := range s {
			i[j] = uint32(se)
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for j, se := range s {
			i[j] = uint64(se)
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for j, se := range s {
			i[j] = uint(se)
		}
		*d = i
	}
}

func copyFromInt32Slice(s []int32, dst interface{}) {
	switch d := dst.(type) {
	case *[]float32:
		f := make([]float32, len(s))
		for j, se := range s {
			f[j] = float32(se)
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for j, se := range s {
			f[j] = float64(se)
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for j, se := range s {
			i[j] = int8(se)
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for j, se := range s {
			i[j] = int16(se)
		}
		*d = i
	case *[]int32:
		*d = s
	case *[]int64:
		i := make([]int64, len(s))
		for j, se := range s {
			i[j] = int64(se)
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for j, se := range s {
			i[j] = int(se)
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for j, se := range s {
			i[j] = uint8(se)
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for j, se := range s {
			i[j] = uint16(se)
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for j, se := range s {
			i[j] = uint32(se)
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for j, se := range s {
			i[j] = uint64(se)
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for j, se := range s {
			i[j] = uint(se)
		}
		*d = i
	}
}

func copyFromInt64Slice(s []int64, dst interface{}) {
	switch d := dst.(type) {
	case *[]float32:
		f := make([]float32, len(s))
		for j, se := range s {
			f[j] = float32(se)
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for j, se := range s {
			f[j] = float64(se)
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for j, se := range s {
			i[j] = int8(se)
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for j, se := range s {
			i[j] = int16(se)
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for j, se := range s {
			i[j] = int32(se)
		}
		*d = i
	case *[]int64:
		*d = s
	case *[]int:
		i := make([]int, len(s))
		for j, se := range s {
			i[j] = int(se)
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for j, se := range s {
			i[j] = uint8(se)
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for j, se := range s {
			i[j] = uint16(se)
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for j, se := range s {
			i[j] = uint32(se)
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for j, se := range s {
			i[j] = uint64(se)
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for j, se := range s {
			i[j] = uint(se)
		}
		*d = i
	}
}

func copyFromIntSlice(s []int, dst interface{}) {
	switch d := dst.(type) {
	case *[]float32:
		f := make([]float32, len(s))
		for j, se := range s {
			f[j] = float32(se)
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for j, se := range s {
			f[j] = float64(se)
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for j, se := range s {
			i[j] = int8(se)
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for j, se := range s {
			i[j] = int16(se)
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for j, se := range s {
			i[j] = int32(se)
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for j, se := range s {
			i[j] = int64(se)
		}
		*d = i
	case *[]int:
		*d = s
	case *[]uint8:
		i := make([]uint8, len(s))
		for j, se := range s {
			i[j] = uint8(se)
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for j, se := range s {
			i[j] = uint16(se)
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for j, se := range s {
			i[j] = uint32(se)
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for j, se := range s {
			i[j] = uint64(se)
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for j, se := range s {
			i[j] = uint(se)
		}
		*d = i
	}
}

func copyFromUint8Slice(s []uint8, dst interface{}) {
	switch d := dst.(type) {
	case *[]float32:
		f := make([]float32, len(s))
		for j, se := range s {
			f[j] = float32(se)
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for j, se := range s {
			f[j] = float64(se)
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for j, se := range s {
			i[j] = int8(se)
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for j, se := range s {
			i[j] = int16(se)
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for j, se := range s {
			i[j] = int32(se)
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for j, se := range s {
			i[j] = int64(se)
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for j, se := range s {
			i[j] = int(se)
		}
		*d = i
	case *[]uint8:
		*d = s
	case *[]uint16:
		i := make([]uint16, len(s))
		for j, se := range s {
			i[j] = uint16(se)
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for j, se := range s {
			i[j] = uint32(se)
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for j, se := range s {
			i[j] = uint64(se)
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for j, se := range s {
			i[j] = uint(se)
		}
		*d = i
	}
}

func copyFromUint16Slice(s []uint16, dst interface{}) {
	switch d := dst.(type) {
	case *[]float32:
		f := make([]float32, len(s))
		for j, se := range s {
			f[j] = float32(se)
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for j, se := range s {
			f[j] = float64(se)
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for j, se := range s {
			i[j] = int8(se)
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for j, se := range s {
			i[j] = int16(se)
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for j, se := range s {
			i[j] = int32(se)
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for j, se := range s {
			i[j] = int64(se)
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for j, se := range s {
			i[j] = int(se)
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for j, se := range s {
			i[j] = uint8(se)
		}
		*d = i
	case *[]uint16:
		*d = s
	case *[]uint32:
		i := make([]uint32, len(s))
		for j, se := range s {
			i[j] = uint32(se)
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for j, se := range s {
			i[j] = uint64(se)
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for j, se := range s {
			i[j] = uint(se)
		}
		*d = i
	}
}

func copyFromUint32Slice(s []uint32, dst interface{}) {
	switch d := dst.(type) {
	case *[]float32:
		f := make([]float32, len(s))
		for j, se := range s {
			f[j] = float32(se)
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for j, se := range s {
			f[j] = float64(se)
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for j, se := range s {
			i[j] = int8(se)
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for j, se := range s {
			i[j] = int16(se)
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for j, se := range s {
			i[j] = int32(se)
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for j, se := range s {
			i[j] = int64(se)
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for j, se := range s {
			i[j] = int(se)
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for j, se := range s {
			i[j] = uint8(se)
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for j, se := range s {
			i[j] = uint16(se)
		}
		*d = i
	case *[]uint32:
		*d = s
	case *[]uint64:
		i := make([]uint64, len(s))
		for j, se := range s {
			i[j] = uint64(se)
		}
		*d = i
	case *[]uint:
		i := make([]uint, len(s))
		for j, se := range s {
			i[j] = uint(se)
		}
		*d = i
	}
}

func copyFromUint64Slice(s []uint64, dst interface{}) {
	switch d := dst.(type) {
	case *[]float32:
		f := make([]float32, len(s))
		for j, se := range s {
			f[j] = float32(se)
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for j, se := range s {
			f[j] = float64(se)
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for j, se := range s {
			i[j] = int8(se)
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for j, se := range s {
			i[j] = int16(se)
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for j, se := range s {
			i[j] = int32(se)
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for j, se := range s {
			i[j] = int64(se)
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for j, se := range s {
			i[j] = int(se)
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for j, se := range s {
			i[j] = uint8(se)
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for j, se := range s {
			i[j] = uint16(se)
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for j, se := range s {
			i[j] = uint32(se)
		}
		*d = i
	case *[]uint64:
		*d = s
	case *[]uint:
		i := make([]uint, len(s))
		for j, se := range s {
			i[j] = uint(se)
		}
		*d = i
	}
}

func copyFromUintSlice(s []uint, dst interface{}) {
	switch d := dst.(type) {
	case *[]float32:
		f := make([]float32, len(s))
		for j, se := range s {
			f[j] = float32(se)
		}
		*d = f
	case *[]float64:
		f := make([]float64, len(s))
		for j, se := range s {
			f[j] = float64(se)
		}
		*d = f
	case *[]int8:
		i := make([]int8, len(s))
		for j, se := range s {
			i[j] = int8(se)
		}
		*d = i
	case *[]int16:
		i := make([]int16, len(s))
		for j, se := range s {
			i[j] = int16(se)
		}
		*d = i
	case *[]int32:
		i := make([]int32, len(s))
		for j, se := range s {
			i[j] = int32(se)
		}
		*d = i
	case *[]int64:
		i := make([]int64, len(s))
		for j, se := range s {
			i[j] = int64(se)
		}
		*d = i
	case *[]int:
		i := make([]int, len(s))
		for j, se := range s {
			i[j] = int(se)
		}
		*d = i
	case *[]uint8:
		i := make([]uint8, len(s))
		for j, se := range s {
			i[j] = uint8(se)
		}
		*d = i
	case *[]uint16:
		i := make([]uint16, len(s))
		for j, se := range s {
			i[j] = uint16(se)
		}
		*d = i
	case *[]uint32:
		i := make([]uint32, len(s))
		for j, se := range s {
			i[j] = uint32(se)
		}
		*d = i
	case *[]uint64:
		i := make([]uint64, len(s))
		for j, se := range s {
			i[j] = uint64(se)
		}
		*d = i
	case *[]uint:
		*d = s
	}
}
