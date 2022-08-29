package signalr

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
)

type loggerConfig struct {
	Enabled bool
	Debug   bool
}

var lConf loggerConfig

var tLog StructuredLogger

func testLoggerOption() func(Party) error {
	testLogger()
	return Logger(tLog, lConf.Debug)
}

func testLogger() StructuredLogger {
	if tLog == nil {
		lConf = loggerConfig{Enabled: false, Debug: false}
		b, err := os.ReadFile("testLogConf.json")
		if err == nil {
			err = json.Unmarshal(b, &lConf)
			if err != nil {
				lConf = loggerConfig{Enabled: false, Debug: false}
			}
		}
		writer := io.Discard
		if lConf.Enabled {
			writer = os.Stderr
		}
		tLog = log.NewLogfmtLogger(writer)
	}
	return tLog
}

type panicLogger struct {
	log log.Logger
}

func (p *panicLogger) Log(keyVals ...interface{}) error {
	_ = p.log.Log(keyVals...)
	panic("panic as expected")
}

type testLogWriter struct {
	mx sync.Mutex
	p  []byte
	t  *testing.T
}

func (t *testLogWriter) Write(p []byte) (n int, err error) {
	t.mx.Lock()
	defer t.mx.Unlock()
	t.p = append(t.p, p...)
	if len(p) > 0 && p[len(p)-1] == 10 { // Will not work on Windows, but doesn't matter. This is only to check if the logger output still looks as expected
		t.t.Log(string(t.p))
		t.p = nil
	}
	return len(p), nil
}

func Test_PanicLogger(t *testing.T) {
	defer func() {
		if err := recover(); err != nil {
			t.Errorf("panic in logger: '%v'", err)
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	server, _ := NewServer(ctx, SimpleHubFactory(&simpleHub{}),
		Logger(&panicLogger{log: log.NewLogfmtLogger(&testLogWriter{t: t})}, true),
		ChanReceiveTimeout(200*time.Millisecond),
		StreamBufferCapacity(5))
	// Create both ends of the connection
	cliConn, srvConn := newClientServerConnections()
	// Start the server
	go func() { _ = server.Serve(srvConn) }()
	// Create the Client
	client, _ := NewClient(ctx, WithConnection(cliConn), Logger(&panicLogger{log: log.NewLogfmtLogger(&testLogWriter{t: t})}, true))
	// Start it
	client.Start()
	// Do something
	<-client.Send("InvokeMe")
	cancel()
}
