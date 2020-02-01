package signalr

import "time"

type party interface {
	onConnected(hc hubConnection)
	onDisconnected(hc hubConnection)
	getInvocationTarget(hc hubConnection) interface{}
	setTimeout(timeout time.Duration)
	setHandshakeTimeout(timeout time.Duration)
	setKeepAliveInterval(interval time.Duration)
	setChanReceiveTimeout(interval time.Duration)
	setStreamBufferCapacity(capacity uint)
	allowReconnect() bool
	setEnableDetailedErrors(enable bool)
	setLoggers(info StructuredLogger, dbg StructuredLogger)
	loggers() (info StructuredLogger, dbg StructuredLogger)
	prefixLoggers() (info StructuredLogger, dbg StructuredLogger)
}

type partyBase struct {
	timeout              time.Duration
	handshakeTimeout     time.Duration
	keepAliveInterval    time.Duration
	chanReceiveTimeout   time.Duration
	streamBufferCapacity uint
	enableDetailedErrors bool
	info                 StructuredLogger
	dbg                  StructuredLogger
}

func (p *partyBase) setTimeout(timeout time.Duration) {
	p.timeout = timeout
}

func (p *partyBase) setHandshakeTimeout(timeout time.Duration) {
	p.handshakeTimeout = timeout
}

func (p *partyBase) setKeepAliveInterval(interval time.Duration) {
	p.keepAliveInterval = interval
}

func (p *partyBase) setChanReceiveTimeout(interval time.Duration) {
	p.chanReceiveTimeout = interval
}

func (p *partyBase) setStreamBufferCapacity(capacity uint) {
	p.streamBufferCapacity = capacity
}

func (p *partyBase) setEnableDetailedErrors(enable bool) {
	p.enableDetailedErrors = enable
}

func (p *partyBase) setLoggers(info StructuredLogger, dbg StructuredLogger) {
	p.info = info
	p.dbg = dbg
}

func (p *partyBase) loggers() (info StructuredLogger, debug StructuredLogger) {
	return p.info, p.dbg
}
