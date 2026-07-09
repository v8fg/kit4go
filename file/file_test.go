package file_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/file"
)

func touch(t *testing.T, name string) {
	_, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
}

// withFS temporarily replaces the package-level DefaultFS for the duration of
// fn, restoring the original on return. It also asserts the mock expectations
// were met. Using a real os-backed FS by default keeps the existing happy-path
// behavior; tests inject a mock only for error-path coverage.
//
// Assertions inside fn use convey.So, so fn must run within a convey.Convey
// context; withFS itself does not create one to keep it composable.
func withFS(t *testing.T, mockFS *file.MockFS, fn func()) {
	t.Helper()
	orig := file.DefaultFS
	file.DefaultFS = mockFS
	defer func() { file.DefaultFS = orig }()
	fn()
	if !mockFS.Mock.AssertExpectations(t) {
		t.Fail()
	}
}

// statInfoFor returns a real os.FileInfo (via a temp file / dir) so a mock can
// return a non-nil value without re-implementing fs.FileInfo. Used to drive
// CopyFile past the Stat gate and reach later error injection points.
func statInfoFor(t *testing.T, path string, isDir bool) fs.FileInfo {
	t.Helper()
	if isDir {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	} else {
		touch(t, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func TestIsDir(t *testing.T) {
	td := t.TempDir()
	if err := os.MkdirAll(filepath.Join(td, "dir"), 0755); err != nil {
		t.Fatal(err)
	}

	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestIsDir", t, func() {
		convey.So(file.IsDir(filepath.Join(td, "dir")), convey.ShouldBeTrue)
		convey.So(file.IsDir(filepath.Join(td, "dir2")), convey.ShouldBeFalse)
	})
}

func TestIsFile(t *testing.T) {
	td := t.TempDir()
	if err := os.MkdirAll(filepath.Join(td, "dir"), 0755); err != nil {
		t.Fatal(err)
	}
	touch(t, filepath.Join(td, "dir", "foo1"))

	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestIsFile", t, func() {
		convey.So(file.IsFile(filepath.Join(td, "dir", "foo1")), convey.ShouldBeTrue)
		convey.So(file.IsFile(filepath.Join(td, "dir")), convey.ShouldBeFalse)
	})
}

func TestIsExist(t *testing.T) {
	td := t.TempDir()
	if err := os.MkdirAll(filepath.Join(td, "dir"), 0755); err != nil {
		t.Fatal(err)
	}
	touch(t, filepath.Join(td, "dir", "foo1"))

	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestIsFile", t, func() {
		convey.So(file.IsExist(filepath.Join(td, "dir", "foo1")), convey.ShouldBeTrue)
		convey.So(file.IsExist(filepath.Join(td, "dir")), convey.ShouldBeTrue)
		convey.So(file.IsExist(filepath.Join(td, "dir2")), convey.ShouldBeFalse)
	})
}

