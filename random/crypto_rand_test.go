package random_test

import (
	"errors"
	"math"
	"math/big"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/mock"

	"github.com/v8fg/kit4go/random"
)

// withCryptoSource temporarily replaces the package-level DefaultCryptoSource
// for fn (defer-restored) and asserts the mock expectations were met. fn must
// run within a convey.Convey context.
func withCryptoSource(t *testing.T, mockSource *random.MockCryptoSource, fn func()) {
	t.Helper()
	orig := random.DefaultCryptoSource
	random.DefaultCryptoSource = mockSource
	defer func() { random.DefaultCryptoSource = orig }()
	fn()
	if !mockSource.Mock.AssertExpectations(t) {
		t.Fail()
	}
}

func TestCryptoInt(t *testing.T) {
	convey.Convey("TestCryptoInt", t, func() {
		// happy-path (real crypto/rand).
		ret, err := random.CryptoInt(2)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ret, convey.ShouldNotBeNil)
		convey.So(ret.Cmp(big.NewInt(0)), convey.ShouldBeGreaterThanOrEqualTo, 0)
		convey.So(ret.Cmp(big.NewInt(2)), convey.ShouldBeLessThan, 0)

		ret, err = random.CryptoInt(1 << 62)
		convey.So(err, convey.ShouldBeNil)
		convey.So(ret.Cmp(big.NewInt(0)), convey.ShouldBeGreaterThanOrEqualTo, 0)

		// error-path: underlying Int returns a value, then an error.
		convey.Convey("Mocked", func() {
			mockSource := new(random.MockCryptoSource)
			mockSource.EXPECT().Int(big.NewInt(2)).Return(big.NewInt(1), nil).Once()
			mockSource.EXPECT().Int(big.NewInt(math.MaxInt64)).Return(big.NewInt(math.MaxInt), nil).Once()
			mockSource.EXPECT().Int(big.NewInt(2)).Return(nil, errors.New("err")).Once()
			withCryptoSource(t, mockSource, func() {
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
		})
	})
}

func TestCryptoPrime(t *testing.T) {
	convey.Convey("TestCryptoPrime", t, func() {
		// happy-path (real crypto/rand): bits < 2 errors, bits >= 2 prime.
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

		// error-path via mock: Prime returns error / specific primes.
		convey.Convey("Mocked", func() {
			mockSource := new(random.MockCryptoSource)
			mockSource.EXPECT().Prime(1).Return(big.NewInt(1), errors.New("crypto/rand: prime size must be at least 2-bit")).Once()
			mockSource.EXPECT().Prime(2).Return(big.NewInt(3), nil).Once()
			mockSource.EXPECT().Prime(3).Return(big.NewInt(7), nil).Once()
			withCryptoSource(t, mockSource, func() {
				ret, err := random.CryptoPrime(1)
				convey.So(ret, convey.ShouldResemble, big.NewInt(1))
				convey.So(err, convey.ShouldBeError)

				ret, err = random.CryptoPrime(2)
				convey.So(ret, convey.ShouldResemble, big.NewInt(3))
				convey.So(err, convey.ShouldBeNil)

				ret, err = random.CryptoPrime(3)
				convey.So(ret, convey.ShouldResemble, big.NewInt(7))
				convey.So(err, convey.ShouldBeNil)
			})
		})
	})
}

func TestCryptoRead(t *testing.T) {
	convey.Convey("TestCryptoRead", t, func() {
		// happy-path (real crypto/rand).
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

		// error-path via mock: Read returns specific counts.
		convey.Convey("Mocked", func() {
			mockSource := new(random.MockCryptoSource)
			mockSource.EXPECT().Read(mock.Anything).Return(0, nil).Once()
			mockSource.EXPECT().Read(mock.Anything).Return(1, nil).Once()
			mockSource.EXPECT().Read(mock.Anything).Return(2, nil).Once()
			mockSource.EXPECT().Read(mock.Anything).Return(4, nil).Once()
			withCryptoSource(t, mockSource, func() {
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
		})
	})
}

func TestCryptoReadString(t *testing.T) {
	convey.Convey("TestCryptoReadString", t, func() {
		// happy-path (real crypto/rand).
		ret := random.CryptoReadString(nil)
		convey.So(ret, convey.ShouldEqual, "")

		ret = random.CryptoReadString([]byte{1})
		convey.So(ret, convey.ShouldNotEqual, "")

		ret = random.CryptoReadString([]byte{1, 2})
		convey.So(ret, convey.ShouldNotEqual, "")

		ret = random.CryptoReadString([]byte{1, 2, 3, 5})
		convey.So(ret, convey.ShouldNotEqual, "")

		// error-path via mock: Read returns short counts; CryptoReadString
		// still base64-encodes whatever bytes are in the buffer.
		convey.Convey("Mocked", func() {
			mockSource := new(random.MockCryptoSource)
			mockSource.EXPECT().Read(mock.Anything).Return(0, nil).Once()
			mockSource.EXPECT().Read(mock.Anything).Return(1, nil).Once()
			mockSource.EXPECT().Read(mock.Anything).Return(2, nil).Once()
			mockSource.EXPECT().Read(mock.Anything).Return(4, nil).Once()
			withCryptoSource(t, mockSource, func() {
				convey.So(random.CryptoReadString(nil), convey.ShouldEqual, "")
				convey.So(random.CryptoReadString([]byte{1}), convey.ShouldNotEqual, "")
				convey.So(random.CryptoReadString([]byte{1, 2}), convey.ShouldNotEqual, "")
				convey.So(random.CryptoReadString([]byte{1, 2, 3, 5}), convey.ShouldNotEqual, "")
			})
		})
	})
}
