package json_test

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/json"
)

func TestValid(t *testing.T) {
	convey.Convey("TestValid", t, func() {
		t.Logf("pkg:%v", json.PKG)

		valid := json.Valid([]byte("xwi88"))
		convey.So(valid, convey.ShouldBeFalse)

		valid = json.Valid([]byte(`"xwi88"`))
		convey.So(valid, convey.ShouldBeTrue)

		valid = json.Valid([]byte(`[1, 2, 3]`))
		convey.So(valid, convey.ShouldBeTrue)

		valid = json.Valid([]byte(`""`))
		convey.So(valid, convey.ShouldBeTrue)
	})
}

// TestBackend verifies the active JSON backend is exposed for monitoring
// (returns the short name; the value depends on the build tag — "stdlib" by
// default, "go_json"/"jsoniter"/"sonic" under the matching tag).
func TestBackend(t *testing.T) {
	if b := json.Backend(); b == "" {
		t.Error("Backend() returned empty; expected a non-empty backend name")
	} else {
		t.Logf("active json backend: %s", b)
	}
}