func TestCopyFile(t *testing.T) {
	td := t.TempDir()
	if err := os.MkdirAll(filepath.Join(td, "dir"), 0755); err != nil {
		t.Fatal(err)
	}

	touch(t, filepath.Join(td, "dir", "foo1"))
	touch(t, filepath.Join(td, "dir", "foo2"))

	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestCopyFile", t, func() {
		convey.So(file.CopyFile(filepath.Join(td, "dir", "foo1"), filepath.Join(td, "dir", "foo2")), convey.ShouldBeNil)
		convey.So(file.IsFile(filepath.Join(td, "dir", "foo2")), convey.ShouldBeTrue)
		convey.So(file.CopyFile(filepath.Join(td, "dir", "foo1"), filepath.Join(td, "dir2", "foo1")), convey.ShouldBeNil)
		convey.So(file.IsFile(filepath.Join(td, "dir2", "foo1")), convey.ShouldBeTrue)
		convey.So(file.CopyFile(filepath.Join(td, "dir", "foo1"), filepath.Join(td, "foo3")), convey.ShouldBeNil)
		convey.So(file.IsFile(filepath.Join(td, "foo3")), convey.ShouldBeTrue)
		convey.So(file.CopyFile(filepath.Join(td, "dir", "foo2"), filepath.Join(td, "dir2/")), convey.ShouldBeNil)
		convey.So(file.IsFile(filepath.Join(td, "dir2", "foo2")), convey.ShouldBeTrue)
	})

	// Regression: CopyFile silently returned a nil error when the source was a
	// non-regular file (directory/device/socket) because the original condition
	// `err != nil || !srcInfo.Mode().IsRegular()` returned the (nil) err for the
	// non-regular branch. It must now surface an explicit error instead.
	convey.Convey("TestCopyFileNonRegularSrcReturnsError", t, func() {
		src := filepath.Join(td, "a_directory")
		convey.So(os.MkdirAll(src, 0755), convey.ShouldBeNil)
		err := file.CopyFile(src, filepath.Join(td, "dst_nonregular"))
		convey.So(err, convey.ShouldBeError)
		convey.So(err.Error(), convey.ShouldContainSubstring, "is not a regular file")
		convey.So(file.IsExist(filepath.Join(td, "dst_nonregular")), convey.ShouldBeFalse)
	})

	// error-path coverage (gomonkey replaced by mockery mock injection).
	convey.Convey("TestCopyFileStatError", t, func() {
		src := filepath.Join(td, "err_src_stat")
		mockFS := new(file.MockFS)
		mockFS.EXPECT().Stat(src).Return(nil, errors.New("os.Stat error")).Once()
		withFS(t, mockFS, func() {
			err := file.CopyFile(src, filepath.Join(td, "dst"))
			convey.So(err, convey.ShouldBeError)
			convey.So(err.Error(), convey.ShouldContainSubstring, "os.Stat error")
		})
	})

	convey.Convey("TestCopyFileOpenError", t, func() {
		src := filepath.Join(td, "err_open")
		srcInfo := statInfoFor(t, src, false)
		mockFS := new(file.MockFS)
		mockFS.EXPECT().Stat(src).Return(srcInfo, nil).Once()
		mockFS.EXPECT().Open(src).Return(nil, errors.New("os.Open error")).Once()
		withFS(t, mockFS, func() {
			err := file.CopyFile(src, filepath.Join(td, "dst"))
			convey.So(err, convey.ShouldBeError)
			convey.So(err.Error(), convey.ShouldContainSubstring, "os.Open error")
		})
	})

	convey.Convey("TestCopyFileMkdirAllError", t, func() {
		src := filepath.Join(td, "err_mkdir")
		srcInfo := statInfoFor(t, src, false)
		srcFh, err := os.Open(src)
		convey.So(err, convey.ShouldBeNil)
		defer srcFh.Close()
		dst := filepath.Join(td, "dst")
		mockFS := new(file.MockFS)
		// CopyFile calls Stat(src), Open(src), then Stat(dst) via IsDir(dst).
		mockFS.EXPECT().Stat(src).Return(srcInfo, nil).Once()
		mockFS.EXPECT().Open(src).Return(srcFh, nil).Once()
		mockFS.EXPECT().Stat(dst).Return(nil, os.ErrNotExist).Once()
		mockFS.EXPECT().MkdirAll(filepath.Dir(dst), os.FileMode(0750)).
			Return(errors.New("os.MkdirAll error")).Once()
		withFS(t, mockFS, func() {
			err := file.CopyFile(src, dst)
			convey.So(err, convey.ShouldBeError)
			convey.So(err.Error(), convey.ShouldContainSubstring, "os.MkdirAll error")
		})
	})

	convey.Convey("TestCopyFileCreateError", t, func() {
		src := filepath.Join(td, "err_create")
		srcInfo := statInfoFor(t, src, false)
		srcFh, err := os.Open(src)
		convey.So(err, convey.ShouldBeNil)
		defer srcFh.Close()
		dst := filepath.Join(td, "dst2")
		mockFS := new(file.MockFS)
		mockFS.EXPECT().Stat(src).Return(srcInfo, nil).Once()
		mockFS.EXPECT().Open(src).Return(srcFh, nil).Once()
		mockFS.EXPECT().Stat(dst).Return(nil, os.ErrNotExist).Once()
		mockFS.EXPECT().MkdirAll(filepath.Dir(dst), os.FileMode(0750)).Return(nil).Once()
		mockFS.EXPECT().Create(dst).Return(nil, errors.New("os.Create error")).Once()
		withFS(t, mockFS, func() {
			err := file.CopyFile(src, dst)
			convey.So(err, convey.ShouldBeError)
			convey.So(err.Error(), convey.ShouldContainSubstring, "os.Create error")
		})
	})

	convey.Convey("TestCopyFileIoCopyError", t, func() {
		src := filepath.Join(td, "err_copy")
		srcInfo := statInfoFor(t, src, false)
		srcFh, err := os.Open(src)
		convey.So(err, convey.ShouldBeNil)
		defer srcFh.Close()
		dstFh, err := os.Create(filepath.Join(td, "err_copy_dst"))
		convey.So(err, convey.ShouldBeNil)
		defer dstFh.Close()
		dst := filepath.Join(td, "err_copy_dst")
		mockFS := new(file.MockFS)
		mockFS.EXPECT().Stat(src).Return(srcInfo, nil).Once()
		mockFS.EXPECT().Open(src).Return(srcFh, nil).Once()
		mockFS.EXPECT().Stat(dst).Return(nil, os.ErrNotExist).Once()
		mockFS.EXPECT().MkdirAll(filepath.Dir(dst), os.FileMode(0750)).Return(nil).Once()
		mockFS.EXPECT().Create(dst).Return(dstFh, nil).Once()
		mockFS.EXPECT().Copy(dstFh, srcFh).Return(int64(0), errors.New("io.Copy error")).Once()
		withFS(t, mockFS, func() {
			err := file.CopyFile(src, dst)
			convey.So(err, convey.ShouldBeError)
			convey.So(err.Error(), convey.ShouldContainSubstring, "io.Copy error")
		})
	})
}

