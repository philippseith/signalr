package signalr

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
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
		b, err := ioutil.ReadFile("testLogConf.json")
		if err == nil {
			err = json.Unmarshal(b, &lConf)
			if err != nil {
				lConf = loggerConfig{Enabled: false, Debug: false}
			}
		}
		writer := ioutil.Discard
		if lConf.Enabled {
			writer = os.Stderr
		}
		tLog = log.NewLogfmtLogger(writer)
	}
	return tLog
}

type panicLogger struct {
}

func (p panicLogger) Log(...interface{}) error {
	panic("panic as expected")
}

func Test_PanicLogger(t *testing.T) {
	defer func() {
		if err := recover(); err != nil {
			t.Errorf("panic in logger: '%v'", err)
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	server, _ := NewServer(ctx, SimpleHubFactory(&simpleHub{}),
		Logger(panicLogger{}, true),
		ChanReceiveTimeout(200*time.Millisecond),
		StreamBufferCapacity(5))
	// Create both ends of the connection
	cliConn, srvConn := newClientServerConnections()
	// Start the server
	go func() { _ = server.Serve(srvConn) }()
	// Create the Client
	client, _ := NewClient(ctx, WithConnection(cliConn), Logger(panicLogger{}, true))
	// Start it
	client.Start()
	// Do something
	<-client.Send("InvokeMe")
	cancel()
}
