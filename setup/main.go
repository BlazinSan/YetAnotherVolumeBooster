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
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	appName    = "YetAnotherVolumeBooster"
	appVersion = "1.7.0"

	expectedAPOHash = "7403be7427bbe1936a40dded082829b6e217fc4f5990fee5cba501f0ae055afa"

	MB_OK              = 0x00000000
	MB_YESNO           = 0x00000004
	MB_ICONERROR       = 0x00000010
	MB_ICONQUESTION    = 0x00000020
	MB_ICONINFORMATION = 0x00000040
	MB_TOPMOST         = 0x00040000
	IDCANCEL           = 2
	IDYES              = 6
	SW_SHOWNORMAL      = 1

	imageIcon      = 1
	lrLoadFromFile = 0x00000010

	wsOverlapped   = 0x00000000
	wsCaption      = 0x00C00000
	wsSysMenu      = 0x00080000
	wsChild        = 0x40000000
	wsVisible      = 0x10000000
	wsTabStop      = 0x00010000
	wsBorder       = 0x00800000
	wsClipChildren = 0x02000000
	wsExDialog     = 0x00000001
	wsExTopmost    = 0x00000008
	esMultiline    = 0x0004
	esAutovscroll  = 0x0040
	esNoHideSel    = 0x0100
	esWantReturn   = 0x1000
	bsDefPush      = 0x00000001
	wmCreate       = 0x0001
	wmDestroy      = 0x0002
	wmCommand      = 0x0111
	wmClose        = 0x0010
	wmPaint        = 0x000F
	wmSetFont      = 0x0030
	wmGetText      = 0x000D
	wmGetTextLen   = 0x000E
	cwUseDefault   = 0x80000000
	defaultGUIFont = 17
	idFeedbackEdit = 3001
	idFeedbackSend = 3002
	idFeedbackBack = 3003

	tdfUseHIconMain            = 0x0002
	tdfAllowDialogCancel       = 0x0008
	tdfUseCommandLinks         = 0x0010
	tdfSizeToContent           = 0x01000000
	taskOpenApp          int32 = 1001
	taskOpenSound        int32 = 1002
	taskDone             int32 = 1003
	taskReasonAudio      int32 = 2001
	taskReasonDesign     int32 = 2002
	taskReasonNoNeed     int32 = 2003
	taskReasonSkip       int32 = 2004

	dtLeft       = 0x00000000
	dtVCenter    = 0x00000004
	dtWordBreak  = 0x00000010
	dtSingleLine = 0x00000020
	transparent  = 1
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

	//go:embed payload/OFL-PlayfairDisplay.txt
	playfairLicensePayload []byte

	user32  = syscall.NewLazyDLL("user32.dll")
	shell32 = syscall.NewLazyDLL("shell32.dll")
	comctl  = syscall.NewLazyDLL("comctl32.dll")
	gdi32   = syscall.NewLazyDLL("gdi32.dll")

	procMessageBoxW      = user32.NewProc("MessageBoxW")
	procLoadImageW       = user32.NewProc("LoadImageW")
	procDestroyIcon      = user32.NewProc("DestroyIcon")
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procUpdateWindow     = user32.NewProc("UpdateWindow")
	procBeginPaint       = user32.NewProc("BeginPaint")
	procEndPaint         = user32.NewProc("EndPaint")
	procDrawTextW        = user32.NewProc("DrawTextW")
	procFillRect         = user32.NewProc("FillRect")
	procSetFocus         = user32.NewProc("SetFocus")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procSendMessageW     = user32.NewProc("SendMessageW")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procGetStockObject   = gdi32.NewProc("GetStockObject")
	procCreateFontW      = gdi32.NewProc("CreateFontW")
	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject     = gdi32.NewProc("DeleteObject")
	procSelectObject     = gdi32.NewProc("SelectObject")
	procSetBkMode        = gdi32.NewProc("SetBkMode")
	procSetTextColor     = gdi32.NewProc("SetTextColor")
	procShellExecute     = shell32.NewProc("ShellExecuteW")
	procIsUserAdmin      = shell32.NewProc("IsUserAnAdmin")
	procTaskDialog       = comctl.NewProc("TaskDialogIndirect")
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

