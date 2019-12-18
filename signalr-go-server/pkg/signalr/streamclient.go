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
		upChan.Send(reflect.ValueOf(streamItem.Item))
	}
}

func (u *streamClient) receiveCompletionItem(completion completionMessage) {
	channel := u.upstreamChannels[completion.InvocationID]
	channel.Close()
	delete(u.upstreamChannels, completion.InvocationID)
}

