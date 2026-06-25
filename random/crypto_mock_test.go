package random

import (
	"io"
	"math/big"
	"testing"
)

// Test_CryptoSource_MockBuilders covers the mockery-generated MockCryptoSource
// Run/RunAndReturn builder branches and the NewMockCryptoSource constructor
// (previously 0%, which dragged the package coverage to 83.6%). Each method
// (Int, Prime, Read) is driven via both Run+Return and RunAndReturn.
func Test_CryptoSource_MockBuilders(t *testing.T) {
	m := NewMockCryptoSource(t)

	// Int: RunAndReturn
	m.EXPECT().Int(big.NewInt(10)).RunAndReturn(func(*big.Int) (*big.Int, error) {
		return big.NewInt(1), nil
	})
	if v, err := m.Int(big.NewInt(10)); err != nil || v.Int64() != 1 {
		t.Fatalf("Int RunAndReturn: v=%v err=%v", v, err)
	}

	// Int: Run + Return
	m.EXPECT().Int(big.NewInt(20)).Run(func(max *big.Int) {}).Return(big.NewInt(2), nil)
	if v, err := m.Int(big.NewInt(20)); err != nil || v.Int64() != 2 {
		t.Fatalf("Int Run+Return: v=%v err=%v", v, err)
	}

	// Prime: RunAndReturn
	m.EXPECT().Prime(8).RunAndReturn(func(int) (*big.Int, error) { return big.NewInt(7), nil })
	if p, err := m.Prime(8); err != nil || p.Int64() != 7 {
		t.Fatalf("Prime RunAndReturn: p=%v err=%v", p, err)
	}

	// Prime: Run + Return
	m.EXPECT().Prime(4).Run(func(bits int) {}).Return(big.NewInt(3), nil)
	if p, err := m.Prime(4); err != nil || p.Int64() != 3 {
		t.Fatalf("Prime Run+Return: p=%v err=%v", p, err)
	}

	// Read: RunAndReturn
	buf := make([]byte, 4)
	m.EXPECT().Read(buf).RunAndReturn(func([]byte) (int, error) { return 4, nil })
	if n, err := m.Read(buf); err != nil || n != 4 {
		t.Fatalf("Read RunAndReturn: n=%d err=%v", n, err)
	}

	// Read: Run + Return (error path) — use a distinct arg so this expectation
	// is matched rather than the RunAndReturn one above.
	buf2 := make([]byte, 8)
	m.EXPECT().Read(buf2).Run(func([]byte) {}).Return(2, io.ErrUnexpectedEOF)
	if n, err := m.Read(buf2); err == nil || n != 2 {
		t.Fatalf("Read Run+Return error: n=%d err=%v", n, err)
	}
}

// Test_CryptoSource_NewMockConstructor covers the NewMockCryptoSource
// constructor path explicitly.
func Test_CryptoSource_NewMockConstructor(t *testing.T) {
	if m := NewMockCryptoSource(t); m == nil {
		t.Fatal("NewMockCryptoSource nil")
	}
}