type taskDialogButton struct {
	nButtonID     int32
	pszButtonText *uint16
}

type taskDialogConfig struct {
	cbSize                  uint32
	hwndParent              syscall.Handle
	hInstance               syscall.Handle
	dwFlags                 uint32
	dwCommonButtons         uint32
	pszWindowTitle          *uint16
	hMainIcon               uintptr
	pszMainInstruction      *uint16
	pszContent              *uint16
	cButtons                uint32
	pButtons                *taskDialogButton
	nDefaultButton          int32
	cRadioButtons           uint32
	pRadioButtons           uintptr
	nDefaultRadioButton     int32
	pszVerificationText     *uint16
	pszExpandedInformation  *uint16
	pszExpandedControlText  *uint16
	pszCollapsedControlText *uint16
	hFooterIcon             uintptr
	pszFooter               *uint16
	pfCallback              uintptr
	lpCallbackData          uintptr
	cxWidth                 uint32
}

type taskChoice struct {
	id   int32
	text string
}

type setupWndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     syscall.Handle
	hIcon         syscall.Handle
	hCursor       syscall.Handle
	hbrBackground syscall.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       syscall.Handle
}

type setupPoint struct{ x, y int32 }

type setupRect struct {
	left, top, right, bottom int32
}

type setupPaintStruct struct {
	hdc         syscall.Handle
	fErase      int32
	rcPaint     setupRect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type setupMsg struct {
	hwnd    syscall.Handle
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      setupPoint
	private uint32
}

var (
	feedbackWndProcCallback uintptr
	feedbackWindow          syscall.Handle
	feedbackEdit            syscall.Handle
	feedbackResult          string
	feedbackAccepted        bool
	feedbackTitleFont       syscall.Handle
	feedbackBodyFont        syscall.Handle
	feedbackButtonFont      syscall.Handle
	feedbackSendDone        chan struct{}
)

func lowordSetup(v uintptr) int32 { return int32(v & 0xffff) }

func setupRGB(r, g, b uint8) uintptr {
	return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16
}

func createSetupFont(size, weight int32, face string) syscall.Handle {
	h, _, _ := procCreateFontW.Call(
		uintptr(-size), 0, 0, 0,
		uintptr(weight), 0, 0, 0,
		1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(utf16(face))),
	)
	return syscall.Handle(h)
}

func ensureFeedbackFonts() {
	if feedbackTitleFont == 0 {
		feedbackTitleFont = createSetupFont(19, 650, "Segoe UI Variable Display")
	}
	if feedbackBodyFont == 0 {
		feedbackBodyFont = createSetupFont(14, 400, "Segoe UI Variable Text")
	}
	if feedbackButtonFont == 0 {
		feedbackButtonFont = createSetupFont(13, 500, "Segoe UI Variable Text")
	}
}

func destroyFeedbackFonts() {
	for _, font := range []syscall.Handle{feedbackTitleFont, feedbackBodyFont, feedbackButtonFont} {
		if font != 0 {
			procDeleteObject.Call(uintptr(font))
		}
	}
	feedbackTitleFont, feedbackBodyFont, feedbackButtonFont = 0, 0, 0
}

func drawSetupText(hdc syscall.Handle, text string, target setupRect, font syscall.Handle, color uintptr, flags uintptr) {
	old, _, _ := procSelectObject.Call(uintptr(hdc), uintptr(font))
	procSetBkMode.Call(uintptr(hdc), transparent)
	procSetTextColor.Call(uintptr(hdc), color)
	procDrawTextW.Call(
		uintptr(hdc),
		uintptr(unsafe.Pointer(utf16(text))),
		uintptr(len([]rune(text))),
		uintptr(unsafe.Pointer(&target)),
		flags,
	)
	procSelectObject.Call(uintptr(hdc), old)
}

