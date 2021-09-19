package signalr_test

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/go-kit/log"
	"github.com/philippseith/signalr"
)

type loggerConfig struct {
	Enabled bool
	Debug   bool
}

var lConf loggerConfig

var tLog signalr.StructuredLogger

func testLoggerOption() func(signalr.Party) error {
	testLogger()
	return signalr.Logger(tLog, lConf.Debug)
}

func testLogger() signalr.StructuredLogger {
	if tLog == nil {
		lConf = loggerConfig{Enabled: false, Debug: false}
		b, err := ioutil.ReadFile("../testLogConf.json")
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
