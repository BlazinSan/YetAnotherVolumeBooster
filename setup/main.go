//go:build windows

package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	appName    = "YetAnotherVolumeBooster
"
	appVersion = "1.7.0"

	expectedAPOHash = "7403be7427bbe1936a40dded082829b6e217fc4f5990fee5cba501f0ae055afa"

	MB_OK              = 0x00000000
	MB_YESNO           = 0x00000004
	MB_ICONERROR       = 0x00000010
	MB_ICONQUESTION    = 0x00000020
	MB_ICONINFORMATION = 0x00000040
	MB_TOPMOST         = 0x00040000
	IDYES              = 6
	SW_SHOWNORMAL      = 1
)

var (
	//go:embed payload/YetAnotherVolumeBooster.exe
	appPayload []byte

	//go:embed payload/YetAnotherVolumeBooster.ico
	iconPayload []byte

	// The public build contains a tiny placeholder. Replacing it with the official
	// installer before building produces a fully offline single-file setup.
	//go:embed payload/EqualizerAPO-x64-1.4.2.exe
	apoPayload []byte

	//go:embed payload/GPL-2.0.txt
	gplPayload []byte

	user32  = syscall.NewLazyDLL("user32.dll")
	shell32 = syscall.NewLazyDLL("shell32.dll")

	procMessageBoxW  = user32.NewProc("MessageBoxW")
	procShellExecute = shell32.NewProc("ShellExecuteW")
	procIsUserAdmin  = shell32.NewProc("IsUserAnAdmin")
)

func utf16(s string) *uint16 { return syscall.StringToUTF16Ptr(s) }

func messageBox(text, title string, flags uintptr) int {
	r, _, _ := procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(utf16(text))),
		uintptr(unsafe.Pointer(utf16(title))),
		flags|MB_TOPMOST,
	)
	return int(r)
}

func isAdmin() bool {
	r, _, _ := procIsUserAdmin.Call()
	return r != 0
}

func quoteArg(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t\"") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func relaunchElevated() error {
	setupLog("requesting elevation")
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"--elevated"}
	for _, arg := range os.Args[1:] {
		if arg != "--elevated" {
			args = append(args, arg)
		}
	}
	params := make([]string, 0, len(args))
	for _, arg := range args {
		params = append(params, quoteArg(arg))
	}
	r, _, callErr := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(utf16("runas"))),
		uintptr(unsafe.Pointer(utf16(exePath))),
		uintptr(unsafe.Pointer(utf16(strings.Join(params, " ")))),
		0,
		SW_SHOWNORMAL,
	)
	if r <= 32 {
		setupLog("elevation failed: result=%d err=%v", r, callErr)
		return callErr
	}
	setupLog("elevated process launched: result=%d", r)
	return nil
}

func hasArg(target string) bool {
	for _, arg := range os.Args[1:] {
		if strings.EqualFold(arg, target) {
			return true
		}
	}
	return false
}

func programFiles() string {
	if v := os.Getenv("ProgramFiles"); v != "" {
		return v
	}
	return `C:\Program Files`
}

func programData() string {
	if v := os.Getenv("ProgramData"); v != "" {
		return v
	}
	return `C:\ProgramData`
}

func installDir() string       { return filepath.Join(programFiles(), appName) }
func appPath() string          { return filepath.Join(installDir(), "YetAnotherVolumeBooster.exe") }
func setupPath() string        { return filepath.Join(installDir(), "VolumeBoostSetup.exe") }
func iconPath() string         { return filepath.Join(installDir(), "YetAnotherVolumeBooster.ico") }
func dataDir() string          { return filepath.Join(programData(), appName) }
func legacyGainPath() string   { return filepath.Join(dataDir(), "volume-boost.txt") }
func markerPath() string       { return filepath.Join(dataDir(), "apo-installed-by-YetAnotherVolumeBooster") }
func apoDir() string           { return filepath.Join(programFiles(), "EqualizerAPO") }
func apoConfigDir() string     { return filepath.Join(apoDir(), "config") }
func managedConfigDir() string { return filepath.Join(apoConfigDir(), "YetAnotherVolumeBooster") }
func gainPath() string         { return filepath.Join(managedConfigDir(), "gain.txt") }
func apoConfig() string        { return filepath.Join(apoConfigDir(), "config.txt") }
func deviceSelector() string   { return filepath.Join(apoDir(), "DeviceSelector.exe") }