func TestCopyDir(t *testing.T) {
	td := t.TempDir()
	if err := os.MkdirAll(filepath.Join(td, "dir"), 0755); err != nil {
		t.Fatal(err)
	}

	touch(t, filepath.Join(td, "dir", "foo1"))
	touch(t, filepath.Join(td, "dir", "foo2"))

	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestCopyDir", t, func() {
		convey.So(file.CopyDir(filepath.Join(td, "dir"), filepath.Join(td, "dir2")), convey.ShouldBeNil)
		convey.So(file.IsDir(filepath.Join(td, "dir2")), convey.ShouldBeTrue)
		convey.So(file.IsFile(filepath.Join(td, "dir2", "foo1")), convey.ShouldBeTrue)
		convey.So(file.IsFile(filepath.Join(td, "dir2", "foo2")), convey.ShouldBeTrue)
		convey.So(file.IsFile(filepath.Join(td, "dir2", "foo3")), convey.ShouldBeFalse)
	})

	// regression: a non-existent src dir must surface ListFiles' error.
	// Previously CopyDir declared a fresh err scoped to an if-init block; when
	// ListFiles failed the block was skipped and the outer named-return err
	// (still nil) was returned, so CopyDir silently reported success.
	convey.Convey("TestCopyDir_NonexistentSrcReturnsError", t, func() {
		src := filepath.Join(td, "does_not_exist")
		dst := filepath.Join(td, "dir_dst_nonexistent")
		err := file.CopyDir(src, dst)
		convey.So(err, convey.ShouldBeError)
		convey.So(errors.Is(err, fs.ErrNotExist), convey.ShouldBeTrue)
		convey.So(file.IsExist(dst), convey.ShouldBeFalse)
	})

	// error-path: CopyFile (invoked per walked file) fails and propagates.
	// We walk a real source dir but inject a Copy failure via the mock FS so
	// the inner CopyFile returns an error, exercising CopyDir's error branch.
	convey.Convey("TestCopyDirCopyFileError", t, func() {
		src := filepath.Join(td, "dir")
		// gather a real source file so we can open it.
		srcFile := filepath.Join(src, "foo1")
		srcInfo, err := os.Stat(srcFile)
		convey.So(err, convey.ShouldBeNil)
		srcFh, err := os.Open(srcFile)
		convey.So(err, convey.ShouldBeNil)
		defer srcFh.Close()
		dst := filepath.Join(td, "dir_err")
		// destination path CopyFile computes for the first file.
		dstFile := filepath.Join(dst, strings.TrimPrefix(srcFile, src))
		mockFS := new(file.MockFS)
		// ListFiles uses filepath.WalkDir directly (not DefaultFS), so only
		// the CopyFile path goes through the mock. Stat(src), Open(src),
		// Stat(dstFile)=not-exist, MkdirAll ok, Create ok, Copy fails.
		mockFS.EXPECT().Stat(srcFile).Return(srcInfo, nil).Once()
		mockFS.EXPECT().Open(srcFile).Return(srcFh, nil).Once()
		mockFS.EXPECT().Stat(dstFile).Return(nil, os.ErrNotExist).Once()
		mockFS.EXPECT().MkdirAll(filepath.Dir(dstFile), os.FileMode(0750)).Return(nil).Once()
		// materialize the destination dir + file so the real os.Create below
		// (handed to the mock as a return value) succeeds.
		convey.So(os.MkdirAll(filepath.Dir(dstFile), 0755), convey.ShouldBeNil)
		dstFh, err := os.Create(dstFile)
		convey.So(err, convey.ShouldBeNil)
		defer dstFh.Close()
		mockFS.EXPECT().Create(dstFile).Return(dstFh, nil).Once()
		mockFS.EXPECT().Copy(dstFh, srcFh).Return(int64(0), errors.New("CopyFile error")).Once()
		withFS(t, mockFS, func() {
			err := file.CopyDir(src, dst)
			convey.So(err, convey.ShouldBeError)
			convey.So(err.Error(), convey.ShouldContainSubstring, "CopyFile error")
		})
	})
}

