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
    "brokers": ["127.0.0.1:9092"]
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

// TestSetupLog_KafkaWriterEnable covers the KafkaWriter.Enable branches in
// SetupLog (level aggregation + NewKafkaWriter/registerOrFail). Brokers are
// intentionally unset so Start fails fast (sarama rejects a broker-less client
// synchronously); SetupLog surfaces that as an error instead of panicking. The
// default logger's writers are snapshotted/restored so the failed registration
// does not leak into later tests.
func TestSetupLog_KafkaWriterEnable(t *testing.T) {
	dl := defaultLogger()
	saved := dl.snapshotWriters()
	defer dl.writers.Store(saved)

	lc := LogConfig{
		Level: "info",
		KafkaWriter: KafkaWriterOptions{
			Enable:        true,
			Level:         "ERROR",
			ProducerTopic: "log4go-cov",
			BufferSize:    8,
			// Brokers intentionally unset: Start fails fast rather than dialing.
		},
	}
	if err := SetupLog(lc); err == nil {
		t.Fatal("expected SetupLog to fail when the enabled kafka writer cannot start")
	}
}
