package signalr

import (
	"fmt"
	"reflect"
	"sync"
	"time"
)

func newStreamClient(protocol hubProtocol, chanReceiveTimeout time.Duration, streamBufferCapacity uint) *streamClient {
	return &streamClient{
		mx:                   sync.Mutex{},
		upstreamChannels:     make(map[string]reflect.Value),
		runningStreams:       make(map[string]bool),
		chanReceiveTimeout:   chanReceiveTimeout,
		streamBufferCapacity: streamBufferCapacity,
		protocol:             protocol,
	}
}

type streamClient struct {
	mx                   sync.Mutex
	upstreamChannels     map[string]reflect.Value
	runningStreams       map[string]bool
	chanReceiveTimeout   time.Duration
	streamBufferCapacity uint
	protocol             hubProtocol
}

func (c *streamClient) buildChannelArgument(invocation invocationMessage, argType reflect.Type, chanCount int) (arg reflect.Value, canClientStreaming bool, err error) {
	c.mx.Lock()
	defer c.mx.Unlock()
	if argType.Kind() != reflect.Chan || argType.ChanDir() == reflect.SendDir {
		return reflect.Value{}, false, nil
	} else if len(invocation.StreamIds) > chanCount {
		// MakeChan does only accept bidirectional channels, and we need to Send to this channel anyway
		arg = reflect.MakeChan(reflect.ChanOf(reflect.BothDir, argType.Elem()), int(c.streamBufferCapacity))
		c.upstreamChannels[invocation.StreamIds[chanCount]] = arg
		return arg, true, nil
	} else {
		// To many channel parameters arguments this method. The client will not send streamItems for these
		return reflect.Value{}, true, fmt.Errorf("method %s has more chan parameters than the client will stream", invocation.Target)
	}
}

func (c *streamClient) newUpstreamChannel(invocationID string) <-chan interface{} {
	c.mx.Lock()
	defer c.mx.Unlock()
	upChan := make(chan interface{}, c.streamBufferCapacity)
	c.upstreamChannels[invocationID] = reflect.ValueOf(upChan)
	return upChan
}

func (c *streamClient) deleteUpstreamChannel(invocationID string) {
	c.mx.Lock()
	if upChan, ok := c.upstreamChannels[invocationID]; ok {
		upChan.Close()
		delete(c.upstreamChannels, invocationID)
	}
	c.mx.Unlock()
}

func (c *streamClient) receiveStreamItem(streamItem streamItemMessage) error {
	c.mx.Lock()
	defer c.mx.Unlock()
	if upChan, ok := c.upstreamChannels[streamItem.InvocationID]; ok {
		// Mark the stream as running to detect illegal completion with result on this id
		c.runningStreams[streamItem.InvocationID] = true
		chanVal := reflect.New(upChan.Type().Elem())
		err := c.protocol.UnmarshalArgument(streamItem.Item, chanVal.Interface())
		if err != nil {
			return err
		}
		return c.sendChanValSave(upChan, chanVal.Elem())
	}
	return fmt.Errorf(`unknown stream id "%v"`, streamItem.InvocationID)
}

func (c *streamClient) sendChanValSave(upChan reflect.Value, chanVal reflect.Value) error {
	done := make(chan error)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("%v", r)
			}
		}()
		upChan.Send(chanVal)
		done <- nil
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(c.chanReceiveTimeout):
		return &hubChanTimeoutError{fmt.Sprintf("timeout (%v) waiting for hub to receive client streamed value", c.chanReceiveTimeout)}
	}
}

type hubChanTimeoutError struct {
	msg string
}

func (h *hubChanTimeoutError) Error() string {
	return h.msg
}

func (c *streamClient) handlesInvocationID(invocationID string) bool {
	c.mx.Lock()
	defer c.mx.Unlock()
	_, ok := c.upstreamChannels[invocationID]
	return ok
}

func (c *streamClient) receiveCompletionItem(completion completionMessage, invokeClient *invokeClient) error {
	c.mx.Lock()
	channel, ok := c.upstreamChannels[completion.InvocationID]
	c.mx.Unlock()
	if ok {
		var err error
		if completion.Error != "" {
			// Push error to the error channel
			err = invokeClient.receiveCompletionItem(completion)
		} else {
			if completion.Result != nil {
				c.mx.Lock()
				running := c.runningStreams[completion.InvocationID]
				c.mx.Unlock()
				if running {
					err = fmt.Errorf("client side streaming: received completion with result %v", completion)
				} else {
					// handle result like a stream item
					err = c.receiveStreamItem(streamItemMessage{
						Type:         2,
						InvocationID: completion.InvocationID,
						Item:         completion.Result,
					})
				}
			}
		}
		channel.Close()
		// Close error channel
		invokeClient.deleteInvocation(completion.InvocationID)
		c.mx.Lock()
		delete(c.upstreamChannels, completion.InvocationID)
		delete(c.runningStreams, completion.InvocationID)
		c.mx.Unlock()
		return err
	}
	return fmt.Errorf("received completion with unknown id %v", completion.InvocationID)
}
