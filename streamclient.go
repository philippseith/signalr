package signalr

import (
	"fmt"
	"reflect"
	"sync"
	"time"
)

func newStreamClient(chanReceiveTimeout time.Duration, streamBufferCapacity uint) *streamClient {
	return &streamClient{
		mx:                   sync.Mutex{},
		upstreamChannels:     make(map[string]reflect.Value),
		runningStreams:       make(map[string]bool),
		chanReceiveTimeout:   chanReceiveTimeout,
		streamBufferCapacity: streamBufferCapacity,
	}
}

type streamClient struct {
	mx                   sync.Mutex
	upstreamChannels     map[string]reflect.Value
	runningStreams       map[string]bool
	chanReceiveTimeout   time.Duration
	streamBufferCapacity uint
}

func (c *streamClient) buildChannelArgument(invocation invocationMessage, argType reflect.Type, chanCount int) (arg reflect.Value, canClientStreaming bool, err error) {
	c.mx.Lock()
	defer c.mx.Unlock()
	if argType.Kind() != reflect.Chan || argType.ChanDir() == reflect.SendDir {
		return reflect.Value{}, false, nil
	} else if len(invocation.StreamIds) > chanCount {
		// MakeChan does only accept bidirectional channels and we need to Send to this channel anyway
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
		// Mark stream as running to detect illegal completion with result on this id
		c.runningStreams[streamItem.InvocationID] = true
		// Hack(?) for missing channel type information when the Protocol decodes StreamItem.Item
		// Protocol specific, as only json has this inexact number type. Messagepack might cause different problems
		chanElm := reflect.Indirect(reflect.New(upChan.Type().Elem())).Interface()
		f, isFloat := streamItem.Item.(float64)
		if isFloat {
			// This type of solution is constrained to basic types, e.g. chan MyInt is not supported
			chanVal, err := convertNumberToChannelType(chanElm, f)
			if err != nil {
				return err
			}
			return c.sendChanValSave(upChan, chanVal)
		}
		// Are stream item and channel type both slices/arrays?
		switch reflect.TypeOf(streamItem.Item).Kind() {
		case reflect.Slice, reflect.Array:
			switch reflect.TypeOf(chanElm).Kind() {
			case reflect.Slice:
				if sis, ok := streamItem.Item.([]interface{}); ok {
					chanElmElmType := upChan.Type().Elem().Elem() // The type of the array elements in the channel
					chanElm = reflect.Indirect(reflect.New(chanElmElmType)).Interface()
					chanVals := make([]reflect.Value, len(sis))
					for i, si := range sis {
						if f, ok := si.(float64); ok {
							chanVal, err := convertNumberToChannelType(chanElm, f)
							if err != nil {
								return err
							}
							chanVals[i] = chanVal
						}
					}
					chanSlice := reflect.Indirect(reflect.New(reflect.SliceOf(chanElmElmType)))
					chanSlice = reflect.Append(chanSlice, chanVals...)
					return c.sendChanValSave(upChan, chanSlice)
				}
			default:
				return fmt.Errorf("stream item of kind %v paired with channel of type %v", reflect.TypeOf(streamItem.Item).Kind(), reflect.TypeOf(chanElm))
			}
		default:
			return c.sendChanValSave(upChan, reflect.ValueOf(streamItem.Item))
		}
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

func convertNumberToChannelType(chanElm interface{}, number float64) (chanVal reflect.Value, err error) {
	switch chanElm.(type) {
	case int:
		return reflect.ValueOf(int(number)), nil
	case int8:
		return reflect.ValueOf(int8(number)), nil
	case int16:
		return reflect.ValueOf(int16(number)), nil
	case int32:
		return reflect.ValueOf(int32(number)), nil
	case int64:
		return reflect.ValueOf(int64(number)), nil
	case uint:
		return reflect.ValueOf(uint(number)), nil
	case uint8:
		return reflect.ValueOf(uint8(number)), nil
	case uint16:
		return reflect.ValueOf(uint16(number)), nil
	case uint32:
		return reflect.ValueOf(uint32(number)), nil
	case uint64:
		return reflect.ValueOf(uint64(number)), nil
	case float32:
		return reflect.ValueOf(float32(number)), nil
	case float64:
		return reflect.ValueOf(number), nil
	case string:
		return reflect.ValueOf(fmt.Sprint(number)), nil
	default:
		return reflect.Value{}, fmt.Errorf("can not convert %v to %v", number, reflect.TypeOf(chanElm))
	}
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
