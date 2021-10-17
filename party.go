package signalr

import (
	"context"
	"time"

	"github.com/go-kit/log"
)

// Party is the common base of Server and Client. The Party methods are only used internally,
// but the interface is public to allow using Options on Party as parameters for external functions
type Party interface {
	context() context.Context
	cancel()

	onConnected(hc hubConnection)
	onDisconnected(hc hubConnection)

	invocationTarget(hc hubConnection) interface{}

	timeout() time.Duration
	setTimeout(timeout time.Duration)

	setHandshakeTimeout(timeout time.Duration)

	keepAliveInterval() time.Duration
	setKeepAliveInterval(interval time.Duration)

	insecureSkipVerify() bool
	setInsecureSkipVerify(skip bool)

	originPatterns()   [] string
	setOriginPatterns(orgs []string)

	chanReceiveTimeout() time.Duration
	setChanReceiveTimeout(interval time.Duration)

	streamBufferCapacity() uint
	setStreamBufferCapacity(capacity uint)

	allowReconnect() bool

	enableDetailedErrors() bool
	setEnableDetailedErrors(enable bool)

	loggers() (info StructuredLogger, dbg StructuredLogger)
	setLoggers(info StructuredLogger, dbg StructuredLogger)

	prefixLoggers(connectionID string) (info StructuredLogger, dbg StructuredLogger)

	maximumReceiveMessageSize() uint
	setMaximumReceiveMessageSize(size uint)
}

func newPartyBase(parentContext context.Context, info log.Logger, dbg log.Logger) partyBase {
	ctx, cancelFunc := context.WithCancel(parentContext)
	return partyBase{
		ctx:                        ctx,
		cancelFunc:                 cancelFunc,
		_timeout:                   time.Second * 30,
		_handshakeTimeout:          time.Second * 15,
		_keepAliveInterval:         time.Second * 5,
		_chanReceiveTimeout:        time.Second * 5,
		_streamBufferCapacity:      10,
		_maximumReceiveMessageSize: 1 << 15, // 32KB
		_enableDetailedErrors:      false,
		_insecureSkipVerify:        false,
		_originPatterns:            nil,
		info:                       info,
		dbg:                        dbg,
	}
}

type partyBase struct {
	ctx                        context.Context
	cancelFunc                 context.CancelFunc
	_timeout                   time.Duration
	_handshakeTimeout          time.Duration
	_keepAliveInterval         time.Duration
	_chanReceiveTimeout        time.Duration
	_streamBufferCapacity      uint
	_maximumReceiveMessageSize uint
	_enableDetailedErrors      bool
	_insecureSkipVerify		   bool
	_originPatterns             []string
	info                       StructuredLogger
	dbg                        StructuredLogger
}

func (p *partyBase) context() context.Context {
	return p.ctx
}

func (p *partyBase) cancel() {
	p.cancelFunc()
}

func (p *partyBase) timeout() time.Duration {
	return p._timeout
}

func (p *partyBase) setTimeout(timeout time.Duration) {
	p._timeout = timeout
}

func (p *partyBase) HandshakeTimeout() time.Duration {
	return p._handshakeTimeout
}

func (p *partyBase) setHandshakeTimeout(timeout time.Duration) {
	p._handshakeTimeout = timeout
}

func (p *partyBase) keepAliveInterval() time.Duration {
	return p._keepAliveInterval
}

func (p *partyBase) setKeepAliveInterval(interval time.Duration) {
	p._keepAliveInterval = interval
}

func (p *partyBase) insecureSkipVerify() bool {
	return  p._insecureSkipVerify
}
func (p *partyBase) setInsecureSkipVerify(skip bool) {
	p._insecureSkipVerify = skip
}

func (p *partyBase) originPatterns() []string {
	return  p._originPatterns
}
func (p *partyBase) setOriginPatterns(origins  []string) {
	p._originPatterns = origins
}

func (p *partyBase) chanReceiveTimeout() time.Duration {
	return p._chanReceiveTimeout
}

func (p *partyBase) setChanReceiveTimeout(interval time.Duration) {
	p._chanReceiveTimeout = interval
}

func (p *partyBase) streamBufferCapacity() uint {
	return p._streamBufferCapacity
}

func (p *partyBase) setStreamBufferCapacity(capacity uint) {
	p._streamBufferCapacity = capacity
}

func (p *partyBase) maximumReceiveMessageSize() uint {
	return p._maximumReceiveMessageSize
}

func (p *partyBase) setMaximumReceiveMessageSize(size uint) {
	p._maximumReceiveMessageSize = size
}

func (p *partyBase) enableDetailedErrors() bool {
	return p._enableDetailedErrors
}

func (p *partyBase) setEnableDetailedErrors(enable bool) {
	p._enableDetailedErrors = enable
}

func (p *partyBase) setLoggers(info StructuredLogger, dbg StructuredLogger) {
	p.info = info
	p.dbg = dbg
}

func (p *partyBase) loggers() (info StructuredLogger, debug StructuredLogger) {
	return p.info, p.dbg
}
