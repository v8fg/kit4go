package file

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

// This white-box test file closes the last *reachable* uncovered gap reported
// by `go tool cover` (see file.go:103-105). No production code is modified.
//
// The remaining uncovered lines in this package are six `if len(ret) == 0`
// panic guards inside the mockery-generated methods of mock_FS.go. They are
// deliberately NOT covered because they are unreachable defensive dead code
// under testify v1.11.1 (the version in go.mod):
//
//	testify's Mock.Called either finds a matching expectation (returning a
//	non-empty Arguments) or, when no expectation matches, calls Mock.fail,
//	which invokes testing.T.FailNow() (a goroutine Goexit) when m.test != nil,
//	or panics itself when m.test == nil. In both no-match cases control never
//	returns to the mock method, so ret can never be empty at the guard.
//	See github.com/stretchr/testify@v1.11.1/mock/mock.go:354 (fail) and :527.
//
// Mockery emits the guard for forward-compatibility / older testify versions
// where the behaviour differed; exercising it would require patching testify
// internals, which is out of scope.

// TestCreateIfNotExists_MkdirAllParentError covers file.go:103-105: when
// isFile=true, IsExist reports the path as absent and MkdirAll on the parent
// directory fails — CreateIfNotExists must propagate that error before ever
// reaching OpenFile. The pre-existing OpenFile-error test mocks MkdirAll to
// *succeed*, so this failure path was never exercised.
func TestCreateIfNotExists_MkdirAllParentError(t *testing.T) {
	td := t.TempDir()
	target := filepath.Join(td, "errdir3", "subfile")

	convey.Convey("CreateIfNotExists MkdirAll(parent) error", t, func() {
		mockFS := new(MockFS)
		// IsExist -> Stat returns not-exist; then MkdirAll(parent) fails.
		mockFS.EXPECT().Stat(target).Return(nil, os.ErrNotExist).Once()
		mockFS.EXPECT().MkdirAll(filepath.Dir(target), os.FileMode(0755)).
			Return(errors.New("parent mkdir error")).Once()

		orig := DefaultFS
		DefaultFS = mockFS
		defer func() { DefaultFS = orig }()

		err := CreateIfNotExists(target, true)
		convey.So(err, convey.ShouldBeError)
		convey.So(err.Error(), convey.ShouldContainSubstring, "parent mkdir error")
		convey.So(mockFS.Mock.AssertExpectations(t), convey.ShouldBeTrue)
	})
}
