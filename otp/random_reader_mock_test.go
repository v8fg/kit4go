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

	// Read: Return with a single-int typed func as the first return value.
	// Exercises the ret.Get(0).(func([]byte) int) branch of the generated
	// Read (combined int+error func check above it stays false, so this path
	// is taken and r0 is produced by calling rf). The typed builder
	// MockRandomReader_Read_Call.Return enforces (int, error), so we register
	// the func-typed return directly on the underlying mock.Mock.
	buf3 := make([]byte, 12)
	m.Mock.On("Read", buf3).Return(func(b []byte) int { return len(b) }, io.EOF)
	if n, err := m.Read(buf3); n != len(buf3) || err != io.EOF {
		t.Fatalf("int-func Read: n=%d err=%v", n, err)
	}

	// Read: Return with a typed error func as the second return value.
	// Exercises the ret.Get(1).(func([]byte) error) branch of the generated
	// Read (the int branch above it is false because the first return is a
	// plain int, so r1 is produced by calling the error func).
	buf4 := make([]byte, 16)
	m.Mock.On("Read", buf4).Return(len(buf4), func([]byte) error { return io.ErrUnexpectedEOF })
	if n, err := m.Read(buf4); n != len(buf4) || err != io.ErrUnexpectedEOF {
		t.Fatalf("err-func Read: n=%d err=%v", n, err)
	}
}

// Test_RandomReader_NewMockConstructor covers the NewMockRandomReader
// constructor path explicitly.
func Test_RandomReader_NewMockConstructor(t *testing.T) {
	if m := NewMockRandomReader(t); m == nil {
		t.Fatal("NewMockRandomReader nil")
	}
}

// Test_RandomReader_ReadNoReturn covers the generated Read's defensive panic
// branch: when an expectation is registered without a Return/RunAndReturn,
// ret has length 0 and Read panics with "no return value specified for Read".
func Test_RandomReader_ReadNoReturn(t *testing.T) {
	// Use a bare mock (no t-bound constructor) so AssertExpectations on Cleanup
	// does not race with the expected panic.
	m := new(MockRandomReader)
	m.EXPECT().Read(make([]byte, 2))

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected Read to panic when no return value was specified")
		}
		msg, ok := r.(string)
		if !ok || msg != "no return value specified for Read" {
			t.Fatalf("unexpected panic value: %#v", r)
		}
	}()
	buf := make([]byte, 2)
	_, _ = m.Read(buf)
}
