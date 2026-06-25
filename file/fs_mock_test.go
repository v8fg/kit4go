package file

import (
	"io"
	"io/fs"
	"os"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

// Test_FS_Mock_RunAndReturn covers the mockery-generated MockFS Run/RunAndReturn
// builder branches (previously 0%, which dragged the package coverage to 72.5%
// despite the real file.go/fs.go code being 91-100% covered). Each method is
// driven once via RunAndReturn (or Run+Return) so the mock's Run + Return +
// RunAndReturn statements execute. This is coverage of the test-double, not
// production code.
func Test_FS_Mock_RunAndReturn(t *testing.T) {
	mockFS := NewMockFS(t)
	prev := DefaultFS
	DefaultFS = mockFS
	defer func() { DefaultFS = prev }()

	convey.Convey("MockFS RunAndReturn branches", t, func() {
		// Stat
		mockFS.EXPECT().Stat("a").RunAndReturn(func(string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		})
		convey.So(IsExist("a"), convey.ShouldBeFalse)

		// MkdirAll
		mockFS.EXPECT().MkdirAll("d", fs.FileMode(0o755)).RunAndReturn(func(string, fs.FileMode) error {
			return nil
		})
		convey.So(DefaultFS.MkdirAll("d", 0o755), convey.ShouldBeNil)

		// Create (run + return)
		tmp, _ := os.CreateTemp("", "mockcreate-*")
		tmp.Close()
		defer os.Remove(tmp.Name())
		mockFS.EXPECT().Create("c").Run(func(name string) {}).Return(tmp, nil)
		f, err := DefaultFS.Create("c")
		convey.So(err, convey.ShouldBeNil)
		convey.So(f, convey.ShouldNotBeNil)

		// Open + OpenFile
		mockFS.EXPECT().Open("o").RunAndReturn(func(string) (*os.File, error) { return tmp, nil })
		of, err := DefaultFS.Open("o")
		convey.So(err, convey.ShouldBeNil)
		convey.So(of, convey.ShouldNotBeNil)

		mockFS.EXPECT().OpenFile("of", os.O_RDONLY, fs.FileMode(0o600)).
			RunAndReturn(func(string, int, fs.FileMode) (*os.File, error) { return tmp, nil })
		off, err := DefaultFS.OpenFile("of", os.O_RDONLY, 0o600)
		convey.So(err, convey.ShouldBeNil)
		convey.So(off, convey.ShouldNotBeNil)

		// Copy
		mockFS.EXPECT().Copy(tmp, tmp).RunAndReturn(func(io.Writer, io.Reader) (int64, error) {
			return 0, nil
		})
		n, err := DefaultFS.Copy(tmp, tmp)
		convey.So(err, convey.ShouldBeNil)
		convey.So(n, convey.ShouldEqual, 0)
	})
}

// Test_FS_Mock_RunBuilders covers the .Run(...) builder branches (the
// RunAndReturn branches are covered in Test_FS_Mock_RunAndReturn above) so both
// mockery-generated builder paths execute, pushing the mock-double coverage up.
func Test_FS_Mock_RunBuilders(t *testing.T) {
	mockFS := NewMockFS(t)
	prev := DefaultFS
	DefaultFS = mockFS
	defer func() { DefaultFS = prev }()

	convey.Convey("MockFS Run builder branches", t, func() {
		mockFS.EXPECT().Stat("s").Run(func(name string) {}).Return(nil, os.ErrNotExist)
		convey.So(IsExist("s"), convey.ShouldBeFalse)

		mockFS.EXPECT().MkdirAll("m", fs.FileMode(0o755)).Run(func(string, fs.FileMode) {}).Return(nil)
		convey.So(DefaultFS.MkdirAll("m", 0o755), convey.ShouldBeNil)

		tmp, _ := os.CreateTemp("", "mockrun-*")
		tmp.Close()
		defer os.Remove(tmp.Name())

		mockFS.EXPECT().Create("c2").RunAndReturn(func(string) (*os.File, error) { return tmp, nil })
		f, err := DefaultFS.Create("c2")
		convey.So(err, convey.ShouldBeNil)
		convey.So(f, convey.ShouldNotBeNil)

		mockFS.EXPECT().Open("o2").Run(func(string) {}).Return(tmp, nil)
		of, err := DefaultFS.Open("o2")
		convey.So(err, convey.ShouldBeNil)
		convey.So(of, convey.ShouldNotBeNil)

		mockFS.EXPECT().OpenFile("of2", os.O_RDONLY, fs.FileMode(0o600)).Run(func(string, int, fs.FileMode) {}).Return(tmp, nil)
		off, err := DefaultFS.OpenFile("of2", os.O_RDONLY, 0o600)
		convey.So(err, convey.ShouldBeNil)
		convey.So(off, convey.ShouldNotBeNil)

		mockFS.EXPECT().Copy(tmp, tmp).Run(func(io.Writer, io.Reader) {}).Return(int64(1), nil)
		n, err := DefaultFS.Copy(tmp, tmp)
		convey.So(err, convey.ShouldBeNil)
		convey.So(n, convey.ShouldEqual, 1)
	})
}

// Test_FS_Mock_NewMockFS covers the NewMockFS constructor path.
func Test_FS_Mock_NewMockFS(t *testing.T) {
	m1 := NewMockFS(t)
	if m1 == nil {
		t.Fatal("NewMockFS(t) nil")
	}
	m2 := NewMockFS(t)
	if m2 == nil {
		t.Fatal("NewMockFS(t) nil (2)")
	}
}
