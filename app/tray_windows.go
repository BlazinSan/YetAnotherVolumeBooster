//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifShowTip = 0x00000080

	mfString    = 0x00000000
	mfSeparator = 0x00000800
	mfChecked   = 0x00000008
	mfDefault   = 0x00001000

	tpmRightButton = 0x0002
	tpmNonotify    = 0x0080
	tpmReturnCmd   = 0x0100

	wmNull        = 0x0000
	wmContextMenu = 0x007B

	trayCommandShow    = 3001
	trayCommand100     = 3100
	trayCommand200     = 3200
	trayCommand300     = 3300
	trayCommand400     = 3400
	trayCommand500     = 3500
	trayCommandStartup = 3600
	trayCommandClose   = 3601
	trayCommandExit    = 3999
)

type notifyIconData struct {
	cbSize           uint32
	hWnd             syscall.Handle
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            syscall.Handle
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         guid
	hBalloonIcon     syscall.Handle
}

var (
	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	procAppendMenuW      = user32.NewProc("AppendMenuW")
	procTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	procDestroyMenu      = user32.NewProc("DestroyMenu")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procPostMessageW     = user32.NewProc("PostMessageW")

	trayAdded      bool
	trayIdleIcon   syscall.Handle
	trayFrames     [8]syscall.Handle
	trayFrameIndex = -1
)

func copyUTF16(dst []uint16, text string) {
	encoded := syscall.StringToUTF16(text)
	if len(encoded) > len(dst) {
		encoded = encoded[:len(dst)]
		encoded[len(encoded)-1] = 0
	}
	copy(dst, encoded)
}

func initTrayIcons() {
	if trayIdleIcon == 0 {
		trayIdleIcon = createVolumeIcon(32, 0, true)
	}
	for i := range trayFrames {
		if trayFrames[i] == 0 {
			trayFrames[i] = createVolumeIcon(32, i, false)
		}
	}
}

func trayTooltip() string {
	return fmt.Sprintf("YetAnotherVolumeBooster · %d%% · %+.2f dB", currentPct, percentToDB(currentPct))
}

func trayData(icon syscall.Handle) notifyIconData {
	data := notifyIconData{
		cbSize:           uint32(unsafe.Sizeof(notifyIconData{})),
		hWnd:             hwndMain,
		uID:              1,
		uFlags:           nifMessage | nifIcon | nifTip | nifShowTip,
		uCallbackMessage: wmTrayIcon,
		hIcon:            icon,
	}
	copyUTF16(data.szTip[:], trayTooltip())
	return data
}

func addTrayIcon() {
	initTrayIcons()
	icon := trayIdleIcon
	if currentPct > 100 {
		icon = trayFrames[0]
	}
	data := trayData(icon)
	r, _, callErr := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&data)))
	trayAdded = r != 0
	if !trayAdded {
		logEvent("tray icon add failed: %v", callErr)
	} else {
		logEvent("tray icon added")
	}
}

func modifyTrayIcon(icon syscall.Handle) {
	if !trayAdded || icon == 0 {
		return
	}
	data := trayData(icon)
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&data)))
}

func updateTrayState() {
	trayFrameIndex = -1
	if currentPct <= 100 {
		modifyTrayIcon(trayIdleIcon)
	} else {
		modifyTrayIcon(trayFrames[animationPhase%len(trayFrames)])
	}
}

func updateTrayAnimation(phase int) {
	if !trayAdded {
		return
	}
	if currentPct <= 100 {
		if trayFrameIndex != -2 {
			trayFrameIndex = -2
			modifyTrayIcon(trayIdleIcon)
		}
		return
	}
	index := (phase / 6) % len(trayFrames)
	if index == trayFrameIndex {
		return
	}
	trayFrameIndex = index
	modifyTrayIcon(trayFrames[index])
}

func removeTrayIcon() {
	if !trayAdded {
		return
	}
	data := trayData(0)
	data.uFlags = 0
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&data)))
	trayAdded = false
	logEvent("tray icon removed")
}

func destroyTrayIcons() {
	if trayIdleIcon != 0 {
		procDestroyIcon.Call(uintptr(trayIdleIcon))
		trayIdleIcon = 0
	}
	for i, icon := range trayFrames {
		if icon != 0 {
			procDestroyIcon.Call(uintptr(icon))
			trayFrames[i] = 0
		}
	}
}

func appendMenu(menu uintptr, flags uintptr, id uintptr, text string) {
	var textPtr uintptr
	if flags&mfSeparator == 0 {
		textPtr = uintptr(unsafe.Pointer(utf16(text)))
	}
	procAppendMenuW.Call(menu, flags, id, textPtr)
}

func showTrayMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	appendMenu(menu, mfString|mfDefault, trayCommandShow, "Open YetAnotherVolumeBooster")
	appendMenu(menu, mfSeparator, 0, "")
	for _, item := range []struct {
		id  uintptr
		pct int
	}{
		{trayCommand100, 100}, {trayCommand200, 200}, {trayCommand300, 300}, {trayCommand400, 400}, {trayCommand500, 500},
	} {
		flags := uintptr(mfString)
		if currentPct == item.pct {
			flags |= mfChecked
		}
		appendMenu(menu, flags, item.id, fmt.Sprintf("%d%%", item.pct))
	}
	appendMenu(menu, mfSeparator, 0, "")
	startupFlags := uintptr(mfString)
	if settings.StartWithWindows {
		startupFlags |= mfChecked
	}
	appendMenu(menu, startupFlags, trayCommandStartup, "Start with Windows")
	closeFlags := uintptr(mfString)
	if settings.CloseToTray {
		closeFlags |= mfChecked
	}
	appendMenu(menu, closeFlags, trayCommandClose, "Close button minimizes to tray")
	appendMenu(menu, mfSeparator, 0, "")
	appendMenu(menu, mfString, trayCommandExit, "Exit")

	var cursor point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	procSetForegroundWindow.Call(uintptr(hwndMain))
	command, _, _ := procTrackPopupMenu.Call(menu, tpmRightButton|tpmNonotify|tpmReturnCmd, uintptr(cursor.x), uintptr(cursor.y), 0, uintptr(hwndMain), 0)
	if command != 0 {
		handleTrayCommand(int(command))
	}
	procPostMessageW.Call(uintptr(hwndMain), wmNull, 0, 0)
}

func handleTrayMessage(lParam uintptr) {
	event := uint32(lParam & 0xffff)
	switch event {
	case wmLButtonUp, wmLButtonDblClk:
		showMainWindow()
	case wmRButtonUp, wmContextMenu:
		showTrayMenu()
	}
}

func handleTrayCommand(command int) {
	switch command {
	case trayCommandShow:
		showMainWindow()
	case trayCommand100:
		applyPercent(100, true, true)
	case trayCommand200:
		applyPercent(200, true, true)
	case trayCommand300:
		applyPercent(300, true, true)
	case trayCommand400:
		applyPercent(400, true, true)
	case trayCommand500:
		applyPercent(500, true, true)
	case trayCommandStartup:
		toggleStartup()
	case trayCommandClose:
		toggleCloseToTray()
	case trayCommandExit:
		requestExit()
	}
}
