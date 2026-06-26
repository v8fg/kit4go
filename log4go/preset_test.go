package log4go

import (
	"strings"
	"testing"
	"time"
)

// Test_Presets_Configuration verifies the preset loggers carry the documented
// level/format/caller settings.
func Test_Presets_Configuration(t *testing.T) {
	prod := NewProduction()
	defer prod.Close()
	if prod.Format() != FormatJSON {
		t.Errorf("production format=%v want JSON", prod.Format())
	}
	if int32(INFO) != prod.level.Load() {
		t.Errorf("production level=%d want INFO", prod.level.Load())
	}

	dev := NewDevelopment()
	defer dev.Close()
	if dev.Format() != FormatText {
		t.Errorf("development format=%v want Text", dev.Format())
	}
	if int32(DEBUG) != dev.level.Load() {
		t.Errorf("development level=%d want DEBUG", dev.level.Load())
	}
}

// Test_Preset_ProductionEmits verifies the production preset actually emits JSON
// to its console writer (no panic, record flows).
func Test_Preset_ProductionEmits(t *testing.T) {
	prod := NewProduction()
	defer prod.Close()
	cw := &captureWriter{}
	prod.Register(cw)
	prod.Info("prod line %d", 1)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
	}
	if cw.Len() == 0 {
		t.Fatal("no record emitted from production preset")
	}
	cw.mu.Lock()
	r := cw.records[0]
	cw.mu.Unlock()
	if !strings.Contains(string(r.formattedBytes), `"msg":"prod line 1"`) {
		t.Errorf("production JSON missing msg: %s", r.formattedBytes)
	}
}