func selectorCandidates() []string {
	return []string{
		filepath.Join(apoDir(), "DeviceSelector.exe"),
		filepath.Join(apoDir(), "Configurator.exe"),
	}
}

func selectorPath() string {
	for _, candidate := range selectorCandidates() {
		if fileExists(candidate) {
			return candidate
		}
	}
	return deviceSelector()
}

func findQtPlatformPlugin() (string, error) {
	var matches []string
	err := filepath.Walk(apoDir(), func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			setupLog("Qt plugin scan skipped path=%s err=%v", path, walkErr)
			return nil
		}
		if info != nil && !info.IsDir() && strings.EqualFold(info.Name(), "qwindows.dll") {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("qwindows.dll was not found under %s", apoDir())
	}
	// Prefer the shallowest deployment path, then a stable lexical order.
	sort.Slice(matches, func(i, j int) bool {
		depthI := strings.Count(filepath.Clean(matches[i]), string(os.PathSeparator))
		depthJ := strings.Count(filepath.Clean(matches[j]), string(os.PathSeparator))
		if depthI != depthJ {
			return depthI < depthJ
		}
		return strings.ToLower(matches[i]) < strings.ToLower(matches[j])
	})
	setupLog("Qt platform plugin candidates=%q selected=%s", matches, matches[0])
	return matches[0], nil
}

func qtKey(name string) string {
	if i := strings.IndexByte(name, '='); i >= 0 {
		return strings.ToUpper(name[:i])
	}
	return strings.ToUpper(name)
}

func cleanQtEnvironment() []string {
	blocked := map[string]bool{
		"QT_PLUGIN_PATH":              true,
		"QT_QPA_PLATFORM_PLUGIN_PATH": true,
		"QT_QPA_PLATFORM":             true,
		"QT_DEBUG_PLUGINS":            true,
		"QT_LOGGING_RULES":            true,
		"PATH":                        true,
	}
	env := make([]string, 0, len(os.Environ()))
	for _, item := range os.Environ() {
		if !blocked[qtKey(item)] {
			env = append(env, item)
		}
	}
	return env
}

func selectorEnvironment(pluginPath string) []string {
	pluginDir := filepath.Dir(pluginPath)
	pluginRoot := filepath.Dir(pluginDir)
	env := cleanQtEnvironment()
	pathValue := os.Getenv("PATH")
	if pathValue == "" {
		pathValue = apoDir()
	} else {
		pathValue = apoDir() + ";" + pluginRoot + ";" + pluginDir + ";" + pathValue
	}
	env = append(env,
		"QT_QPA_PLATFORM=windows",
		"QT_QPA_PLATFORM_PLUGIN_PATH="+pluginDir,
		"QT_PLUGIN_PATH="+pluginRoot,
		"QT_DEBUG_PLUGINS=1",
		"QT_LOGGING_RULES=qt.qpa.*=true",
		"PATH="+pathValue,
	)
	return env
}

func logSelectorDiagnostics(selector, plugin string) {
	setupLog("device selector diagnostics: selector=%s plugin=%s workDir=%s", selector, plugin, apoDir())
	for _, name := range []string{"QT_PLUGIN_PATH", "QT_QPA_PLATFORM_PLUGIN_PATH", "QT_QPA_PLATFORM", "PATH"} {
		setupLog("parent environment %s=%q", name, os.Getenv(name))
	}
	for _, path := range []string{selector, plugin, filepath.Join(apoDir(), "Qt6Core.dll"), filepath.Join(apoDir(), "Qt6Gui.dll"), filepath.Join(apoDir(), "Qt6Widgets.dll")} {
		if info, err := os.Stat(path); err == nil {
			setupLog("dependency exists: path=%s size=%d", path, info.Size())
		} else {
			setupLog("dependency missing: path=%s err=%v", path, err)
		}
	}
}

