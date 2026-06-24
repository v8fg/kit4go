package log4go

import (
	"testing"
	"time"
)

var (
	logConfig = `{
  "level": "info",
  "full_path": true,
	"debug": true,
	
  "file_writer": {
    "level": "warn",
    "filename": "./test/log4go-test-%Y%M%D.log",
	"enable": true
  },

  "console_writer": {
    "level": "error",
    "enable": true,
    "color": true,
	"full_color": true
  },
	
  "kafka_writer": {
    "level": "ERROR",
    "enable": false,
    "buffer_size": 10,
    "debug": true,
	"msg": {
		"server_ip": "127.0.0.1"
	},
    "specify_version":true,
    "version":"0.10.0.1",
    "key": "kafka-test",
    "producer_topic": "log4go-kafka-test",
    "producer_return_successes": true,
    "producer_timeout": 1,
    "brokers": ["47.94.201.80:9092"]
  }
}
`
)

func TestConfig(t *testing.T) {
	if err := SetLog([]byte(logConfig)); err != nil {
		panic(err)
	}
	var name = "log4go config test"
	Debug("log4go by %s debug", name)
	Info("log4go by %s info", name)
	Notice("log4go by %s notice", name)
	Warn("log4go by %s warn", name)
	Error("log4go by %s error", name)
	Critical("log4go by %s critical", name)
	Alert("log4go by %s alert", name)
	Emergency("log4go by %s emergency", name)

	time.Sleep(1 * time.Second)
}