func TestCreateIfNotExists(t *testing.T) {
	td := t.TempDir()
	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestCreateIfNotExists", t, func() {
		convey.So(file.IsDir(filepath.Join(td, "dir")), convey.ShouldBeFalse)
		convey.So(file.IsFile(filepath.Join(td, "dir", "foo1")), convey.ShouldBeFalse)
		convey.So(file.CreateIfNotExists(filepath.Join(td, "dir"), false), convey.ShouldBeNil)
		convey.So(file.IsDir(filepath.Join(td, "dir")), convey.ShouldBeTrue)
		convey.So(file.IsFile(filepath.Join(td, "dir", "foo1")), convey.ShouldBeFalse)
		convey.So(file.CreateIfNotExists(filepath.Join(td, "dir", "foo1"), true), convey.ShouldBeNil)
		convey.So(file.IsDir(filepath.Join(td, "dir")), convey.ShouldBeTrue)
		convey.So(file.IsFile(filepath.Join(td, "dir", "foo1")), convey.ShouldBeTrue)
		convey.So(file.IsFile(filepath.Join(td, "dir", "foo2")), convey.ShouldBeFalse)
	})

	// error-path: MkdirAll failure when creating a directory.
	convey.Convey("TestCreateIfNotExistsMkdirAllError", t, func() {
		path := filepath.Join(td, "errdir", "sub")
		mockFS := new(file.MockFS)
		// IsExist -> Stat returns not-exist; then MkdirAll fails.
		mockFS.EXPECT().Stat(path).Return(nil, os.ErrNotExist).Once()
		mockFS.EXPECT().MkdirAll(path, os.FileMode(0755)).Return(errors.New("os.MkdirAll error")).Once()
		withFS(t, mockFS, func() {
			err := file.CreateIfNotExists(path, false)
			convey.So(err, convey.ShouldBeError)
			convey.So(err.Error(), convey.ShouldContainSubstring, "os.MkdirAll error")
		})
	})

	// error-path: OpenFile failure when creating a file.
	convey.Convey("TestCreateIfNotExistsOpenFileError", t, func() {
		path := filepath.Join(td, "errdir2", "file")
		mockFS := new(file.MockFS)
		mockFS.EXPECT().Stat(path).Return(nil, os.ErrNotExist).Once()
		mockFS.EXPECT().MkdirAll(filepath.Dir(path), os.FileMode(0755)).Return(nil).Once()
		mockFS.EXPECT().OpenFile(path, os.O_CREATE, os.FileMode(0755)).Return(nil, errors.New("os.OpenFile error")).Once()
		withFS(t, mockFS, func() {
			err := file.CreateIfNotExists(path, true)
			convey.So(err, convey.ShouldBeError)
			convey.So(err.Error(), convey.ShouldContainSubstring, "os.OpenFile error")
		})
	})
}