func paintFeedbackWindow(hwnd syscall.Handle) {
	var ps setupPaintStruct
	hdcRaw, _, _ := procBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
	if hdcRaw == 0 {
		return
	}
	defer procEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
	hdc := syscall.Handle(hdcRaw)
	bg, _, _ := procCreateSolidBrush.Call(setupRGB(248, 251, 249))
	if bg != 0 {
		full := setupRect{left: 0, top: 0, right: 620, bottom: 360}
		procFillRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(&full)), bg)
		procDeleteObject.Call(bg)
	}
	text := setupRGB(28, 40, 39)
	muted := setupRGB(87, 105, 103)
	drawSetupText(hdc, "We're sad to see you go.", setupRect{left: 30, top: 24, right: 570, bottom: 56}, feedbackTitleFont, text, dtLeft|dtVCenter|dtSingleLine)
	drawSetupText(hdc, "Before YetAnotherVolumeBooster is removed, tell us what made you uninstall it.", setupRect{left: 30, top: 66, right: 570, bottom: 104}, feedbackBodyFont, text, dtLeft|dtWordBreak)
	drawSetupText(hdc, "Your reason and basic app diagnostics will be sent to the developer.", setupRect{left: 30, top: 108, right: 570, bottom: 142}, feedbackBodyFont, muted, dtLeft|dtWordBreak)
}

func loadDialogIcon() syscall.Handle {
	if !fileExists(iconPath()) {
		return 0
	}
	h, _, _ := procLoadImageW.Call(
		0,
		uintptr(unsafe.Pointer(utf16(iconPath()))),
		imageIcon,
		32,
		32,
		lrLoadFromFile,
	)
	return syscall.Handle(h)
}

func showTaskDialog(title, instruction, content, footer string, choices []taskChoice, defaultID int32) (int32, error) {
	buttons := make([]taskDialogButton, len(choices))
	buttonText := make([]*uint16, len(choices))
	for i, choice := range choices {
		buttonText[i] = utf16(choice.text)
		buttons[i] = taskDialogButton{nButtonID: choice.id, pszButtonText: buttonText[i]}
	}

	icon := loadDialogIcon()
	if icon != 0 {
		defer procDestroyIcon.Call(uintptr(icon))
	}
	flags := uint32(tdfAllowDialogCancel | tdfUseCommandLinks | tdfSizeToContent)
	if icon != 0 {
		flags |= tdfUseHIconMain
	}

	var clicked int32
	config := taskDialogConfig{
		cbSize:             uint32(unsafe.Sizeof(taskDialogConfig{})),
		dwFlags:            flags,
		pszWindowTitle:     utf16(title),
		hMainIcon:          uintptr(icon),
		pszMainInstruction: utf16(instruction),
		pszContent:         utf16(content),
		cButtons:           uint32(len(buttons)),
		pButtons:           &buttons[0],
		nDefaultButton:     defaultID,
		pszFooter:          utf16(footer),
		cxWidth:            0,
	}
	hr, _, callErr := procTaskDialog.Call(
		uintptr(unsafe.Pointer(&config)),
		uintptr(unsafe.Pointer(&clicked)),
		0,
		0,
	)
	runtime.KeepAlive(buttonText)
	runtime.KeepAlive(buttons)
	runtime.KeepAlive(config)
	if hr != 0 {
		setupLog("TaskDialogIndirect failed: HRESULT=0x%08X err=%v", uint32(hr), callErr)
		messageBox(instruction+"\n\n"+content, title, MB_OK|MB_ICONINFORMATION)
		return defaultID, fmt.Errorf("TaskDialogIndirect failed: HRESULT 0x%08X", uint32(hr))
	}
	return clicked, nil
}

func setControlFont(hwnd syscall.Handle, font uintptr) {
	if hwnd != 0 && font != 0 {
		procSendMessageW.Call(uintptr(hwnd), wmSetFont, font, 1)
	}
}

func createFeedbackChild(parent syscall.Handle, className, text string, style uintptr, x, y, w, h int32, id int32, font uintptr) syscall.Handle {
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16(className))),
		uintptr(unsafe.Pointer(utf16(text))),
		style|wsChild|wsVisible,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		uintptr(parent), uintptr(id), 0, 0,
	)
	child := syscall.Handle(hwnd)
	setControlFont(child, font)
	return child
}

