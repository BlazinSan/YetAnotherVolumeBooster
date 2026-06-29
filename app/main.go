//go:build windows

package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	appTitle   = "YetAnotherVolumeBooster"
	appVersion = "1.7.0"
)

var (
	hwndMain syscall.Handle

	currentPct = 100
	displayPct = float64(100)
	targetPct  = 100

	settings appSettings

	statusText        = "Ready"
	currentStatusTone = toneReady

	lastMasterSync   time.Time
	lastMasterSyncOK bool
	startupLaunch    bool
	isDragging       bool
	isClosing        bool

	windowIcon syscall.Handle
)

func utf16(s string) *uint16 { return syscall.StringToUTF16Ptr(s) }

func hasArg(target string) bool {
	for _, arg := range os.Args[1:] {
		if strings.EqualFold(arg, target) {
			return true
		}
	}
	return false
}

func programDataDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "YetAnotherVolumeBooster")
}

func equalizerAPOConfigDir() string {
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	return filepath.Join(pf, "EqualizerAPO", "config")
}

func installDir() string {
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	return filepath.Join(pf, "YetAnotherVolumeBooster")
}

func gainFile() string {
	return filepath.Join(equalizerAPOConfigDir(), "YetAnotherVolumeBooster", "gain.txt")
}
func apoConfigFile() string { return filepath.Join(equalizerAPOConfigDir(), "config.txt") }
func iconFile() string      { return filepath.Join(installDir(), "YetAnotherVolumeBooster.ico") }
func setupPath() string     { return filepath.Join(installDir(), "YetAnotherVolumeBoosterSetup.exe") }

func deviceSelectorPath() string {
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	return filepath.Join(pf, "EqualizerAPO", "DeviceSelector.exe")
}

func percentToDB(percent int) float64 {
	if percent <= 100 {
		return 0
	}
	return 20 * math.Log10(float64(percent)/100.0)
}

func readPercent() int {
	data, err := os.ReadFile(gainFile())
	if err != nil {
		return 100
	}
	re := regexp.MustCompile(`(?i)Preamp:\s*([+-]?[0-9]+(?:\.[0-9]+)?)\s*dB`)
	match := re.FindStringSubmatch(string(data))
	if len(match) != 2 {
		return 100
	}
	db, err := strconv.ParseFloat(match[1], 64)
	if err != nil || db <= 0 {
		return 100
	}
	pct := int(math.Round(math.Pow(10, db/20) * 100))
	return clampPercent(pct)
}

func clampPercent(percent int) int {
	if percent < 100 {
		return 100
	}
	if percent > 500 {
		return 500
	}
	return percent
}

func atomicWrite(path string, data []byte) error {
	logEvent("atomic write begin: path=%s bytes=%d", path, len(data))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	r, _, callErr := procMoveFileExW.Call(
		uintptr(unsafe.Pointer(utf16(tmp))),
		uintptr(unsafe.Pointer(utf16(path))),
		moveFileReplaceExisting|moveFileWriteThrough,
	)
	if r == 0 {
		_ = os.Remove(tmp)
		return callErr
	}
	logEvent("atomic write complete: path=%s", path)
	return nil
}

