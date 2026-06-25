package otp

import (
	"io"
	"testing"
)

// Test_RandomReader_MockBuilders covers the mockery-generated MockRandomReader
// Run/RunAndReturn builder branches and the NewMockRandomReader constructor
// (previously 0%, which dragged the package coverage to 86.5%).
func Test_RandomReader_MockBuilders(t *testing.T) {
	m := NewMockRandomReader(t)

	// Read: RunAndReturn (success path)
	buf := make([]byte, 4)
	m.EXPECT().Read(buf).RunAndReturn(func([]byte) (int, error) { return 4, nil })
	if n, err := m.Read(buf); err != nil || n != 4 {
		t.Fatalf("RunAndReturn Read: n=%d err=%v", n, err)
	}

	// Read: Run + Return (error path) — distinct arg so this expectation matches.
	buf2 := make([]byte, 8)
	m.EXPECT().Read(buf2).Run(func([]byte) {}).Return(0, io.ErrUnexpectedEOF)
	if n, err := m.Read(buf2); err == nil || n != 0 {
		t.Fatalf("Run+Return Read: n=%d err=%v", n, err)
	}
}

// Test_RandomReader_NewMockConstructor covers the NewMockRandomReader
// constructor path explicitly.
func Test_RandomReader_NewMockConstructor(t *testing.T) {
	if m := NewMockRandomReader(t); m == nil {
		t.Fatal("NewMockRandomReader nil")
	}
}
