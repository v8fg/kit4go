package random

import (
	"math/big"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

// This file covers branches of the mockery-generated MockCryptoSource that the
// happy-path tests in crypto_mock_test.go do not reach:
//
//   - The func-cast branches where a return argument is supplied as a
//     single-return-value / single-return-error func (e.g.
//     `func(*big.Int) *big.Int`). The typed EXPECT().Return(...) builder
//     rejects funcs (its params are concrete types), so we drive the mock via
//     the underlying m.Mock.On(...).Return(fn, fn) path.
//   - The "no return value specified" panic path when a mock method is called
//     without any Return at all.
//
// These are white-box tests (package random) because they manipulate the mock
// internals directly.

func TestMockCryptoSource_IntFuncReturnBranches(t *testing.T) {
	convey.Convey("Int func-cast branches", t, func() {
		m := new(MockCryptoSource)

		// ret[0] = func(*big.Int) *big.Int  -> line 37 cast (r0 = rf(max))
		// ret[1] = func(*big.Int) error     -> line 45 cast (r1 = rf(max))
		// The combined-signature func(*big.Int)(*big.Int,error) cast at line 34
		// must NOT fire, so use two separate single-return funcs.
		m.Mock.On("Int", big.NewInt(1)).Return(
			func(max *big.Int) *big.Int { return new(big.Int).Add(max, big.NewInt(41)) },
			func(*big.Int) error { return nil },
		).Once()

		v, err := m.Int(big.NewInt(1))
		convey.So(err, convey.ShouldBeNil)
		convey.So(v.Int64(), convey.ShouldEqual, 42)
	})
}

func TestMockCryptoSource_PrimeFuncReturnBranches(t *testing.T) {
	convey.Convey("Prime func-cast branches", t, func() {
		m := new(MockCryptoSource)

		// ret[0] = func(int) *big.Int -> line 95 cast
		// ret[1] = func(int) error     -> line 103 cast
		m.Mock.On("Prime", 5).Return(
			func(bits int) *big.Int { return big.NewInt(int64(bits) + 1) },
			func(int) error { return nil },
		).Once()

		p, err := m.Prime(5)
		convey.So(err, convey.ShouldBeNil)
		convey.So(p.Int64(), convey.ShouldEqual, 6)
	})
}

func TestMockCryptoSource_ReadFuncReturnBranches(t *testing.T) {
	convey.Convey("Read func-cast branches", t, func() {
		m := new(MockCryptoSource)

		// ret[0] = func([]byte) int   -> line 153 cast
		// ret[1] = func([]byte) error -> line 159 cast
		m.Mock.On("Read", []byte{1, 2, 3}).Return(
			func(b []byte) int { return len(b) },
			func([]byte) error { return nil },
		).Once()

		n, err := m.Read([]byte{1, 2, 3})
		convey.So(err, convey.ShouldBeNil)
		convey.So(n, convey.ShouldEqual, 3)
	})
}

// TestMockCryptoSource_NoReturnPanic covers the
// `if len(ret) == 0 { panic("no return value specified for X") }` branches
// (lines 28, 86, 144).
//
// These branches only fire when a Call exists with a zero-argument Return
// (m.Mock.On(...).Return()). Merely calling the method with NO expectation set
// does NOT reach them: testify's Called panics first with "mock: I don't know
// what to return because the method call was unexpected". So the branches are
// reachable only via the degenerate empty-Return arrangement exercised here.
func TestMockCryptoSource_NoReturnPanic(t *testing.T) {
	convey.Convey("Int panics when Return() has no values", t, func() {
		m := new(MockCryptoSource)
		m.Mock.On("Int", big.NewInt(1)).Return().Once()
		convey.So(func() { _, _ = m.Int(big.NewInt(1)) }, convey.ShouldPanic)
	})
	convey.Convey("Prime panics when Return() has no values", t, func() {
		m := new(MockCryptoSource)
		m.Mock.On("Prime", 1).Return().Once()
		convey.So(func() { _, _ = m.Prime(1) }, convey.ShouldPanic)
	})
	convey.Convey("Read panics when Return() has no values", t, func() {
		m := new(MockCryptoSource)
		m.Mock.On("Read", []byte{0}).Return().Once()
		convey.So(func() { _, _ = m.Read([]byte{0}) }, convey.ShouldPanic)
	})
}
