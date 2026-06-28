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
	appLogMu     sync.Mutex
	appLogFile   *os.File
	appLogPath   string
	appStartTime time.Time
)

func preferredLogDir() string {
	if base := os.Getenv("ProgramData"); base != "" {
		return filepath.Join(base, "YetAnotherVolumeBooster", "logs")
	}
	if base := os.Getenv("LOCALAPPDATA"); base != "" {
		return filepath.Join(base, "YetAnotherVolumeBooster", "logs")
	}
	return filepath.Join(os.TempDir(), "YetAnotherVolumeBooster", "logs")
}

func initLogging(fileName string) string {
	appStartTime = time.Now()
	candidates := []string{
		preferredLogDir(),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "YetAnotherVolumeBooster", "logs"),
		filepath.Join(os.TempDir(), "YetAnotherVolumeBooster", "logs"),
	}

	for _, dir := range candidates {
		if dir == "" || stringsEqualFoldClean(dir, `YetAnotherVolumeBooster\logs`) {
			continue
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			continue
		}
		path := filepath.Join(dir, fileName)
		rotateLog(path)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			continue
		}
		appLogFile = f
		appLogPath = path
		// Capture Go runtime fatal errors and unrecovered traces in the same file.
		os.Stderr = f
		procSetStdHandle.Call(uintptr(STD_ERROR_HANDLE), f.Fd())
		break
	}

	debug.SetPanicOnFault(true)
	logEvent("============================================================")
	logEvent("process start: version=%s go=%s os=%s arch=%s pid=%d", appVersion, runtime.Version(), runtime.GOOS, runtime.GOARCH, os.Getpid())
	if exe, err := os.Executable(); err == nil {
		logEvent("executable=%s", exe)
	} else {
		logEvent("os.Executable failed: %v", err)
	}
	logEvent("ProgramData=%q ProgramFiles=%q LOCALAPPDATA=%q", os.Getenv("ProgramData"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA"))
	return appLogPath
}

func stringsEqualFoldClean(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func rotateLog(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < 2*1024*1024 {
		return
	}
	old := path + ".1"
	_ = os.Remove(old)
	_ = os.Rename(path, old)
}

func logEvent(format string, args ...any) {
	appLogMu.Lock()
	defer appLogMu.Unlock()
	if appLogFile == nil {
		return
	}
	line := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(appLogFile, "%s %s\r\n", time.Now().Format("2006-01-02 15:04:05.000"), line)
	_ = appLogFile.Sync()
}

func startHeartbeat() {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			logEvent("heartbeat: uptime=%s", time.Since(appStartTime).Round(time.Second))
		}
	}()
}

func logRecoveredPanic(scope string, value any) {
	logEvent("PANIC in %s: %v\r\n%s", scope, value, debug.Stack())
}

func closeLogging() {
	logEvent("process exit")
	appLogMu.Lock()
	defer appLogMu.Unlock()
	if appLogFile != nil {
		_ = appLogFile.Close()
		appLogFile = nil
	}
}

func currentLogPath() string {
	return appLogPath
}

func currentLogDir() string {
	if appLogPath != "" {
		return filepath.Dir(appLogPath)
	}
	return preferredLogDir()
}
