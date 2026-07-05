package file

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/mock"
)

// TestCreateIfNotExists_FileExistsNoop covers the early-return branch of
// CreateIfNotExists when the path already exists (the `if !exist` false path):
// it must not call MkdirAll/OpenFile and must return nil.
func TestCreateIfNotExists_FileExistsNoop(t *testing.T) {
	td := t.TempDir()
	existing := td // the temp dir already exists
	convey.Convey("CreateIfNotExists on existing path is a noop", t, func() {
		convey.So(CreateIfNotExists(existing, false), convey.ShouldBeNil)
		// And for the isFile=true branch on an existing path (still noop, returns nil).
		convey.So(CreateIfNotExists(existing, true), convey.ShouldBeNil)
	})
}

// TestMockFS_SingleReturnFuncBranches covers the second type-assertion branch in
// each mockery-generated MockFS method (the `ret.Get(0).(func(args...) T)`
// single-return-function path). These branches are unreachable through the typed
// EXPECT().RunAndReturn() builder (which always sets the combined (T, error)
// func form); we drive them by registering expectations directly on the embedded
// mock.Mock with a single-return func as the first return value.
func TestMockFS_SingleReturnFuncBranches(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "mock-single-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	var tmpInfo fs.FileInfo
	if tmpInfo, err = os.Stat(tmpFile.Name()); err != nil {
		t.Fatal(err)
	}

	convey.Convey("single-return func branches", t, func() {
		// Create: ret.Get(0).(func(string) *os.File)
		m1 := &MockFS{}
		m1.Mock.Test(t)
		m1.On("Create", "c").Return(func(string) *os.File { return tmpFile }, nil)
		f, err := m1.Create("c")
		convey.So(err, convey.ShouldBeNil)
		convey.So(f, convey.ShouldEqual, tmpFile)

		// Open: ret.Get(0).(func(string) *os.File)
		m2 := &MockFS{}
		m2.Mock.Test(t)
		m2.On("Open", "o").Return(func(string) *os.File { return tmpFile }, nil)
		of, err := m2.Open("o")
		convey.So(err, convey.ShouldBeNil)
		convey.So(of, convey.ShouldEqual, tmpFile)

		// OpenFile: ret.Get(0).(func(string, int, fs.FileMode) *os.File)
		m3 := &MockFS{}
		m3.Mock.Test(t)
		m3.On("OpenFile", "of", os.O_RDONLY, fs.FileMode(0o600)).
			Return(func(string, int, fs.FileMode) *os.File { return tmpFile }, nil)
		off, err := m3.OpenFile("of", os.O_RDONLY, 0o600)
		convey.So(err, convey.ShouldBeNil)
		convey.So(off, convey.ShouldEqual, tmpFile)

		// Stat: ret.Get(0).(func(string) fs.FileInfo)
		m4 := &MockFS{}
		m4.Mock.Test(t)
		m4.On("Stat", "s").Return(func(string) fs.FileInfo { return tmpInfo }, nil)
		st, err := m4.Stat("s")
		convey.So(err, convey.ShouldBeNil)
		convey.So(st, convey.ShouldEqual, tmpInfo)

		// Copy: ret.Get(0).(func(io.Writer, io.Reader) int64)
		m5 := &MockFS{}
		m5.Mock.Test(t)
		m5.On("Copy", tmpFile, tmpFile).Return(func(io.Writer, io.Reader) int64 { return 7 }, nil)
		n, err := m5.Copy(tmpFile, tmpFile)
		convey.So(err, convey.ShouldBeNil)
		convey.So(n, convey.ShouldEqual, int64(7))

		// MkdirAll: ret.Get(0).(func(string, fs.FileMode) error)
		m6 := &MockFS{}
		m6.Mock.Test(t)
		m6.On("MkdirAll", "m", fs.FileMode(0o755)).Return(func(string, fs.FileMode) error { return nil })
		convey.So(m6.MkdirAll("m", 0o755), convey.ShouldBeNil)
	})
}

// TestMockFS_NilReturnBranches covers the `if ret.Get(0) != nil` false branch in
// the *os.File-returning methods (Create/Open/OpenFile) by returning an explicit
// nil file with a non-nil error — so the nil-guard skips the type assertion.
func TestMockFS_NilReturnBranches(t *testing.T) {
	errSentinel := errors.New("nil-fs")

	convey.Convey("nil *os.File return branches", t, func() {
		m1 := &MockFS{}
		m1.Mock.Test(t)
		m1.On("Create", "c").Return((*os.File)(nil), errSentinel)
		f, err := m1.Create("c")
		convey.So(f, convey.ShouldBeNil)
		convey.So(err, convey.ShouldEqual, errSentinel)

		m2 := &MockFS{}
		m2.Mock.Test(t)
		m2.On("Open", "o").Return((*os.File)(nil), errSentinel)
		of, err := m2.Open("o")
		convey.So(of, convey.ShouldBeNil)
		convey.So(err, convey.ShouldEqual, errSentinel)

		m3 := &MockFS{}
		m3.Mock.Test(t)
		m3.On("OpenFile", "of", os.O_RDONLY, fs.FileMode(0o600)).Return((*os.File)(nil), errSentinel)
		off, err := m3.OpenFile("of", os.O_RDONLY, 0o600)
		convey.So(off, convey.ShouldBeNil)
		convey.So(err, convey.ShouldEqual, errSentinel)

		// Stat: nil FileInfo with error.
		m4 := &MockFS{}
		m4.Mock.Test(t)
		m4.On("Stat", "s").Return((fs.FileInfo)(nil), errSentinel)
		st, err := m4.Stat("s")
		convey.So(st, convey.ShouldBeNil)
		convey.So(err, convey.ShouldEqual, errSentinel)
	})
}