func writeGain(percent int) error {
	percent = clampPercent(percent)
	db := percentToDB(percent)
	content := fmt.Sprintf(
		"# Managed by YetAnotherVolumeBooster %s\r\n# %d%% = %+.2f dB\r\nPreamp: %.2f dB\r\n",
		appVersion,
		percent,
		db,
		db,
	)
	logEvent("gain update: percent=%d db=%+.2f file=%s", percent, db, gainFile())
	return atomicWrite(gainFile(), []byte(content))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func integrationActiveFor(percent int) bool {
	if !fileExists(deviceSelectorPath()) {
		return false
	}
	data, err := os.ReadFile(apoConfigFile())
	if err != nil {
		return false
	}
	if !strings.Contains(strings.ToLower(string(data)), `include: YetAnotherVolumeBooster\gain.txt`) {
		return false
	}
	return percent <= 100 || fileExists(gainFile())
}

func setStatus(text string, tone statusKind) {
	statusText = text
	currentStatusTone = tone
	invalidateWindow()
}

func applyPercent(percent int, persist bool, animate bool) {
	percent = clampPercent(percent)
	previous := currentPct
	currentPct = percent
	targetPct = percent
	if !animate || isDragging {
		displayPct = float64(percent)
	}

	if persist {
		if err := writeGain(percent); err != nil {
			logEvent("gain update failed: percent=%d err=%v", percent, err)
			setStatus("Could not update the gain file", toneError)
			return
		}
	}

	if percent > 100 && (lastMasterSync.IsZero() || previous <= 100 || time.Since(lastMasterSync) >= 2*time.Second) {
		lastMasterSync = time.Now()
		if err := ensureWindowsMasterVolumeMax(); err != nil {
			lastMasterSyncOK = false
			logEvent("Windows master-volume synchronization failed: %v", err)
		} else {
			lastMasterSyncOK = true
		}
	}

	if integrationActiveFor(percent) {
		db := percentToDB(percent)
		if percent > 100 && lastMasterSyncOK {
			setStatus(fmt.Sprintf("Active · Windows 100%% · APO %+.2f dB", db), toneActive)
		} else if percent > 100 {
			setStatus("Active · set Windows volume to 100% for full boost", toneWarning)
		} else {
			setStatus("Active · live APO gain · shared-mode audio", toneReady)
		}
	} else {
		setStatus("Audio integration needs setup or repair", toneWarning)
	}
	updateTrayState()
}

func shellRun(path, params string, elevated bool) error {
	verb := "open"
	if elevated {
		verb = "runas"
	}
	logEvent("ShellExecute request: path=%q params=%q elevated=%t", path, params, elevated)
	r, _, callErr := procShellExecuteW.Call(
		uintptr(hwndMain),
		uintptr(unsafe.Pointer(utf16(verb))),
		uintptr(unsafe.Pointer(utf16(path))),
		uintptr(unsafe.Pointer(utf16(params))),
		0,
		swShow,
	)
	if r <= 32 {
		return fmt.Errorf("ShellExecute result %d: %w", r, callErr)
	}
	return nil
}

func openDeviceSetup() {
	if !fileExists(setupPath()) {
		showError("YetAnotherVolumeBoosterSetup.exe was not found. Re-run the latest setup file.")
		return
	}
	if err := shellRun(setupPath(), "--device-selector", true); err != nil {
		logEvent("device setup launch failed: %v", err)
		showError("Could not open device setup.")
		return
	}
	setStatus("Device setup opened · finish in the selector", toneReady)
}

func repairIntegration() {
	if !fileExists(setupPath()) {
		showError("YetAnotherVolumeBoosterSetup.exe was not found. Re-run the latest setup file.")
		return
	}
	if err := shellRun(setupPath(), "--repair --no-launch", true); err != nil {
		logEvent("repair launch failed: %v", err)
		showError("Could not start the repair process.")
		return
	}
	setStatus("Repair started · finish device setup", toneReady)
}

func openLogs() {
	if err := os.MkdirAll(currentLogDir(), 0755); err != nil {
		showError("Could not create the diagnostic log folder.")
		return
	}
	if err := shellRun("explorer.exe", `"`+currentLogDir()+`"`, false); err != nil {
		showError("Could not open the diagnostic log folder.")
	}
}

func showError(text string) {
	messageBox(text+"\n\nDiagnostic log:\n"+currentLogPath(), appTitle, mbOK|mbIconError)
}

func toggleStartup() {
	next := !settings.StartWithWindows
	if err := setStartupEnabled(next); err != nil {
		logEvent("startup toggle failed: enabled=%t err=%v", next, err)
		setStatus("Could not update startup settings", toneError)
		return
	}
	settings.StartWithWindows = next
	if err := saveSettings(settings); err != nil {
		logEvent("settings save failed after startup toggle: %v", err)
	}
	if next {
		setStatus("Starts with Windows · launches quietly in the tray", toneReady)
	} else {
		setStatus("Start with Windows disabled", toneReady)
	}
	updateTrayState()
}

func toggleCloseToTray() {
	settings.CloseToTray = !settings.CloseToTray
	if err := saveSettings(settings); err != nil {
		logEvent("settings save failed after close-to-tray toggle: %v", err)
		setStatus("Could not save the close-to-tray setting", toneError)
		return
	}
	if settings.CloseToTray {
		setStatus("Close button now minimizes to tray", toneReady)
	} else {
		setStatus("Close button now exits YetAnotherVolumeBooster", toneReady)
	}
	updateTrayState()
}

func resetBoostBeforeExit() {
	if currentPct <= 100 {
		return
	}
	logEvent("exit requested: resetting APO gain from %d%% to 100%%", currentPct)
	if err := writeGain(100); err != nil {
		logEvent("exit gain reset failed: %v", err)
		return
	}
	currentPct = 100
	targetPct = 100
	displayPct = 100
	logEvent("exit gain reset complete: APO preamp is now 0.00 dB")
}

func requestExit() {
	if isClosing {
		return
	}
	isClosing = true
	resetBoostBeforeExit()
	procDestroyWindow.Call(uintptr(hwndMain))
}

func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	initLogging("YetAnotherVolumeBooster.log")
	defer closeLogging()
	defer func() {
		if recovered := recover(); recovered != nil {
			logRecoveredPanic("main", recovered)
			messageBox("YetAnotherVolumeBooster crashed. Diagnostic details were written to:\n\n"+currentLogPath(), appTitle, mbOK|mbIconError)
		}
	}()

	startupLaunch = hasArg("--startup")
	if activateExistingInstance(!startupLaunch) {
		return
	}
	singleInstanceHandle, alreadyRunning, err := acquireSingleInstance()
	if err != nil {
		logEvent("single-instance lock unavailable: %v", err)
	} else if alreadyRunning {
		logEvent("existing instance detected")
		waitForExistingInstance(!startupLaunch)
		releaseSingleInstance(singleInstanceHandle)
		return
	} else {
		defer releaseSingleInstance(singleInstanceHandle)
	}

	settings = loadSettings()
	if registryState, err := startupEnabled(); err == nil {
		settings.StartWithWindows = registryState
	}

	if err := initCoreAudio(); err != nil {
		logEvent("Core Audio initialization failed: %v", err)
	}
	defer closeCoreAudio()

	currentPct = readPercent()
	targetPct = currentPct
	displayPct = float64(currentPct)
	logEvent("startup: version=%s percent=%d startupLaunch=%t settings=%+v", appVersion, currentPct, startupLaunch, settings)

	if err := runWindow(); err != nil {
		logEvent("window initialization failed: %v", err)
		messageBox("YetAnotherVolumeBooster could not start.\n\n"+err.Error()+"\n\nDiagnostic log:\n"+currentLogPath(), appTitle, mbOK|mbIconError)
	}
}