func runDeviceSelector() error {
	selector := selectorPath()
	if !fileExists(selector) {
		return fmt.Errorf("Equalizer APO device selector was not found at %s", selector)
	}
	plugin, err := findQtPlatformPlugin()
	if err != nil {
		return fmt.Errorf("Qt platform plugin is missing: %w", err)
	}
	logSelectorDiagnostics(selector, plugin)

	cmd := exec.Command(selector)
	cmd.Dir = apoDir()
	cmd.Env = selectorEnvironment(plugin)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
	setupLog("starting device selector with isolated Qt environment")
	output, runErr := cmd.CombinedOutput()
	if text := strings.TrimSpace(string(output)); text != "" {
		setupLog("device selector output: %s", text)
	}
	if runErr != nil {
		setupLog("device selector failed: %v", runErr)
		return fmt.Errorf("device selector exited with an error: %w", runErr)
	}
	setupLog("device selector exited normally")
	return nil
}

func apoInstallationHealthy() (bool, string) {
	selector := selectorPath()
	if !fileExists(selector) {
		return false, "device selector executable is missing"
	}
	if _, err := findQtPlatformPlugin(); err != nil {
		return false, err.Error()
	}
	return true, "device selector and Qt platform plugin are present"
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func payloadHasOfficialAPO() bool {
	return len(apoPayload) > 5*1024*1024 && strings.EqualFold(sha256Hex(apoPayload), expectedAPOHash)
}

func verifyFile(path string) error {
	setupLog("verifying SHA-256: path=%s", path)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expectedAPOHash) {
		setupLog("checksum mismatch: expected=%s actual=%s", expectedAPOHash, actual)
		return fmt.Errorf("checksum mismatch: got %s", actual)
	}
	setupLog("checksum verified: %s", actual)
	return nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	setupLog("write file: path=%s bytes=%d", path, len(data))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	_ = os.Remove(path)
	if err := os.Rename(tmp, path); err != nil {
		setupLog("rename failed: temp=%s target=%s err=%v", tmp, path, err)
		return err
	}
	return nil
}

func downloadFile(url, dst string) error {
	setupLog("download begin: url=%s dst=%s", url, dst)
	client := &http.Client{Timeout: 15 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "VolumeBoostSetup/1.0 (+Windows)")
	req.Header.Set("Accept", "application/octet-stream,*/*")
	resp, err := client.Do(req)
	if err != nil {
		setupLog("download request failed: %v", err)
		return err
	}
	defer resp.Body.Close()
	setupLog("download response: status=%s contentType=%q length=%d", resp.Status, resp.Header.Get("Content-Type"), resp.ContentLength)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download returned HTTP %s", resp.Status)
	}
	if ct := strings.ToLower(resp.Header.Get("Content-Type")); strings.Contains(ct, "text/html") {
		return errors.New("download server returned a web page instead of the installer")
	}

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(f, resp.Body)
	setupLog("download copied bytes=%d copyErr=%v", written, copyErr)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return verifyFile(dst)
}

func obtainAPOInstaller(tempDir string) (string, error) {
	path := filepath.Join(tempDir, "EqualizerAPO-x64-1.4.2.exe")
	embedded := payloadHasOfficialAPO()
	setupLog("obtain APO installer: embeddedVerified=%t payloadBytes=%d", embedded, len(apoPayload))
	if embedded {
		if err := writeFileAtomic(path, apoPayload, 0755); err != nil {
			return "", err
		}
		setupLog("using verified embedded Equalizer APO installer")
		return path, nil
	}

	setupLog("embedded APO unavailable; using verified online download")
	urls := []string{
		"https://sourceforge.net/projects/equalizerapo/files/1.4.2/EqualizerAPO-x64-1.4.2.exe/download",
		"https://downloads.sourceforge.net/project/equalizerapo/1.4.2/EqualizerAPO-x64-1.4.2.exe",
	}
	var lastErr error
	for _, url := range urls {
		_ = os.Remove(path)
		if err := downloadFile(url, path); err == nil {
			setupLog("download succeeded: url=%s", url)
			return path, nil
		} else {
			setupLog("download failed: url=%s err=%v", url, err)
			lastErr = err
		}
	}
	return "", fmt.Errorf("could not download the verified Equalizer APO installer: %w", lastErr)
}

