package random_test

import (
	"math"
	"testing"

	"github.com/agiledragon/gomonkey"
	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/random"
)

func TestExpFloat64(t *testing.T) {
	convey.Convey("TestExpFloat64", t, func() {
		convey.So(random.ExpFloat64(), convey.ShouldBeGreaterThanOrEqualTo, 0)
	})
}

func TestFloat32(t *testing.T) {
	convey.Convey("TestFloat32", t, func() {
		convey.So(random.Float32(), convey.ShouldBeBetweenOrEqual, 0, 1)
	})
}

func TestFloat32Between(t *testing.T) {
	convey.Convey("TestFloat32Between", t, func() {
		convey.So(random.Float32Between(-10, 15), convey.ShouldBeBetweenOrEqual, -10, 15)
	})
}

func TestFloat64(t *testing.T) {
	convey.Convey("TestFloat64", t, func() {
		convey.So(random.Float64(), convey.ShouldBeBetweenOrEqual, 0, 1)
	})
}

func TestFloat64Between(t *testing.T) {
	convey.Convey("TestFloat64Between", t, func() {
		convey.So(random.Float64Between(-10, 15), convey.ShouldBeBetweenOrEqual, -10, 15)
	})
}

func TestInt(t *testing.T) {
	convey.Convey("TestInt", t, func() {
		convey.So(random.Int(), convey.ShouldBeBetweenOrEqual, math.MinInt, math.MaxInt)
	})
}

func TestInt31(t *testing.T) {
	convey.Convey("TestInt31", t, func() {
		convey.So(random.Int31(), convey.ShouldBeBetweenOrEqual, math.MinInt32, math.MaxInt32)
	})
}

func TestInt31Between(t *testing.T) {
	convey.Convey("TestInt31Between", t, func() {
		convey.So(random.Int31Between(-10, 15), convey.ShouldBeBetweenOrEqual, -10, 15)
	})
}

func TestInt63(t *testing.T) {
	convey.Convey("TestInt63", t, func() {
		convey.So(random.Int63(), convey.ShouldBeBetweenOrEqual, math.MinInt64, math.MaxInt64)
	})
}

func TestInt63Between(t *testing.T) {
	convey.Convey("TestInt63Between", t, func() {
		convey.So(random.Int63Between(-10, 15), convey.ShouldBeBetweenOrEqual, -10, 15)
	})
}

func TestIntBetween(t *testing.T) {
	convey.Convey("TestIntBetween", t, func() {
		convey.So(random.IntBetween(-10, 15), convey.ShouldBeBetweenOrEqual, -10, 15)
	})
}

func TestNormFloat64(t *testing.T) {
	convey.Convey("TestNormFloat64", t, func() {
		convey.So(random.NormFloat64(), convey.ShouldBeBetweenOrEqual, -math.MaxFloat32, math.MaxFloat64)
	})
}

func TestPercent(t *testing.T) {
	convey.Convey("TestPercent", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{0.0}, Times: 1},
			{Values: gomonkey.Params{float64(100)}, Times: 1},
			{Values: gomonkey.Params{100.5}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(random.Float64Between, outputs)
		defer af.Reset()

		convey.So(random.Percent(), convey.ShouldEqual, 0)
		convey.So(random.Percent(), convey.ShouldEqual, 100)
		convey.So(random.Percent(), convey.ShouldEqual, 100)
	})
}

func TestPerm(t *testing.T) {
	convey.Convey("TestPerm", t, func() {
		convey.So(random.Perm(0), convey.ShouldBeEmpty)
		convey.So(random.Perm(1), convey.ShouldResemble, []int{0})
	})
}

func TestPermBetween(t *testing.T) {
	convey.Convey("TestPermBetween", t, func() {
		convey.So(random.PermBetween(0, 0), convey.ShouldBeEmpty)
		convey.So(random.PermBetween(0, 10), convey.ShouldHaveLength, 10)
		convey.So(random.PermBetween(-10, 15), convey.ShouldHaveLength, 25)
	})
}

func TestRead(t *testing.T) {
	convey.Convey("TestRead", t, func() {
		n, err := random.Read([]byte{})
		convey.So(n, convey.ShouldEqual, 0)
		convey.So(err, convey.ShouldBeNil)

		n, err = random.Read([]byte{1})
		convey.So(n, convey.ShouldEqual, 1)
		convey.So(err, convey.ShouldBeNil)
	})
}

func TestSeed(t *testing.T) {
	convey.Convey("TestSeed", t, func() {
		random.Seed(0)
	})
}

func TestSeedReset(t *testing.T) {
	convey.Convey("TestSeedReset", t, func() {
		random.SeedReset()
	})
}

func TestShuffle(t *testing.T) {
	convey.Convey("TestShuffle", t, func() {
		convey.So(func() { random.Shuffle(-1, nil) }, convey.ShouldPanic)

		chooseSet := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		random.Shuffle(5, func(i, j int) {
			chooseSet[i], chooseSet[j] = chooseSet[j], chooseSet[i]
		})
		convey.So(chooseSet, convey.ShouldHaveLength, 10)

	})
}

func TestStringByRead(t *testing.T) {
	convey.Convey("TestStringByRead", t, func() {
		convey.So(random.StringByRead([]byte{}), convey.ShouldBeEmpty)
	})
}

func TestUint32(t *testing.T) {
	convey.Convey("TestUint32", t, func() {
		convey.So(random.Uint32(), convey.ShouldBeLessThanOrEqualTo, math.MaxUint32)
	})
}

func TestUint64(t *testing.T) {
	convey.Convey("TestUint64", t, func() {
		convey.So(random.Uint64(), convey.ShouldBeGreaterThanOrEqualTo, 0)
	})
}
