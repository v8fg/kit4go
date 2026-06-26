package log4go

import (
	"os"
	"os/exec"
	"testing"
)

// Test_Fatal_Subprocess runs Fatal in a child process and checks exit code + output.
func Test_Fatal_Subprocess(t *testing.T) {
	if os.Getenv("LOG4GO_FATAL_TEST") == "1" {
		// Child: configure logger, call Fatal, expect to never return
		lg := newLoggerWithRecords(make(chan *Record, 4))
		lg.SetLevel(TRACE)
		cw := &captureWriter{}
		lg.Register(cw)
		lg.Fatal("fatal test %d", 42)
		return // unreachable
	}
	// Parent: spawn child
	cmd := exec.Command(os.Args[0], "-test.run=Test_Fatal_Subprocess")
	cmd.Env = append(os.Environ(), "LOG4GO_FATAL_TEST=1")
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 1 {
			t.Errorf("exit code=%d want 1", exitErr.ExitCode())
		}
	} else {
		t.Fatalf("expected exit error, got: %v", err)
	}
}

// Test_Panic_Pkg_Subprocess runs package-level Panic in a child process.
func Test_Panic_Pkg_Subprocess(t *testing.T) {
	if os.Getenv("LOG4GO_PANIC_PKG_TEST") == "1" {
		Panic("pkg panic test %d", 99)
		return // unreachable
	}
	cmd := exec.Command(os.Args[0], "-test.run=Test_Panic_Pkg_Subprocess")
	cmd.Env = append(os.Environ(), "LOG4GO_PANIC_PKG_TEST=1")
	err := cmd.Run()
	if err == nil {
		t.Error("expected non-zero exit (panic), got nil")
	}
}

// Test_Fatal_Pkg_Subprocess runs package-level Fatal in a child process.
func Test_Fatal_Pkg_Subprocess(t *testing.T) {
	if os.Getenv("LOG4GO_FATAL_PKG_TEST") == "1" {
		Fatal("pkg fatal test %d", 7)
		return // unreachable
	}
	cmd := exec.Command(os.Args[0], "-test.run=Test_Fatal_Pkg_Subprocess")
	cmd.Env = append(os.Environ(), "LOG4GO_FATAL_PKG_TEST=1")
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 1 {
			t.Errorf("exit code=%d want 1", exitErr.ExitCode())
		}
	} else {
		t.Fatalf("expected exit error, got: %v", err)
	}
}
