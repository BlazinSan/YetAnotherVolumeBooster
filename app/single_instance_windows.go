//go:build windows

package main

import (
	"syscall"
	"time"
	"unsafe"
)

const (
	errorAlreadyExists = 183

	singleInstanceMutexName = `Local\YetAnotherVolumeBooster.SingleInstance`
	appWindowClassName      = "YetAnotherVolumeBoosterMainWindowV5"
)

var (
	procCreateMutexW     = kernel32.NewProc("CreateMutexW")
	procGetLastError     = kernel32.NewProc("GetLastError")
	procCloseHandle      = kernel32.NewProc("CloseHandle")
	procFindWindowW      = user32.NewProc("FindWindowW")
	procBringWindowToTop = user32.NewProc("BringWindowToTop")
)

func acquireSingleInstance() (syscall.Handle, bool, error) {
	h, _, callErr := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(utf16(singleInstanceMutexName))))
	lastErr, _, _ := procGetLastError.Call()
	if h == 0 {
		return 0, false, callErr
	}
	return syscall.Handle(h), lastErr == errorAlreadyExists, nil
}

func releaseSingleInstance(handle syscall.Handle) {
	if handle != 0 {
		procCloseHandle.Call(uintptr(handle))
	}
}

func findExistingMainWindow() syscall.Handle {
	for _, className := range []string{appWindowClassName} {
		hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(utf16(className))), 0)
		if hwnd != 0 {
			return syscall.Handle(hwnd)
		}
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(utf16(appTitle))))
	return syscall.Handle(hwnd)
}

func activateWindow(hwnd syscall.Handle, show bool) {
	if show {
		procShowWindow.Call(uintptr(hwnd), swRestore)
		procBringWindowToTop.Call(uintptr(hwnd))
		procSetForegroundWindow.Call(uintptr(hwnd))
	}
}

func activateExistingInstance(show bool) bool {
	hwnd := findExistingMainWindow()
	if hwnd == 0 {
		return false
	}
	activateWindow(hwnd, show)
	return true
}

func waitForExistingInstance(show bool) bool {
	for attempt := 0; attempt < 20; attempt++ {
		if activateExistingInstance(show) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}
