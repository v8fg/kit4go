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