// TestMockFS_ErrorFuncReturn covers the second type-assertion branch for the
// error return slot of each (T, error) method: `ret.Get(1).(func(args...) error)`.
// We register a non-func first return (so the T func branches are skipped) plus a
// single-return error func as the second return. Also covers Copy, which the
// other mock tests leave at the `ret.Error(1)` branch only.
func TestMockFS_ErrorFuncReturn(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "mock-err-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	errSentinel := errors.New("errfunc")

	convey.Convey("error func return branches (ret.Get(1).(func...) error)", t, func() {
		// Create: single-return-error second slot.
		m := &MockFS{}
		m.Mock.Test(t)
		m.On("Create", "c").Return(tmpFile, func(string) error { return errSentinel })
		f, err := m.Create("c")
		convey.So(f, convey.ShouldEqual, tmpFile)
		convey.So(err, convey.ShouldEqual, errSentinel)

		// Open
		m2 := &MockFS{}
		m2.Mock.Test(t)
		m2.On("Open", "o").Return(tmpFile, func(string) error { return errSentinel })
		of, err := m2.Open("o")
		convey.So(of, convey.ShouldEqual, tmpFile)
		convey.So(err, convey.ShouldEqual, errSentinel)

		// OpenFile
		m3 := &MockFS{}
		m3.Mock.Test(t)
		m3.On("OpenFile", "of", os.O_RDONLY, fs.FileMode(0o600)).
			Return(tmpFile, func(string, int, fs.FileMode) error { return errSentinel })
		off, err := m3.OpenFile("of", os.O_RDONLY, 0o600)
		convey.So(off, convey.ShouldEqual, tmpFile)
		convey.So(err, convey.ShouldEqual, errSentinel)

		// Copy: int64 T + single-return error func.
		m4 := &MockFS{}
		m4.Mock.Test(t)
		m4.On("Copy", tmpFile, tmpFile).
			Return(int64(3), func(io.Writer, io.Reader) error { return errSentinel })
		n, err := m4.Copy(tmpFile, tmpFile)
		convey.So(n, convey.ShouldEqual, int64(3))
		convey.So(err, convey.ShouldEqual, errSentinel)
	})
}

// TestMockFS_ErrorFuncStat covers the second type-assertion branch for Stat's
// error return: ret.Get(1).(func(string) error).
func TestMockFS_ErrorFuncStat(t *testing.T) {
	tmpInfo, err := os.Stat(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	errSentinel := errors.New("stat-errfunc")
	m := &MockFS{}
	m.Mock.Test(t)
	m.On("Stat", "s").Return(tmpInfo, func(string) error { return errSentinel })
	st, err := m.Stat("s")
	convey.Convey("Stat error func return", t, func() {
		convey.So(st, convey.ShouldEqual, tmpInfo)
		convey.So(err, convey.ShouldEqual, errSentinel)
	})
}

// TestMockFS_ExpecterNotNil sanity-covers the EXPECT() accessor (cheap, ensures
// the expecter path runs at least once in this file too).
func TestMockFS_ExpecterNotNil(t *testing.T) {
	m := &MockFS{}
	e := m.EXPECT()
	if e == nil {
		t.Fatal("EXPECT() returned nil")
	}
}

// TestListFiles_WalkError covers the `return e` branch of ListFiles' walk
// callback when WalkDir itself reports an error (e.g. permission denied on the
// root). We point it at a non-existent path so the walk errors.
func TestListFiles_WalkError(t *testing.T) {
	_, err := ListFiles("/this/path/does/not/exist/anywhere/xyz", TypeFile)
	if err == nil {
		t.Fatal("expected a walk error for a non-existent root")
	}
}

// TestInfoStr_ModTimeCoversFile exercises InfoStr on a freshly-modified file so
// the ModTime field is populated (defensive — the existing TestInfoStr already
// covers this, but we keep an explicit assertion here for stability).
func TestInfoStr_ModTimeCoversFile(t *testing.T) {
	td := t.TempDir()
	p := td + "/modtimefile"
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := InfoStr(p)
	if s == "" {
		t.Fatal("InfoStr of a real file must be non-empty")
	}
}

// --- mock.Arguments reference (unused import guard) -------------------------
var _ = mock.Arguments{}
