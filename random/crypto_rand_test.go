package random_test

import (
	"crypto/rand"
	"errors"
	"io"
	"math"
	"math/big"
	"testing"

	"github.com/agiledragon/gomonkey"
	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/random"
)

func TestCryptoInt(t *testing.T) {
	convey.Convey("TestCryptoInt", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{big.NewInt(1), nil}, Times: 1},
			{Values: gomonkey.Params{big.NewInt(math.MaxInt), nil}, Times: 1},
			{Values: gomonkey.Params{nil, errors.New("err")}, Times: 1},
		}

		// rand.Int use crypto/rand
		af := gomonkey.ApplyFuncSeq(rand.Int, outputs)
		defer af.Reset()

		ret, err := random.CryptoInt(2)
		convey.So(ret, convey.ShouldResemble, big.NewInt(1))
		convey.So(err, convey.ShouldBeNil)

		ret, err = random.CryptoInt(math.MaxInt64)
		convey.So(ret, convey.ShouldResemble, big.NewInt(math.MaxInt))
		convey.So(err, convey.ShouldBeNil)

		ret, err = random.CryptoInt(2)
		convey.So(ret, convey.ShouldBeNil)
		convey.So(err, convey.ShouldBeError)
	})

}

func TestCryptoPrime(t *testing.T) {
	convey.Convey("TestCryptoPrime", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{big.NewInt(1), errors.New("crypto/rand: prime size must be at least 2-bit")}, Times: 1},
			{Values: gomonkey.Params{big.NewInt(3), nil}, Times: 1},
			{Values: gomonkey.Params{big.NewInt(7), nil}, Times: 1},
		}

		// rand.Int use crypto/rand
		af := gomonkey.ApplyFuncSeq(rand.Prime, outputs)
		defer af.Reset()

		var ret *big.Int
		var err error

		ret, err = random.CryptoPrime(1)
		convey.So(ret, convey.ShouldResemble, big.NewInt(1))
		convey.So(err, convey.ShouldBeError)

		ret, err = random.CryptoPrime(2)
		convey.So(ret, convey.ShouldResemble, big.NewInt(3))
		convey.So(err, convey.ShouldBeNil)

		ret, err = random.CryptoPrime(3)
		convey.So(ret, convey.ShouldResemble, big.NewInt(7))
		convey.So(err, convey.ShouldBeNil)
	})

}

func TestCryptoRead(t *testing.T) {
	convey.Convey("TestCryptoRead", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{0, nil}, Times: 1},
			{Values: gomonkey.Params{1, nil}, Times: 1},
			{Values: gomonkey.Params{2, nil}, Times: 1},
			{Values: gomonkey.Params{4, nil}, Times: 1},
		}

		// rand.Int use crypto/rand
		af := gomonkey.ApplyFuncSeq(rand.Read, outputs)
		defer af.Reset()

		ret, err := random.CryptoRead(nil)
		convey.So(ret, convey.ShouldResemble, 0)
		convey.So(err, convey.ShouldBeNil)

		ret, err = random.CryptoRead([]byte{})
		convey.So(ret, convey.ShouldResemble, 1)
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
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{0, nil}, Times: 1},
			{Values: gomonkey.Params{1, nil}, Times: 1},
			{Values: gomonkey.Params{2, nil}, Times: 1},
			{Values: gomonkey.Params{4, nil}, Times: 1},
		}

		// rand.Int use crypto/rand
		af := gomonkey.ApplyFuncSeq(io.Reader.Read, outputs)
		defer af.Reset()

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
