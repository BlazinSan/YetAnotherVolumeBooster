//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

type appSettings struct {
	StartWithWindows bool
	CloseToTray      bool
	DarkMode         bool
}

const (
	hkeyCurrentUser   = 0x80000001
	keyQueryValue     = 0x0001
	keySetValue       = 0x0002
	regSZ             = 1
	errorFileNotFound = 2
)

var (
	advapi32             = syscall.NewLazyDLL("advapi32.dll")
	procRegCreateKeyExW  = advapi32.NewProc("RegCreateKeyExW")
	procRegOpenKeyExW    = advapi32.NewProc("RegOpenKeyExW")
	procRegSetValueExW   = advapi32.NewProc("RegSetValueExW")
	procRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	procRegDeleteValueW  = advapi32.NewProc("RegDeleteValueW")
	procRegCloseKey      = advapi32.NewProc("RegCloseKey")
)

func userSettingsDir() string {
	base := os.Getenv("APPDATA")
	if base == "" {
		base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
	}
	return filepath.Join(base, "YetAnotherVolumeBooster")
}

func settingsFile() string { return filepath.Join(userSettingsDir(), "settings.ini") }

func loadSettings() appSettings {
	result := appSettings{CloseToTray: true}
	data, err := os.ReadFile(settingsFile())
	if err != nil {
		return result
	}
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.EqualFold(strings.TrimSpace(parts[1]), "true") || strings.TrimSpace(parts[1]) == "1"
		switch key {
		case "start_with_windows":
			result.StartWithWindows = value
		case "close_to_tray":
			result.CloseToTray = value
		case "dark_mode":
			result.DarkMode = value
		}
	}
	return result
}

func saveSettings(value appSettings) error {
	content := fmt.Sprintf(
		"# YetAnotherVolumeBooster user settings\r\nstart_with_windows=%t\r\nclose_to_tray=%t\r\ndark_mode=%t\r\n",
		value.StartWithWindows,
		value.CloseToTray,
		value.DarkMode,
	)
	if err := os.MkdirAll(userSettingsDir(), 0755); err != nil {
		return err
	}
	tmp := settingsFile() + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return err
	}
	_ = os.Remove(settingsFile())
	return os.Rename(tmp, settingsFile())
}

func runRegistryPath() string {
	return `Software\Microsoft\Windows\CurrentVersion\Run`
}

func openRunKey(access uintptr, create bool) (syscall.Handle, error) {
	var key syscall.Handle
	if create {
		var disposition uint32
		result, _, _ := procRegCreateKeyExW.Call(
			hkeyCurrentUser,
			uintptr(unsafe.Pointer(utf16(runRegistryPath()))),
			0,
			0,
			0,
			access,
			0,
			uintptr(unsafe.Pointer(&key)),
			uintptr(unsafe.Pointer(&disposition)),
		)
		if result != 0 {
			return 0, syscall.Errno(result)
		}
		return key, nil
	}
	result, _, _ := procRegOpenKeyExW.Call(
		hkeyCurrentUser,
		uintptr(unsafe.Pointer(utf16(runRegistryPath()))),
		0,
		access,
		uintptr(unsafe.Pointer(&key)),
	)
	if result != 0 {
		return 0, syscall.Errno(result)
	}
	return key, nil
}

func startupEnabled() (bool, error) {
	key, err := openRunKey(keyQueryValue, false)
	if err != nil {
		if errno, ok := err.(syscall.Errno); ok && uintptr(errno) == errorFileNotFound {
			return false, nil
		}
		return false, err
	}
	defer procRegCloseKey.Call(uintptr(key))

	var valueType uint32
	var size uint32
	result, _, _ := procRegQueryValueExW.Call(
		uintptr(key),
		uintptr(unsafe.Pointer(utf16(appTitle))),
		0,
		uintptr(unsafe.Pointer(&valueType)),
		0,
		uintptr(unsafe.Pointer(&size)),
	)
	if result == errorFileNotFound {
		return false, nil
	}
	if result != 0 {
		return false, syscall.Errno(result)
	}
	return valueType == regSZ && size > 2, nil
}

func setStartupEnabled(enabled bool) error {
	key, err := openRunKey(keySetValue|keyQueryValue, true)
	if err != nil {
		return err
	}
	defer procRegCloseKey.Call(uintptr(key))

	if !enabled {
		result, _, _ := procRegDeleteValueW.Call(uintptr(key), uintptr(unsafe.Pointer(utf16(appTitle))))
		if result != 0 && result != errorFileNotFound {
			return syscall.Errno(result)
		}
		logEvent("startup registry entry removed")
		return nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	command := `"` + exePath + `" --startup`
	encoded := syscall.StringToUTF16(command)
	result, _, _ := procRegSetValueExW.Call(
		uintptr(key),
		uintptr(unsafe.Pointer(utf16(appTitle))),
		0,
		regSZ,
		uintptr(unsafe.Pointer(&encoded[0])),
		uintptr(len(encoded)*2),
	)
	if result != 0 {
		return syscall.Errno(result)
	}
	logEvent("startup registry entry written: %s", command)
	return nil
}