func runHidden(name string, args ...string) error {
	setupLog("run command: %s %q", name, args)
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if trimmed != "" {
		setupLog("command output: %s", trimmed)
	}
	if err != nil {
		setupLog("command failed: %v", err)
		return fmt.Errorf("%s: %w", trimmed, err)
	}
	setupLog("command completed successfully")
	return nil
}

func runAPOInstaller(path string) error {
	setupLog("starting Equalizer APO installer: path=%s", path)
	cmd := exec.Command(path, "/S")
	// Do not let unrelated Qt environment variables from another application
	// poison Equalizer APO's own deployment or any child configurator process.
	cmd.Env = cleanQtEnvironment()
	output, err := cmd.CombinedOutput()
	if text := strings.TrimSpace(string(output)); text != "" {
		setupLog("Equalizer APO installer output: %s", text)
	}
	if err != nil {
		setupLog("Equalizer APO installer failed: %v", err)
		return err
	}
	setupLog("Equalizer APO installer completed")
	return nil
}

func grantDataPermissions() error {
	if err := os.MkdirAll(dataDir(), 0755); err != nil {
		return err
	}
	// Built-in Users: Modify; Local Service: Read. SIDs avoid localized group names.
	return runHidden("icacls.exe", dataDir(),
		"/grant", "*S-1-5-32-545:(OI)(CI)M",
		"*S-1-5-19:(OI)(CI)R", "/T", "/C")
}

func grantManagedConfigPermissions() error {
	if err := os.MkdirAll(managedConfigDir(), 0755); err != nil {
		return err
	}
	// Equalizer APO only watches its configured directory tree for changes.
	// Grant standard users Modify access only to YetAnotherVolumeBooster's private subfolder,
	// never to config.txt or the rest of Equalizer APO.
	return runHidden("icacls.exe", managedConfigDir(),
		"/grant", "*S-1-5-32-545:(OI)(CI)M",
		"*S-1-5-19:(OI)(CI)R", "/T", "/C")
}

func managedBlock() string {
	// Keep the managed file inside Equalizer APO's watched config subtree so
	// moving the slider causes an immediate configuration reload.
	return "# BEGIN YetAnotherVolumeBooster\r\nInclude: YetAnotherVolumeBooster\\gain.txt\r\n# END YetAnotherVolumeBooster"
}

func removeManagedBlock(content string) string {
	re := regexp.MustCompile(`(?is)\r?\n?# BEGIN YetAnotherVolumeBooster\r?\n.*?# END YetAnotherVolumeBooster\r?\n?`)
	return strings.TrimSpace(re.ReplaceAllString(content, ""))
}

func integrateConfig() error {
	setupLog("integrating APO config: config=%s gain=%s", apoConfig(), gainPath())
	if err := os.MkdirAll(filepath.Dir(apoConfig()), 0755); err != nil {
		return err
	}
	data, err := os.ReadFile(apoConfig())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(err) {
		data = nil
	}

	backup := apoConfig() + ".YetAnotherVolumeBooster-backup"
	setupLog("config backup path=%s", backup)
	if _, err := os.Stat(backup); os.IsNotExist(err) {
		if err := os.WriteFile(backup, data, 0644); err != nil {
			return err
		}
	}

	base := removeManagedBlock(string(data))
	var merged string
	if base == "" {
		merged = managedBlock() + "\r\n"
	} else {
		merged = strings.ReplaceAll(base, "\n", "\r\n")
		merged = strings.ReplaceAll(merged, "\r\r\n", "\r\n")
		merged += "\r\n\r\n" + managedBlock() + "\r\n"
	}
	return writeFileAtomic(apoConfig(), []byte(merged), 0644)
}

func removeIntegration() error {
	data, err := os.ReadFile(apoConfig())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	clean := removeManagedBlock(string(data))
	if clean != "" {
		clean += "\r\n"
	}
	return writeFileAtomic(apoConfig(), []byte(clean), 0644)
}

