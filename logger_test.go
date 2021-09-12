package signalr

import (
	"encoding/json"
	"github.com/go-kit/kit/log"
	"io/ioutil"
	"os"
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
