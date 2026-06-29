//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

var (
	setupLogMu   sync.Mutex
	setupLogFile *os.File
	setupLogPath string
)

func setupPreferredLogDir() string {
	if base := os.Getenv("ProgramData"); base != "" {
		return filepath.Join(base, "YetAnotherVolumeBooster", "logs")
	}
	if base := os.Getenv("LOCALAPPDATA"); base != "" {
		return filepath.Join(base, "YetAnotherVolumeBooster", "logs")
	}
	return filepath.Join(os.TempDir(), "YetAnotherVolumeBooster", "logs")
}

func initSetupLogging() string {
	candidates := []string{
		setupPreferredLogDir(),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "YetAnotherVolumeBooster", "logs"),
		filepath.Join(os.TempDir(), "YetAnotherVolumeBooster", "logs"),
	}
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			continue
		}
		path := filepath.Join(dir, "YetAnotherVolumeBoosterSetup.log")
		rotateSetupLog(path)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			continue
		}
		setupLogFile = f
		setupLogPath = path
		break
	}
	debug.SetPanicOnFault(true)
	setupLog("============================================================")
	setupLog("setup start: version=%s go=%s os=%s arch=%s pid=%d args=%q", appVersion, runtime.Version(), runtime.GOOS, runtime.GOARCH, os.Getpid(), os.Args)
	if exe, err := os.Executable(); err == nil {
		setupLog("executable=%s", exe)
	} else {
		setupLog("os.Executable failed: %v", err)
	}
	setupLog("ProgramData=%q ProgramFiles=%q LOCALAPPDATA=%q", os.Getenv("ProgramData"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA"))
	return setupLogPath
}

func rotateSetupLog(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < 4*1024*1024 {
		return
	}
	old := path + ".1"
	_ = os.Remove(old)
	_ = os.Rename(path, old)
}

func setupLog(format string, args ...any) {
	setupLogMu.Lock()
	defer setupLogMu.Unlock()
	if setupLogFile == nil {
		return
	}
	_, _ = fmt.Fprintf(setupLogFile, "%s %s\r\n", time.Now().Format("2006-01-02 15:04:05.000"), fmt.Sprintf(format, args...))
	_ = setupLogFile.Sync()
}

func setupLogPanic(scope string, value any) {
	setupLog("PANIC in %s: %v\r\n%s", scope, value, debug.Stack())
}

func closeSetupLogging() {
	setupLog("setup exit")
	setupLogMu.Lock()
	defer setupLogMu.Unlock()
	if setupLogFile != nil {
		_ = setupLogFile.Sync()
		_ = setupLogFile.Close()
		setupLogFile = nil
	}
}

func setupLogLocation() string {
	if setupLogPath != "" {
		return setupLogPath
	}
	return filepath.Join(setupPreferredLogDir(), "YetAnotherVolumeBoosterSetup.log")
}