func fileInfoStr(s fs.FileInfo) string {
	type fileInfo struct {
		Name    string
		Size    int64
		Mode    uint32
		ModeStr string
		ModTime time.Time
		IsDir   bool
	}
	info := fileInfo{
		Name:    s.Name(),
		Size:    s.Size(),
		Mode:    uint32(s.Mode()),
		ModeStr: s.Mode().String(),
		ModTime: s.ModTime().UTC(),
		IsDir:   s.IsDir(),
	}
	bts, _ := json.Marshal(info)
	return string(bts)
}

func TestInfoStr(t *testing.T) {
	td := t.TempDir()
	if err := os.MkdirAll(filepath.Join(td, "dir"), 0755); err != nil {
		t.Fatal(err)
	}
	touch(t, filepath.Join(td, "dir", "foo1"))

	stat, _ := os.Stat(filepath.Join(td, "dir", "foo1"))
	stat2, _ := os.Stat(filepath.Join(td, "dir"))
	statStr := fileInfoStr(stat)
	statStr2 := fileInfoStr(stat2)

	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestInfoStr", t, func() {
		convey.So(file.InfoStr(filepath.Join(td, "dir", "foo1")), convey.ShouldEqual, statStr)
		convey.So(file.InfoStr(filepath.Join(td, "dir")), convey.ShouldEqual, statStr2)
		convey.So(file.InfoStr(filepath.Join(td, "not_exist")), convey.ShouldBeEmpty)
	})

	// error-path: Stat returns an error -> InfoStr returns "".
	convey.Convey("TestInfoStrStatError", t, func() {
		path := filepath.Join(td, "anything")
		mockFS := new(file.MockFS)
		mockFS.EXPECT().Stat(path).Return(nil, errors.New("stat error")).Once()
		withFS(t, mockFS, func() {
			convey.So(file.InfoStr(path), convey.ShouldBeEmpty)
		})
	})
}

func TestListFiles(t *testing.T) {
	td := t.TempDir()
	if err := os.MkdirAll(filepath.Join(td, "dir"), 0755); err != nil {
		t.Fatal(err)
	}
	touch(t, filepath.Join(td, "dir/foo1"))
	touch(t, filepath.Join(td, "dir/foo2"))

	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestListFiles", t, func() {
		files, _ := file.ListFiles(filepath.Join(td, "dir"), file.TypeFile)
		convey.So(files, convey.ShouldResemble, []string{filepath.Join(td, "dir/foo1"), filepath.Join(td, "dir/foo2")})

		files, _ = file.ListFiles(filepath.Join(td, "dir"), file.TypeDir)
		convey.So(files, convey.ShouldResemble, []string{filepath.Join(td, "dir")})

		files, _ = file.ListFiles(filepath.Join(td, "dir"), file.TypeAll)
		convey.So(files, convey.ShouldResemble, []string{filepath.Join(td, "dir"), filepath.Join(td, "dir/foo1"), filepath.Join(td, "dir/foo2")})

		files, _ = file.ListFiles(filepath.Join(td, "dir"), 0)
		convey.So(files, convey.ShouldResemble, []string{filepath.Join(td, "dir/foo1"), filepath.Join(td, "dir/foo2")})
	})

	// error-path: non-existent dir returns a walk error.
	convey.Convey("TestListFilesNotExist", t, func() {
		_, err := file.ListFiles(filepath.Join(td, "nope"), file.TypeFile)
		convey.So(err, convey.ShouldBeError)
	})
}