func feedbackText() string {
	if feedbackEdit == 0 {
		return ""
	}
	length, _, _ := procSendMessageW.Call(uintptr(feedbackEdit), wmGetTextLen, 0, 0)
	buf := make([]uint16, int(length)+1)
	if len(buf) == 0 {
		return ""
	}
	procSendMessageW.Call(uintptr(feedbackEdit), wmGetText, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	return strings.TrimSpace(syscall.UTF16ToString(buf))
}

func feedbackWndProc(hwnd syscall.Handle, message uint32, wParam, lParam uintptr) (result uintptr) {
	defer func() {
		if recovered := recover(); recovered != nil {
			setupLogPanic("feedbackWndProc", recovered)
			result = 0
		}
	}()

	switch message {
	case wmCreate:
		feedbackWindow = hwnd
		ensureFeedbackFonts()
		feedbackEdit = createFeedbackChild(hwnd, "EDIT", "", wsBorder|esMultiline|esAutovscroll|esNoHideSel|esWantReturn, 30, 152, 530, 110, idFeedbackEdit, uintptr(feedbackBodyFont))
		createFeedbackChild(hwnd, "BUTTON", "Uninstall", wsTabStop|bsDefPush, 348, 286, 102, 34, idFeedbackSend, uintptr(feedbackButtonFont))
		createFeedbackChild(hwnd, "BUTTON", "Cancel", wsTabStop, 462, 286, 98, 34, idFeedbackBack, uintptr(feedbackButtonFont))
		return 0
	case wmPaint:
		paintFeedbackWindow(hwnd)
		return 0
	case wmCommand:
		switch lowordSetup(wParam) {
		case idFeedbackSend:
			feedbackResult = feedbackText()
			if feedbackResult == "" {
				feedbackResult = "No reason provided."
			}
			feedbackAccepted = true
			procDestroyWindow.Call(uintptr(hwnd))
			return 0
		case idFeedbackBack:
			feedbackAccepted = false
			procDestroyWindow.Call(uintptr(hwnd))
			return 0
		}
	case wmClose:
		feedbackAccepted = false
		procDestroyWindow.Call(uintptr(hwnd))
		return 0
	case wmDestroy:
		feedbackWindow = 0
		feedbackEdit = 0
		destroyFeedbackFonts()
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ = procDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return result
}

func showUninstallFeedbackWindow() (string, bool) {
	feedbackWindow = 0
	feedbackEdit = 0
	feedbackResult = ""
	feedbackAccepted = false
	hInst, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	className := utf16(appName + "UninstallFeedback")
	feedbackWndProcCallback = syscall.NewCallback(feedbackWndProc)
	icon := loadDialogIcon()
	if icon != 0 {
		defer procDestroyIcon.Call(uintptr(icon))
	}
	wc := setupWndClassEx{
		cbSize:        uint32(unsafe.Sizeof(setupWndClassEx{})),
		lpfnWndProc:   feedbackWndProcCallback,
		hInstance:     syscall.Handle(hInst),
		hIcon:         icon,
		hbrBackground: syscall.Handle(6),
		lpszClassName: className,
		hIconSm:       icon,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	width, height := int32(620), int32(370)
	screenW, _, _ := procGetSystemMetrics.Call(0)
	screenH, _, _ := procGetSystemMetrics.Call(1)
	x := (int32(screenW) - width) / 2
	y := (int32(screenH) - height) / 2
	hwnd, _, _ := procCreateWindowExW.Call(
		wsExDialog|wsExTopmost,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(utf16("Uninstall "+appName))),
		wsOverlapped|wsCaption|wsSysMenu|wsClipChildren,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		setupLog("uninstall feedback window failed to create")
		return "No reason provided.", true
	}
	feedbackWindow = syscall.Handle(hwnd)
	procShowWindow.Call(hwnd, SW_SHOWNORMAL)
	procUpdateWindow.Call(hwnd)
	if feedbackEdit != 0 {
		procSetFocus.Call(uintptr(feedbackEdit))
	}
	var message setupMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
	}
	return feedbackResult, feedbackAccepted
}

func sendUninstallFeedback(reason string) {
	values := url.Values{}
	values.Set("_subject", appName+" uninstall feedback")
	values.Set("_captcha", "false")
	values.Set("app", appName)
	values.Set("version", appVersion)
	values.Set("reason", reason)
	values.Set("timestamp", time.Now().Format(time.RFC3339))
	values.Set("os", runtime.GOOS)
	values.Set("arch", runtime.GOARCH)
	values.Set("setup_log", setupLogLocation())
	if host, err := os.Hostname(); err == nil {
		values.Set("computer", host)
	}
	if username := os.Getenv("USERNAME"); username != "" {
		values.Set("windows_user", username)
	}
	client := http.Client{Timeout: 5 * time.Second}
	for _, endpoint := range []string{
		"https://formsubmit.co/ajax/hammau05@gmail.com",
		"https://formsubmit.co/hammau05@gmail.com",
	} {
		req, err := http.NewRequest("POST", endpoint, strings.NewReader(values.Encode()))
		if err != nil {
			setupLog("uninstall feedback request build failed for %s: %v", endpoint, err)
			continue
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			setupLog("uninstall feedback email send failed for %s: %v", endpoint, err)
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			setupLog("uninstall feedback email send accepted by %s: status=%d", endpoint, resp.StatusCode)
			return
		}
		setupLog("uninstall feedback email send rejected by %s: status=%d body=%s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

func startUninstallFeedbackSend(reason string) {
	feedbackSendDone = make(chan struct{})
	go func() {
		defer close(feedbackSendDone)
		sendUninstallFeedback(reason)
	}()
}

func waitForUninstallFeedback(maxWait time.Duration) {
	if feedbackSendDone == nil {
		return
	}
	select {
	case <-feedbackSendDone:
	case <-time.After(maxWait):
		setupLog("uninstall feedback send still running; continuing uninstall")
	}
	feedbackSendDone = nil
}

func shellOpen(path, params string) error {
	r, _, callErr := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(utf16("open"))),
		uintptr(unsafe.Pointer(utf16(path))),
		uintptr(unsafe.Pointer(utf16(params))),
		0,
		SW_SHOWNORMAL,
	)
	if r <= 32 {
		return fmt.Errorf("ShellExecute result %d: %w", r, callErr)
	}
	return nil
}

func openWindowsSoundSettings() {
	if err := shellOpen("ms-settings:sound", ""); err != nil {
		setupLog("ms-settings:sound failed: %v", err)
		if fallbackErr := shellOpen("control.exe", "mmsys.cpl"); fallbackErr != nil {
			setupLog("control mmsys.cpl fallback failed: %v", fallbackErr)
		}
	}
}

func showAudioOnboarding(status string) bool {
	content := "YetAnotherVolumeBooster updated its managed Equalizer APO gain file and config include. The app is ready for live 100-500% control."
	if status != "" {
		content += "\n\n" + status
	}
	choice, err := showTaskDialog(
		appName+" Audio Setup",
		"Audio engine connected",
		content,
		"Tip: if you switch headphones, speakers, or Bluetooth output, open Windows sound settings and pick the output you want to boost.",
		[]taskChoice{
			{taskOpenApp, "Open YetAnotherVolumeBooster\nStart or focus the boosted volume controller now."},
			{taskOpenSound, "Open Windows sound settings\nChoose your active playback output before testing boost."},
			{taskDone, "Done\nClose setup and keep the current audio configuration."},
		},
		taskOpenApp,
	)
	if err != nil {
		setupLog("audio onboarding fallback used: %v", err)
	}
	switch choice {
	case taskOpenSound:
		openWindowsSoundSettings()
		return false
	case taskOpenApp:
		if err := launchControllerDetached(appPath()); err != nil {
			setupLog("audio onboarding app launch failed: %v", err)
			return false
		}
		return true
	case IDCANCEL, 0:
		setupLog("audio onboarding closed without an action")
		return false
	default:
		return false
	}
}

func showUninstallFeedback() bool {
	reason, accepted := showUninstallFeedbackWindow()
	if !accepted {
		setupLog("uninstall cancelled from feedback dialog")
		return false
	}
	setupLog("uninstall feedback: %s", reason)
	startUninstallFeedbackSend(reason)
	return true
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

func unattendedSetup() bool {
	return hasArg("--yes") || hasArg("--quiet") || hasArg("--silent")
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
func setupPath() string        { return filepath.Join(installDir(), "YetAnotherVolumeBoosterSetup.exe") }
func iconPath() string         { return filepath.Join(installDir(), "YetAnotherVolumeBooster.ico") }
func dataDir() string          { return filepath.Join(programData(), appName) }
func markerPath() string       { return filepath.Join(dataDir(), "apo-installed-by-YetAnotherVolumeBooster") }
func apoDir() string           { return filepath.Join(programFiles(), "EqualizerAPO") }
func apoConfigDir() string     { return filepath.Join(apoDir(), "config") }
func managedConfigDir() string { return filepath.Join(apoConfigDir(), "YetAnotherVolumeBooster") }
func gainPath() string         { return filepath.Join(managedConfigDir(), "gain.txt") }
func apoConfig() string        { return filepath.Join(apoConfigDir(), "config.txt") }
func deviceSelector() string   { return filepath.Join(apoDir(), "DeviceSelector.exe") }

func legacyGainPaths() []string {
	return []string{
		filepath.Join(dataDir(), "YetAnotherVolumeBooster.txt"),
		filepath.Join(dataDir(), "volume-boost.txt"),
	}
}

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
	req.Header.Set("User-Agent", "YetAnotherVolumeBoosterSetup/1.0 (+Windows)")
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
	_ = os.Remove(filepath.Join(installDir(), "Volume"+"BoostSetup.exe"))
	if err := writeFileAtomic(filepath.Join(installDir(), "GPL-2.0.txt"), gplPayload, 0644); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(installDir(), "OFL-PlayfairDisplay.txt"), playfairLicensePayload, 0644); err != nil {
		return err
	}
	thirdParty := []byte("Equalizer APO 1.4.2\r\nOfficial project: https://sourceforge.net/projects/equalizerapo/\r\nSource: https://sourceforge.net/p/equalizerapo/code/\r\nLicense: GNU GPL v2\r\n\r\nPlayfair Display\r\nOfficial project: https://fonts.google.com/specimen/Playfair+Display\r\nSource: https://github.com/google/fonts/tree/main/ofl/playfairdisplay\r\nLicense: SIL Open Font License 1.1 (see OFL-PlayfairDisplay.txt)\r\n")
	return writeFileAtomic(filepath.Join(installDir(), "THIRD-PARTY-NOTICES.txt"), thirdParty, 0644)
}

func ensureGainFile() error {
	if _, err := os.Stat(gainPath()); err == nil {
		return nil
	}

	content := []byte("# Managed by YetAnotherVolumeBooster\r\n# 100% = +0.00 dB\r\nPreamp: 0.00 dB\r\n")
	for _, legacyPath := range legacyGainPaths() {
		if legacy, err := os.ReadFile(legacyPath); err == nil {
			if regexp.MustCompile(`(?i)Preamp:\s*[+-]?[0-9]+(?:\.[0-9]+)?\s*dB`).Match(legacy) {
				setupLog("migrating legacy gain file: from=%s to=%s", legacyPath, gainPath())
				content = legacy
			}
			break
		}
	}
	if err := writeFileAtomic(gainPath(), content, 0644); err != nil {
		return err
	}
	// The old file lived outside Equalizer APO's watched tree and is no longer used.
	for _, legacyPath := range legacyGainPaths() {
		_ = os.Remove(legacyPath)
	}
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
			if unattendedSetup() {
				setupLog("Equalizer APO repair notice suppressed for unattended setup")
			} else {
				messageBox("Equalizer APO's device selector is incomplete or its Qt platform plugin is missing.\n\nYetAnotherVolumeBooster will repair it using the verified official Equalizer APO 1.4.2 installer.", appName+" Setup", MB_OK|MB_ICONINFORMATION)
			}
		} else {
			if unattendedSetup() {
				setupLog("Equalizer APO install notice suppressed for unattended setup")
			} else {
				messageBox("YetAnotherVolumeBooster will now install the official Equalizer APO 1.4.2 audio engine.\n\nAfter installation, select the speakers, headphones, or Bluetooth output you actually use.", appName+" Setup", MB_OK|MB_ICONINFORMATION)
			}
		}

		tempDir, err := os.MkdirTemp("", "YetAnotherVolumeBoosterSetup-")
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

	openedFromOnboarding := false
	if repair || apoInstalledOrRepaired || apoWasPresent {
		if unattendedSetup() {
			setupLog("unattended setup: skipping audio onboarding so YetAnotherVolumeBooster can open immediately")
		} else {
			audioStatus := "Equalizer APO configuration was updated."
			if err := restartAudioService(); err != nil {
				setupLog("automatic audio-service restart failed; a Windows restart is required: %v", err)
				audioStatus = "Windows Audio could not be restarted automatically. Restart Windows once if boost does not affect the selected output."
			} else {
				audioStatus = "Windows Audio was restarted so the updated Equalizer APO config can take effect."
			}
			openedFromOnboarding = showAudioOnboarding(audioStatus)
		}
	}
	if launchApp && !openedFromOnboarding {
		setupLog("launching controller independently: %s", appPath())
		if err := launchControllerDetached(appPath()); err != nil {
			return fmt.Errorf("launch YetAnotherVolumeBooster: %w", err)
		}
	} else if openedFromOnboarding {
		setupLog("controller launch already requested from audio onboarding")
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
	if !unattendedSetup() {
		if !showUninstallFeedback() {
			return nil
		}
	}
	if err := removeIntegration(); err != nil {
		return err
	}
	_ = os.RemoveAll(managedConfigDir())
	removeShortcuts()
	removeUninstallEntry()
	_ = runHidden("reg.exe", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", appName, "/f")
	_ = os.RemoveAll(dataDir())
	waitForUninstallFeedback(3 * time.Second)

	// Delete the installation directory after this executable exits.
	cmdLine := fmt.Sprintf(`timeout /t 2 /nobreak >nul & rmdir /s /q "%s"`, installDir())
	cmd := exec.Command("cmd.exe", "/C", cmdLine)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
	if unattendedSetup() {
		setupLog("uninstall success message suppressed for unattended setup")
	} else {
		messageBox("YetAnotherVolumeBooster was removed.\n\nEqualizer APO remains installed and can be removed separately from Windows Settings if you no longer use it.", "YetAnotherVolumeBooster", MB_OK|MB_ICONINFORMATION)
	}
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
		setupLog("mode: audio onboarding helper")
		if err := grantDataPermissions(); err != nil {
			setupLog("audio onboarding data permission failed: %v", err)
			messageBox("Audio setup could not update app permissions:\n\n"+err.Error()+"\n\nDiagnostic log:\n"+setupLogLocation(), appName, MB_OK|MB_ICONERROR)
			return
		}
		if err := ensureGainFile(); err != nil {
			setupLog("audio onboarding gain file failed: %v", err)
			messageBox("Audio setup could not update the gain file:\n\n"+err.Error()+"\n\nDiagnostic log:\n"+setupLogLocation(), appName, MB_OK|MB_ICONERROR)
			return
		}
		if err := grantManagedConfigPermissions(); err != nil {
			setupLog("audio onboarding config permission failed: %v", err)
			messageBox("Audio setup could not update Equalizer APO permissions:\n\n"+err.Error()+"\n\nDiagnostic log:\n"+setupLogLocation(), appName, MB_OK|MB_ICONERROR)
			return
		}
		if err := integrateConfig(); err != nil {
			setupLog("audio onboarding integration failed: %v", err)
			messageBox("Audio setup could not update Equalizer APO:\n\n"+err.Error()+"\n\nDiagnostic log:\n"+setupLogLocation(), appName, MB_OK|MB_ICONERROR)
			return
		}
		status := "Equalizer APO configuration was updated."
		if err := restartAudioService(); err != nil {
			setupLog("audio onboarding audio-service restart failed: %v", err)
			status = "Windows Audio could not be restarted automatically. Restart Windows once if boost does not affect the selected output."
		} else {
			status = "Windows Audio was restarted so the updated Equalizer APO config can take effect."
		}
		if unattendedSetup() {
			setupLog("unattended audio onboarding helper: UI suppressed")
			return
		}
		showAudioOnboarding(status)
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
	setupLog("mode: repair=%t launchApp=%t uninstall=%t unattended=%t", repair, launchApp, hasArg("--uninstall"), unattendedSetup())
	if !repair && !unattendedSetup() {
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

	setupLog("setup completed successfully; success message suppressed")
}
