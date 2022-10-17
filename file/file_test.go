package file_test

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey"
	"github.com/smartystreets/goconvey/convey"

	"github.com/v8fg/kit4go/file"
)

func touch(t *testing.T, name string) {
	_, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
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

		convey.Convey("TestCopyFileStatError", func() {
			outputs := []gomonkey.OutputCell{
				{Values: gomonkey.Params{nil, errors.New("os.Stat error")}, Times: 1},
			}
			af := gomonkey.ApplyFuncSeq(os.Stat, outputs)
			defer af.Reset()

			convey.So(file.CopyFile(filepath.Join(td, "dir", "foo1"), filepath.Join(td, "dir_err", "foo2")), convey.ShouldBeError)
		})

		convey.Convey("TestCopyFileOpenError", func() {
			outputs := []gomonkey.OutputCell{
				{Values: gomonkey.Params{nil, errors.New("os.Open error")}, Times: 1},
			}
			af := gomonkey.ApplyFuncSeq(os.Open, outputs)
			defer af.Reset()

			convey.So(file.CopyFile(filepath.Join(td, "dir", "foo1"), filepath.Join(td, "dir_err", "foo2")), convey.ShouldBeError)
		})

		convey.Convey("TestCopyFileMkdirAllError", func() {
			outputs := []gomonkey.OutputCell{
				{Values: gomonkey.Params{errors.New("os.MkdirAll error")}, Times: 1},
			}
			af := gomonkey.ApplyFuncSeq(os.MkdirAll, outputs)
			defer af.Reset()

			convey.So(file.CopyFile(filepath.Join(td, "dir", "foo1"), filepath.Join(td, "dir_err", "foo2")), convey.ShouldBeError)
		})

		convey.Convey("TestCopyFileCreateError", func() {
			outputs := []gomonkey.OutputCell{
				{Values: gomonkey.Params{nil, errors.New("os.Create error")}, Times: 1},
			}
			af := gomonkey.ApplyFuncSeq(os.Create, outputs)
			defer af.Reset()

			convey.So(file.CopyFile(filepath.Join(td, "dir", "foo1"), filepath.Join(td, "dir_err", "foo2")), convey.ShouldBeError)
		})

		convey.Convey("TestCopyFileIoCopyError", func() {
			outputs := []gomonkey.OutputCell{
				{Values: gomonkey.Params{int64(0), errors.New("io.Copy error")}, Times: 1},
			}
			af := gomonkey.ApplyFuncSeq(io.Copy, outputs)
			defer af.Reset()

			convey.So(file.CopyFile(filepath.Join(td, "dir", "foo1"), filepath.Join(td, "dir_err", "foo2")), convey.ShouldBeError)
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

		convey.Convey("TestCopyDirCopyFileError", func() {
			outputs := []gomonkey.OutputCell{
				{Values: gomonkey.Params{errors.New("CopyFile error")}, Times: 1},
			}
			af := gomonkey.ApplyFuncSeq(file.CopyFile, outputs)
			defer af.Reset()

			convey.So(file.CopyDir(filepath.Join(td, "dir", "foo1"), filepath.Join(td, "dir_err", "foo2")), convey.ShouldBeError)
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
	convey.Convey("TestCreateIfNotExistsMkdirAllError", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{errors.New("os.MkdirAll error")}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(os.MkdirAll, outputs)
		defer af.Reset()

		convey.So(file.CreateIfNotExists(filepath.Join(td, "dir", "foo3"), true), convey.ShouldBeError)
	})

	convey.Convey("TestCreateIfNotExistsOpenFileError", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{nil, errors.New("os.OpenFile error")}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(os.OpenFile, outputs)
		defer af.Reset()

		convey.So(file.CreateIfNotExists(filepath.Join(td, "dir", "foo3"), true), convey.ShouldBeError)
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

	stat, err := os.Stat(filepath.Join(td, "dir", "foo1"))
	stat2, err2 := os.Stat(filepath.Join(td, "dir"))
	statStr := fileInfoStr(stat)
	statStr2 := fileInfoStr(stat2)

	convey.SetDefaultFailureMode(convey.FailureContinues)
	convey.Convey("TestInfoStr", t, func() {
		outputs := []gomonkey.OutputCell{
			{Values: gomonkey.Params{stat, err}, Times: 1},
			{Values: gomonkey.Params{stat2, err2}, Times: 1},
			{Values: gomonkey.Params{nil, errors.New("error")}, Times: 1},
		}
		af := gomonkey.ApplyFuncSeq(os.Stat, outputs)
		defer af.Reset()

		convey.So(file.InfoStr(filepath.Join(td, "dir", "foo1")), convey.ShouldEqual, statStr)
		convey.So(file.InfoStr(filepath.Join(td, "dir")), convey.ShouldEqual, statStr2)
		convey.So(file.InfoStr(filepath.Join(td, "dir")), convey.ShouldBeEmpty)
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
}