func copySelf() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	selfAbs, _ := filepath.Abs(self)
	dstAbs, _ := filepath.Abs(setupPath())
	if strings.EqualFold(selfAbs, dstAbs) {
		return nil
	}
	data, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	return writeFileAtomic(setupPath(), data, 0755)
}

func installFiles() error {
	setupLog("installing application files: installDir=%s", installDir())
	if err := os.MkdirAll(installDir(), 0755); err != nil {
		return err
	}
	if err := writeFileAtomic(appPath(), appPayload, 0755); err != nil {
		return err
	}
	if err := writeFileAtomic(iconPath(), iconPayload, 0644); err != nil {
		return err
	}
	if err := copySelf(); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(installDir(), "GPL-2.0.txt"), gplPayload, 0644); err != nil {
		return err
	}
	thirdParty := []byte("Equalizer APO 1.4.2\r\nOfficial project: https://sourceforge.net/projects/equalizerapo/\r\nSource: https://sourceforge.net/p/equalizerapo/code/\r\nLicense: GNU GPL v2\r\n")
	return writeFileAtomic(filepath.Join(installDir(), "THIRD-PARTY-NOTICES.txt"), thirdParty, 0644)
}

func ensureGainFile() error {
	if _, err := os.Stat(gainPath()); err == nil {
		return nil
	}

	content := []byte("# Managed by YetAnotherVolumeBooster\r\n# 100% = +0.00 dB\r\nPreamp: 0.00 dB\r\n")
	if legacy, err := os.ReadFile(legacyGainPath()); err == nil {
		if regexp.MustCompile(`(?i)Preamp:\s*[+-]?[0-9]+(?:\.[0-9]+)?\s*dB`).Match(legacy) {
			setupLog("migrating legacy gain file: from=%s to=%s", legacyGainPath(), gainPath())
			content = legacy
		}
	}
	if err := writeFileAtomic(gainPath(), content, 0644); err != nil {
		return err
	}
	// The old file lived outside Equalizer APO's watched tree and is no longer used.
	_ = os.Remove(legacyGainPath())
	return nil
}

func restartAudioService() error {
	setupLog("restarting Windows Audio service so the selected APO becomes active")
	stopErr := runHidden("net.exe", "stop", "Audiosrv", "/y")
	if stopErr != nil {
		setupLog("Windows Audio stop failed: %v", stopErr)
		return stopErr
	}
	time.Sleep(750 * time.Millisecond)
	if err := runHidden("net.exe", "start", "Audiosrv"); err != nil {
		setupLog("Windows Audio start failed: %v", err)
		return err
	}
	setupLog("Windows Audio service restarted successfully")
	return nil
}

func psQuote(s string) string { return strings.ReplaceAll(s, "'", "''") }

