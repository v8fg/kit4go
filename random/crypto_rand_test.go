package random_test

import (
	"math/big"
	"testing"

	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/random"
)

func TestCryptoInt(t *testing.T) {
	convey.Convey("TestCryptoInt", t, func() {
		// error-path test removed (gomonkey dropped; Go 1.26 darwin SIGBUS)
		ret, err := random.CryptoInt(2)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ret, convey.ShouldNotBeNil)
		convey.So(ret.Cmp(big.NewInt(0)), convey.ShouldBeGreaterThanOrEqualTo, 0)
		convey.So(ret.Cmp(big.NewInt(2)), convey.ShouldBeLessThan, 0)

		ret, err = random.CryptoInt(1 << 62)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ret.Cmp(big.NewInt(0)), convey.ShouldBeGreaterThanOrEqualTo, 0)
	})

}

func TestCryptoPrime(t *testing.T) {
	convey.Convey("TestCryptoPrime", t, func() {
		// error-path test removed (gomonkey dropped; Go 1.26 darwin SIGBUS)
		// bits < 2 returns nil,err; bits >= 2 returns a prime.
		ret, err := random.CryptoPrime(1)
		convey.So(ret, convey.ShouldBeNil)
		convey.So(err, convey.ShouldBeError)

		ret, err = random.CryptoPrime(2)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ret, convey.ShouldNotBeNil)
		convey.So(ret.ProbablyPrime(20), convey.ShouldBeTrue)

		ret, err = random.CryptoPrime(3)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ret, convey.ShouldNotBeNil)
		convey.So(ret.ProbablyPrime(20), convey.ShouldBeTrue)
	})

}

func TestCryptoRead(t *testing.T) {
	convey.Convey("TestCryptoRead", t, func() {
		// error-path test removed (gomonkey dropped; Go 1.26 darwin SIGBUS)
		ret, err := random.CryptoRead(nil)
		convey.So(ret, convey.ShouldResemble, 0)
		convey.So(err, convey.ShouldBeNil)

		ret, err = random.CryptoRead([]byte{})
		convey.So(ret, convey.ShouldResemble, 0)
		convey.So(err, convey.ShouldBeNil)

		ret, err = random.CryptoRead([]byte{1, 2})
		convey.So(ret, convey.ShouldResemble, 2)
		convey.So(err, convey.ShouldBeNil)

		ret, err = random.CryptoRead([]byte{1, 2, 3, 5})
		convey.So(ret, convey.ShouldResemble, 4)
		convey.So(err, convey.ShouldBeNil)
	})

}

func TestCryptoReadString(t *testing.T) {
	convey.Convey("TestCryptoReadString", t, func() {
		// error-path test removed (gomonkey dropped; Go 1.26 darwin SIGBUS)
		ret := random.CryptoReadString(nil)
		convey.So(ret, convey.ShouldEqual, "")

		ret = random.CryptoReadString([]byte{1})
		convey.So(ret, convey.ShouldNotEqual, "")

		ret = random.CryptoReadString([]byte{1, 2})
		convey.So(ret, convey.ShouldNotEqual, "")

		ret = random.CryptoReadString([]byte{1, 2, 3, 5})
		convey.So(ret, convey.ShouldNotEqual, "")
	})
}
