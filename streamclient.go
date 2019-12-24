package signalr

import (
	"fmt"
	"reflect"
)

func newStreamClient() *streamClient {
	return &streamClient{make(map[string]reflect.Value)}
}

type streamClient struct {
	upstreamChannels map[string]reflect.Value
}

func (u *streamClient) buildChannelArgument(invocation invocationMessage, argType reflect.Type, chanCount int) (arg reflect.Value, canClientStreaming bool, err error) {
	if argType.Kind() != reflect.Chan || argType.ChanDir() == reflect.SendDir {
		return reflect.Value{}, false,nil
	} else if len(invocation.StreamIds) > chanCount {
		// MakeChan does only accept bidirectional channels and we need to Send to this channel anyway
		arg = reflect.MakeChan(reflect.ChanOf(reflect.BothDir, argType.Elem()), 0)
		u.upstreamChannels[invocation.StreamIds[chanCount]] = arg
		return arg, true, nil
	} else {
		// To many channel parameters arguments this method. The client will not send streamItems for these
		return reflect.Value{}, true, fmt.Errorf("method %s has more chan parameters than the client will stream", invocation.Target)
	}
}

func (u *streamClient) receiveStreamItem(streamItem streamItemMessage) {
	if upChan, ok := u.upstreamChannels[streamItem.InvocationID]; ok {
		// Hack(?) for missing channel type information when the Protocol decodes StreamItem.Item
		// Protocol specific, as only json has this inexact number type. Messagepack might cause different problems
		if f, ok := streamItem.Item.(float64); ok {
			// This type of solution is constrained to basic types, e.g. chan MyInt is not supported
			chanElm := reflect.Indirect(reflect.New(upChan.Type().Elem())).Interface()
			switch chanElm.(type) {
			case int:
				upChan.Send(reflect.ValueOf(int(f)))
			case int8:
				upChan.Send(reflect.ValueOf(int8(f)))
			case int16:
				upChan.Send(reflect.ValueOf(int16(f)))
			case int32:
				upChan.Send(reflect.ValueOf(int32(f)))
			case int64:
				upChan.Send(reflect.ValueOf(int64(f)))
			case uint:
				upChan.Send(reflect.ValueOf(uint(f)))
			case uint8:
				upChan.Send(reflect.ValueOf(uint8(f)))
			case uint16:
				upChan.Send(reflect.ValueOf(uint16(f)))
			case uint32:
				upChan.Send(reflect.ValueOf(uint32(f)))
			case uint64:
				upChan.Send(reflect.ValueOf(uint64(f)))
			case float32:
				upChan.Send(reflect.ValueOf(float32(f)))
			case float64:
				upChan.Send(reflect.ValueOf(f))
			case string:
				upChan.Send(reflect.ValueOf(fmt.Sprint(f)))
			}
		} else {
			upChan.Send(reflect.ValueOf(streamItem.Item))
		}
	}
}

func (u *streamClient) receiveCompletionItem(completion completionMessage) {
	channel := u.upstreamChannels[completion.InvocationID]
	channel.Close()
	delete(u.upstreamChannels, completion.InvocationID)
}