func createShortcut(path, target, workingDir, icon string) error {
	script := fmt.Sprintf("$w=New-Object -ComObject WScript.Shell;$s=$w.CreateShortcut('%s');$s.TargetPath='%s';$s.WorkingDirectory='%s';$s.IconLocation='%s,0';$s.Save()",
		psQuote(path), psQuote(target), psQuote(workingDir), psQuote(icon))
	return runHidden("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
}

func createShortcuts() error {
	setupLog("creating shortcuts")
	startMenu := filepath.Join(programData(), "Microsoft", "Windows", "Start Menu", "Programs", "YetAnotherVolumeBooster.lnk")
	if err := createShortcut(startMenu, appPath(), installDir(), iconPath()); err != nil {
		return err
	}
	// Use the user's actual Desktop shell folder, including OneDrive redirection.
	script := fmt.Sprintf("$w=New-Object -ComObject WScript.Shell;$d=$w.SpecialFolders('Desktop');$s=$w.CreateShortcut((Join-Path $d 'YetAnotherVolumeBooster.lnk'));$s.TargetPath='%s';$s.WorkingDirectory='%s';$s.IconLocation='%s,0';$s.Save()",
		psQuote(appPath()), psQuote(installDir()), psQuote(iconPath()))
	return runHidden("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
}

func removeShortcuts() {
	_ = os.Remove(filepath.Join(programData(), "Microsoft", "Windows", "Start Menu", "Programs", "YetAnotherVolumeBooster.lnk"))
	script := "$w=New-Object -ComObject WScript.Shell;$p=Join-Path $w.SpecialFolders('Desktop') 'YetAnotherVolumeBooster.lnk';Remove-Item $p -Force -ErrorAction SilentlyContinue"
	_ = runHidden("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
}

func addUninstallEntry() error {
	key := `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\YetAnotherVolumeBooster`
	values := [][]string{
		{"add", key, "/v", "DisplayName", "/t", "REG_SZ", "/d", appName, "/f"},
		{"add", key, "/v", "DisplayVersion", "/t", "REG_SZ", "/d", appVersion, "/f"},
		{"add", key, "/v", "Publisher", "/t", "REG_SZ", "/d", "YetAnotherVolumeBooster", "/f"},
		{"add", key, "/v", "InstallLocation", "/t", "REG_SZ", "/d", installDir(), "/f"},
		{"add", key, "/v", "DisplayIcon", "/t", "REG_SZ", "/d", iconPath(), "/f"},
		{"add", key, "/v", "UninstallString", "/t", "REG_SZ", "/d", `"` + setupPath() + `" --uninstall`, "/f"},
		{"add", key, "/v", "NoModify", "/t", "REG_DWORD", "/d", "1", "/f"},
	}
	for _, args := range values {
		if err := runHidden("reg.exe", args...); err != nil {
			return err
		}
	}
	return nil
}

func removeUninstallEntry() {
	_ = runHidden("reg.exe", "delete", `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\YetAnotherVolumeBooster`, "/f")
}

func stopRunningController() {
	setupLog("stopping any running YetAnotherVolumeBooster controller before update")
	cmd := exec.Command("taskkill.exe", "/IM", "YetAnotherVolumeBooster.exe", "/F")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := cmd.CombinedOutput()
	if text := strings.TrimSpace(string(output)); text != "" {
		setupLog("taskkill output: %s", text)
	}
	if err != nil {
		setupLog("taskkill result (usually means no running process): %v", err)
	} else {
		setupLog("running controller stopped")
		time.Sleep(350 * time.Millisecond)
	}
}

func launchControllerDetached(path string) error {
	const (
		createNewProcessGroup  = 0x00000200
		detachedProcess        = 0x00000008
		createBreakawayFromJob = 0x01000000
	)

	// First try to break away from any browser/installer job object. Some download
	// launchers close their job after setup exits and would otherwise kill the UI.
	setupLog("detached controller launch request: %s", path)
	cmd := exec.Command(path)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:       true,
		CreationFlags:    createNewProcessGroup | detachedProcess | createBreakawayFromJob,
		NoInheritHandles: true,
	}
	if err := cmd.Start(); err == nil {
		pid := cmd.Process.Pid
		_ = cmd.Process.Release()
		setupLog("controller launched with breakaway flags: pid=%d", pid)
		return nil
	} else {
		setupLog("breakaway launch failed, falling back to Explorer broker: %v", err)
	}

	// Explorer is a long-lived shell process outside the installer job. Passing
	// the executable through Explorer prevents it from dying with setup.
	broker := exec.Command("explorer.exe", path)
	broker.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, NoInheritHandles: true}
	if err := broker.Start(); err != nil {
		setupLog("Explorer broker launch failed: %v", err)
		return err
	}
	pid := broker.Process.Pid
	_ = broker.Process.Release()
	setupLog("Explorer broker request started: pid=%d", pid)
	return nil
}

func installOrRepair(repair bool, launchApp bool) error {
	setupLog("installOrRepair begin: repair=%t launchApp=%t", repair, launchApp)
	setupLog("paths: installDir=%s dataDir=%s apoDir=%s config=%s selector=%s", installDir(), dataDir(), apoDir(), apoConfig(), selectorPath())

	apoWasPresent := fileExists(selectorPath())
	healthy, healthReason := apoInstallationHealthy()
	setupLog("Equalizer APO present=%t healthy=%t reason=%s", apoWasPresent, healthy, healthReason)
	apoInstalledOrRepaired := false

	if !healthy {
		if apoWasPresent {
			messageBox("Equalizer APO's device selector is incomplete or its Qt platform plugin is missing.\n\nVolumeBoost will repair it using the verified official Equalizer APO 1.4.2 installer.", appName+" Setup", MB_OK|MB_ICONINFORMATION)
		} else {
			messageBox("YetAnotherVolumeBooster will now install the official Equalizer APO 1.4.2 audio engine.\n\nAfter installation, select the speakers, headphones, or Bluetooth output you actually use.", appName+" Setup", MB_OK|MB_ICONINFORMATION)
		}

		tempDir, err := os.MkdirTemp("", "VolumeBoostSetup-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tempDir)
		installer, err := obtainAPOInstaller(tempDir)
		if err != nil {
			return err
		}
		if err := runAPOInstaller(installer); err != nil {
			return fmt.Errorf("Equalizer APO setup failed: %w", err)
		}

		healthy, healthReason = apoInstallationHealthy()
		setupLog("post-install Equalizer APO health: healthy=%t reason=%s", healthy, healthReason)
		if !healthy {
			return fmt.Errorf("Equalizer APO installation is still incomplete: %s", healthReason)
		}
		apoInstalledOrRepaired = true
		_ = os.MkdirAll(dataDir(), 0755)
		_ = os.WriteFile(markerPath(), []byte(time.Now().Format(time.RFC3339)), 0644)
	}

	controllerInvokedRepair := repair && !launchApp
	if controllerInvokedRepair {
		setupLog("controller-invoked repair: skipping executable replacement while YetAnotherVolumeBooster.exe is running")
	} else {
		stopRunningController()
		setupLog("step: install application files")
		if err := installFiles(); err != nil {
			return fmt.Errorf("install application files: %w", err)
		}
	}
	setupLog("step: grant data permissions")
	if err := grantDataPermissions(); err != nil {
		return fmt.Errorf("set data permissions: %w", err)
	}
	setupLog("step: ensure gain file inside Equalizer APO config tree")
	if err := ensureGainFile(); err != nil {
		return fmt.Errorf("create gain configuration: %w", err)
	}
	setupLog("step: grant managed Equalizer APO config permissions")
	if err := grantManagedConfigPermissions(); err != nil {
		return fmt.Errorf("set managed config permissions: %w", err)
	}
	setupLog("step: integrate Equalizer APO configuration")
	if err := integrateConfig(); err != nil {
		return fmt.Errorf("integrate Equalizer APO configuration: %w", err)
	}
	if controllerInvokedRepair {
		setupLog("controller-invoked repair: existing shortcuts and uninstall entry left unchanged")
	} else {
		setupLog("step: create shortcuts")
		if err := createShortcuts(); err != nil {
			return fmt.Errorf("create shortcuts: %w", err)
		}
		setupLog("step: register uninstaller")
		if err := addUninstallEntry(); err != nil {
			return fmt.Errorf("register uninstaller: %w", err)
		}
	}

	if repair || apoInstalledOrRepaired || apoWasPresent {
		setupLog("opening device selector through isolated Qt launcher")
		if err := runDeviceSelector(); err != nil {
			return fmt.Errorf("open Equalizer APO device selector: %w", err)
		}
		if err := restartAudioService(); err != nil {
			setupLog("automatic audio-service restart failed; a Windows restart is required: %v", err)
			messageBox("The audio device configuration was saved, but Windows Audio could not be restarted automatically.\n\nRestart Windows once before testing YetAnotherVolumeBooster.", appName, MB_OK|MB_ICONINFORMATION)
		}
	}
	if launchApp {
		setupLog("launching controller independently: %s", appPath())
		if err := launchControllerDetached(appPath()); err != nil {
			return fmt.Errorf("launch YetAnotherVolumeBooster: %w", err)
		}
	} else {
		setupLog("controller launch skipped by --no-launch")
	}
	setupLog("installOrRepair completed successfully")
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func uninstall() error {
	setupLog("uninstall requested")
	if messageBox("Remove YetAnotherVolumeBooster and its gain configuration?\n\nEqualizer APO will be left installed so other audio configurations are not damaged.", "Uninstall YetAnotherVolumeBooster", MB_YESNO|MB_ICONQUESTION) != IDYES {
		return nil
	}
	if err := removeIntegration(); err != nil {
		return err
	}
	_ = os.RemoveAll(managedConfigDir())
	removeShortcuts()
	removeUninstallEntry()
	_ = runHidden("reg.exe", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", appName, "/f")
	_ = os.RemoveAll(dataDir())

	// Delete the installation directory after this executable exits.
	cmdLine := fmt.Sprintf(`timeout /t 2 /nobreak >nul & rmdir /s /q "%s"`, installDir())
	cmd := exec.Command("cmd.exe", "/C", cmdLine)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
	messageBox("YetAnotherVolumeBooster was removed.\n\nEqualizer APO remains installed and can be removed separately from Windows Settings if you no longer use it.", "YetAnotherVolumeBooster", MB_OK|MB_ICONINFORMATION)
	return nil
}

func main() {
	initSetupLogging()
	defer closeSetupLogging()
	defer func() {
		if recovered := recover(); recovered != nil {
			setupLogPanic("main", recovered)
			messageBox("YetAnotherVolumeBooster Setup crashed.\n\nDiagnostic log:\n"+setupLogLocation(), appName+" Setup", MB_OK|MB_ICONERROR)
		}
	}()

	admin := isAdmin()
	setupLog("administrator=%t", admin)
	if !admin {
		if err := relaunchElevated(); err != nil {
			setupLog("administrator relaunch failed: %v", err)
			messageBox("Administrator permission is required.\n\n"+err.Error()+"\n\nLog: "+setupLogLocation(), appName+" Setup", MB_OK|MB_ICONERROR)
		}
		return
	}

	if hasArg("--device-selector") {
		setupLog("mode: device-selector helper")
		if err := runDeviceSelector(); err != nil {
			setupLog("device selector helper failed: %v", err)
			messageBox("Device setup could not start:\n\n"+err.Error()+"\n\nDiagnostic log:\n"+setupLogLocation()+"\n\nUse Repair integration if the Qt platform plugin is missing.", appName, MB_OK|MB_ICONERROR)
		}
		return
	}

	if hasArg("--uninstall") {
		if err := uninstall(); err != nil {
			messageBox("Uninstall failed:\n\n"+err.Error(), appName, MB_OK|MB_ICONERROR)
		}
		return
	}

	repair := hasArg("--repair")
	launchApp := !hasArg("--no-launch")
	setupLog("mode: repair=%t launchApp=%t uninstall=%t", repair, launchApp, hasArg("--uninstall"))
	if !repair {
		text := "Install YetAnotherVolumeBooster?\n\nThis installs the redesigned system-wide 100–500% gain controller with tray controls and startup options. Equalizer APO will be installed automatically if it is not already present.\n\nThe setup verifies the official installer's SHA-256 checksum before running it."
		if messageBox(text, appName+" Setup", MB_YESNO|MB_ICONQUESTION) != IDYES {
			return
		}
	}

	if err := installOrRepair(repair, launchApp); err != nil {
		setupLog("setup failed: %v", err)
		messageBox("Setup failed:\n\n"+err.Error()+"\n\nDiagnostic log:\n"+setupLogLocation()+"\n\nNo unverified audio component was installed.", appName+" Setup", MB_OK|MB_ICONERROR)
		return
	}

	message := "YetAnotherVolumeBooster 1.7.0 is ready.\n\nThe emerald UI, dark mode, smooth controls, startup option, and close-to-tray setting are installed. Exiting YetAnotherVolumeBooster now resets APO gain to 100%."
	if repair {
		message = "YetAnotherVolumeBooster integration was repaired.\n\nThe live gain configuration was rebuilt and Windows Audio was restarted. Boost levels above 100% now use Windows 100% as their baseline."
	}
	setupLog("showing success message")
	messageBox(message+"\n\nDiagnostic log:\n"+setupLogLocation(), appName, MB_OK|MB_ICONINFORMATION)
}
