//go:build windows

package main

import (
	"fmt"
	"math"
	"syscall"
	"time"
	"unsafe"
)

const (
	logicalClientWidth  = 760
	logicalClientHeight = 700
	sliderKnobInset     = 16

	STD_ERROR_HANDLE = ^uint32(11)

	wsOverlapped   = 0x00000000
	wsPopup        = 0x80000000
	wsCaption      = 0x00C00000
	wsSysMenu      = 0x00080000
	wsThickFrame   = 0x00040000
	wsMinimizeBox  = 0x00020000
	wsMaximizeBox  = 0x00010000
	wsClipChildren = 0x02000000
	wsExAppWindow  = 0x00040000

	csVRedraw = 0x0001
	csHRedraw = 0x0002
	csDblClks = 0x0008

	cwUseDefault = 0x80000000

	swHide     = 0
	swMaximize = 3
	swShow     = 5
	swMinimize = 6
	swRestore  = 9

	wmCreate        = 0x0001
	wmDestroy       = 0x0002
	wmGetMinMaxInfo = 0x0024
	wmSize          = 0x0005
	wmPaint         = 0x000F
	wmClose         = 0x0010
	wmEraseBkgnd    = 0x0014
	wmNcCalcSize    = 0x0083
	wmNcHitTest     = 0x0084
	wmNcRButtonUp   = 0x00A5
	wmCommand       = 0x0111
	wmTimer         = 0x0113
	wmSetIcon       = 0x0080
	wmMouseMove     = 0x0200
	wmLButtonDown   = 0x0201
	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205
	wmMouseLeave    = 0x02A3
	wmDpiChanged    = 0x02E0
	wmApp           = 0x8000

	wmTrayIcon = wmApp + 1

	sizeMinimized = 1

	iconSmall = 0
	iconBig   = 1

	mbOK              = 0x00000000
	mbIconError       = 0x00000010
	mbIconInformation = 0x00000040

	idcArrow = 32512

	transparent   = 1
	dtLeft        = 0x0000
	dtCenter      = 0x0001
	dtRight       = 0x0002
	dtVCenter     = 0x0004
	dtSingleLine  = 0x0020
	dtEndEllipsis = 0x8000

	psSolid = 0
	srccopy = 0x00CC0020

	gradientFillRectH = 0x00000000

	tmeLeave = 0x00000002

	animationTimerID = 1

	titleBarHeight       = 34
	titleButtonSize      = 16
	titleButtonSpacing   = 5
	titleButtonRight     = 20
	titleButtonTop       = 8
	titleThemeButtonSize = 28
	titleThemeGap        = 12
	resizeBorder         = 8

	htClient      = 1
	htCaption     = 2
	htLeft        = 10
	htRight       = 11
	htTop         = 12
	htTopLeft     = 13
	htTopRight    = 14
	htBottom      = 15
	htBottomLeft  = 16
	htBottomRight = 17

	moveFileReplaceExisting = 0x1
	moveFileWriteThrough    = 0x8

	uiNone        = 0
	uiSlider      = 1
	uiPreset100   = 10
	uiPreset200   = 11
	uiPreset300   = 12
	uiPreset400   = 13
	uiPreset500   = 14
	uiDevice      = 20
	uiRepair      = 21
	uiStartup     = 30
	uiCloseToTray = 31
	uiLogs        = 40
	uiTheme       = 41
	uiTitleClose  = 50
	uiTitleMax    = 51
	uiTitleMin    = 52
)

type statusKind int

const (
	toneReady statusKind = iota
	toneActive
	toneWarning
	toneError
)

type wndClassEx struct {
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

type point struct{ x, y int32 }

type msg struct {
	hwnd    syscall.Handle
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
	private uint32
}

type rect struct {
	left, top, right, bottom int32
}

type paintStruct struct {
	hdc         syscall.Handle
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type trackMouseEvent struct {
	cbSize      uint32
	dwFlags     uint32
	hwndTrack   syscall.Handle
	dwHoverTime uint32
}

type triVertex struct {
	x, y             int32
	red, green, blue uint16
	alpha            uint16
}

type gradientRect struct{ upperLeft, lowerRight uint32 }

type minMaxInfo struct {
	ptReserved     point
	ptMaxSize      point
	ptMaxPosition  point
	ptMinTrackSize point
	ptMaxTrackSize point
}

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	msimg32  = syscall.NewLazyDLL("msimg32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	procRegisterClassExW              = user32.NewProc("RegisterClassExW")
	procCreateWindowExW               = user32.NewProc("CreateWindowExW")
	procDefWindowProcW                = user32.NewProc("DefWindowProcW")
	procShowWindow                    = user32.NewProc("ShowWindow")
	procUpdateWindow                  = user32.NewProc("UpdateWindow")
	procGetMessageW                   = user32.NewProc("GetMessageW")
	procTranslateMessage              = user32.NewProc("TranslateMessage")
	procDispatchMessageW              = user32.NewProc("DispatchMessageW")
	procPostQuitMessage               = user32.NewProc("PostQuitMessage")
	procDestroyWindow                 = user32.NewProc("DestroyWindow")
	procLoadCursorW                   = user32.NewProc("LoadCursorW")
	procGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	procMessageBoxW                   = user32.NewProc("MessageBoxW")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetDpiForSystem               = user32.NewProc("GetDpiForSystem")
	procGetDpiForWindow               = user32.NewProc("GetDpiForWindow")
	procAdjustWindowRectEx            = user32.NewProc("AdjustWindowRectEx")
	procBeginPaint                    = user32.NewProc("BeginPaint")
	procEndPaint                      = user32.NewProc("EndPaint")
	procGetClientRect                 = user32.NewProc("GetClientRect")
	procInvalidateRect                = user32.NewProc("InvalidateRect")
	procSetTimer                      = user32.NewProc("SetTimer")
	procKillTimer                     = user32.NewProc("KillTimer")
	procTrackMouseEvent               = user32.NewProc("TrackMouseEvent")
	procSetCapture                    = user32.NewProc("SetCapture")
	procReleaseCapture                = user32.NewProc("ReleaseCapture")
	procSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	procScreenToClient                = user32.NewProc("ScreenToClient")
	procIsWindowVisible               = user32.NewProc("IsWindowVisible")
	procSetWindowPos                  = user32.NewProc("SetWindowPos")
	procSendMessageW                  = user32.NewProc("SendMessageW")
	procIsZoomed                      = user32.NewProc("IsZoomed")
	procLoadImageW                    = user32.NewProc("LoadImageW")
	procDestroyIcon                   = user32.NewProc("DestroyIcon")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	procSetStdHandle     = kernel32.NewProc("SetStdHandle")
	procRtlMoveMemory    = kernel32.NewProc("RtlMoveMemory")
	procMoveFileExW      = kernel32.NewProc("MoveFileExW")

	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	procCreatePen              = gdi32.NewProc("CreatePen")
	procRoundRect              = gdi32.NewProc("RoundRect")
	procRectangle              = gdi32.NewProc("Rectangle")
	procEllipse                = gdi32.NewProc("Ellipse")
	procMoveToEx               = gdi32.NewProc("MoveToEx")
	procLineTo                 = gdi32.NewProc("LineTo")
	procSetBkMode              = gdi32.NewProc("SetBkMode")
	procSetTextColor           = gdi32.NewProc("SetTextColor")
	procCreateFontW            = gdi32.NewProc("CreateFontW")
	procGetStockObject         = gdi32.NewProc("GetStockObject")
	procCreateRoundRectRgn     = gdi32.NewProc("CreateRoundRectRgn")
	procCreateEllipticRgn      = gdi32.NewProc("CreateEllipticRgn")
	procSelectClipRgn          = gdi32.NewProc("SelectClipRgn")
	procFillRect               = user32.NewProc("FillRect")

	procDrawTextW             = user32.NewProc("DrawTextW")
	procGradientFill          = msimg32.NewProc("GradientFill")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	procShellExecuteW         = shell32.NewProc("ShellExecuteW")
)

var (
	dpiScale        = 1.0
	wndProcCallback uintptr

	fontTitle  syscall.Handle
	fontValue  syscall.Handle
	fontDB     syscall.Handle
	fontButton syscall.Handle
	fontBody   syscall.Handle
	fontSmall  syscall.Handle
	fontStatus syscall.Handle

	hoverElement   = uiNone
	pressedElement = uiNone
	mouseTracked   bool
	animationPhase int
	dragAppliedPct int
	dragAppliedAt  time.Time

	startupToggleVisual  float64
	closeToggleVisual    float64
	titleMinVisual       float64
	titleMaxVisual       float64
	titleCloseVisual     float64
	themeTransition      bool
	themeTransitionFrom  bool
	themeTransitionTo    bool
	themeTransitionStart time.Time
)

func rgb(r, g, b uint8) uintptr { return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16 }

func colorComponents(color uintptr) (uint8, uint8, uint8) {
	return uint8(color & 0xff), uint8((color >> 8) & 0xff), uint8((color >> 16) & 0xff)
}

type uiPalette struct {
	background, card, cardSoft, text, muted, border, shadow uintptr
	accentDark, accent, accentLight, active, warning, error uintptr
	warningBG, warningBorder, warningText                   uintptr
}

func lightPalette() uiPalette {
	// Mirrors EverythingUTM's emerald palette and color-mix-derived light surfaces.
	return uiPalette{
		background:    rgb(228, 237, 232),
		card:          rgb(248, 251, 250),
		cardSoft:      rgb(238, 246, 243),
		text:          rgb(18, 32, 28),
		muted:         rgb(90, 106, 108),
		border:        rgb(179, 198, 183),
		shadow:        rgb(213, 225, 219),
		accentDark:    rgb(10, 92, 62),
		accent:        rgb(15, 122, 82),
		accentLight:   rgb(31, 169, 120),
		active:        rgb(15, 122, 82),
		warning:       rgb(177, 119, 15),
		error:         rgb(196, 62, 62),
		warningBG:     rgb(255, 247, 211),
		warningBorder: rgb(235, 213, 131),
		warningText:   rgb(101, 82, 30),
	}
}

func darkPalette() uiPalette {
	// --bg: color-mix(in srgb, #189a6c 10%, #0d0f14).
	return uiPalette{
		background:    rgb(14, 29, 29),
		card:          rgb(22, 38, 41),
		cardSoft:      rgb(31, 51, 55),
		text:          rgb(240, 248, 245),
		muted:         rgb(174, 191, 194),
		border:        rgb(46, 78, 80),
		shadow:        rgb(12, 24, 25),
		accentDark:    rgb(14, 107, 73),
		accent:        rgb(24, 154, 108),
		accentLight:   rgb(90, 209, 168),
		active:        rgb(90, 209, 168),
		warning:       rgb(232, 181, 70),
		error:         rgb(240, 111, 111),
		warningBG:     rgb(57, 48, 24),
		warningBorder: rgb(114, 91, 31),
		warningText:   rgb(250, 224, 154),
	}
}

var palette = lightPalette()

func setPalette(dark bool) {
	if dark {
		palette = darkPalette()
	} else {
		palette = lightPalette()
	}
}

func mixColor(a, b uintptr, t float64) uintptr {
	ar, ag, ab := colorComponents(a)
	br, bg, bb := colorComponents(b)
	return rgb(
		uint8(math.Round(float64(ar)*(1-t)+float64(br)*t)),
		uint8(math.Round(float64(ag)*(1-t)+float64(bg)*t)),
		uint8(math.Round(float64(ab)*(1-t)+float64(bb)*t)),
	)
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func logicalRect(l, t, r, b int32) rect { return rect{l, t, r, b} }

var (
	sliderRect  = logicalRect(78, 194, 682, 232)
	presetRects = []rect{
		logicalRect(80, 276, 176, 334),
		logicalRect(196, 276, 292, 334),
		logicalRect(312, 276, 408, 334),
		logicalRect(428, 276, 524, 334),
		logicalRect(544, 276, 640, 334),
	}
	deviceRect     = logicalRect(140, 382, 360, 428)
	repairRect     = logicalRect(400, 382, 620, 428)
	startupRect    = logicalRect(80, 454, 680, 510)
	closeTrayRect  = logicalRect(80, 522, 680, 578)
	statusCardRect = logicalRect(80, 604, 680, 650)
	logsRect       = logicalRect(598, 612, 660, 642)
	warningRect    = logicalRect(195, 666, 565, 694)
)

func scaleInt(value int32) int32 { return int32(math.Round(float64(value) * dpiScale)) }

func clientLogicalOrigin() int32 {
	if hwndMain == 0 {
		return 0
	}
	var rc rect
	procGetClientRect.Call(uintptr(hwndMain), uintptr(unsafe.Pointer(&rc)))
	logicalWidth := float64(rc.right-rc.left) / dpiScale
	if logicalWidth <= logicalClientWidth {
		return 0
	}
	return int32(math.Round((logicalWidth - logicalClientWidth) / 2))
}

func scaledRect(source rect) rect {
	ox := clientLogicalOrigin()
	return rect{
		left:   scaleInt(source.left + ox),
		top:    scaleInt(source.top),
		right:  scaleInt(source.right + ox),
		bottom: scaleInt(source.bottom),
	}
}

func scaledRawRect(source rect) rect {
	return rect{
		left:   scaleInt(source.left),
		top:    scaleInt(source.top),
		right:  scaleInt(source.right),
		bottom: scaleInt(source.bottom),
	}
}

func unscalePoint(x, y int32) point {
	ox := clientLogicalOrigin()
	return point{
		x: int32(math.Round(float64(x)/dpiScale)) - ox,
		y: int32(math.Round(float64(y) / dpiScale)),
	}
}

func unscaleRawPoint(x, y int32) point {
	return point{
		x: int32(math.Round(float64(x) / dpiScale)),
		y: int32(math.Round(float64(y) / dpiScale)),
	}
}

func pointInRect(p point, r rect) bool {
	return p.x >= r.left && p.x < r.right && p.y >= r.top && p.y < r.bottom
}

func rawLogicalClientWidth() int32 {
	width, _ := rawLogicalClientSize()
	return width
}

func rawLogicalClientSize() (int32, int32) {
	if hwndMain == 0 {
		return logicalClientWidth, logicalClientHeight
	}
	var rc rect
	procGetClientRect.Call(uintptr(hwndMain), uintptr(unsafe.Pointer(&rc)))
	width := int32(math.Round(float64(rc.right-rc.left) / dpiScale))
	height := int32(math.Round(float64(rc.bottom-rc.top) / dpiScale))
	if width < logicalClientWidth {
		width = logicalClientWidth
	}
	if height < logicalClientHeight {
		height = logicalClientHeight
	}
	return width, height
}

type titleButtonSpec struct {
	id         int
	target     rect
	hoverColor uintptr
	progress   float64
}

func titleButtonRects() []titleButtonSpec {
	right := rawLogicalClientWidth() - titleButtonRight
	closeRect := logicalRect(right-titleButtonSize, titleButtonTop, right, titleButtonTop+titleButtonSize)
	right -= titleButtonSize + titleButtonSpacing
	maxRect := logicalRect(right-titleButtonSize, titleButtonTop, right, titleButtonTop+titleButtonSize)
	right -= titleButtonSize + titleButtonSpacing
	minRect := logicalRect(right-titleButtonSize, titleButtonTop, right, titleButtonTop+titleButtonSize)
	return []titleButtonSpec{
		{id: uiTitleClose, target: closeRect, hoverColor: rgb(191, 25, 25), progress: titleCloseVisual},
		{id: uiTitleMax, target: maxRect, hoverColor: rgb(29, 196, 29), progress: titleMaxVisual},
		{id: uiTitleMin, target: minRect, hoverColor: rgb(212, 212, 38), progress: titleMinVisual},
	}
}

func titleThemeRect() rect {
	buttons := titleButtonRects()
	leftOfControls := rawLogicalClientWidth() - titleButtonRight
	if len(buttons) > 0 {
		leftOfControls = buttons[len(buttons)-1].target.left
	}
	right := leftOfControls - titleThemeGap
	top := int32((titleBarHeight - titleThemeButtonSize) / 2)
	return logicalRect(right-titleThemeButtonSize, top, right, top+titleThemeButtonSize)
}

func titleHitTest(p point) int {
	if pointInRect(p, titleThemeRect()) {
		return uiTheme
	}
	for _, button := range titleButtonRects() {
		cx := (button.target.left + button.target.right) / 2
		cy := (button.target.top + button.target.bottom) / 2
		radius := (button.target.right - button.target.left) / 2
		dx := p.x - cx
		dy := p.y - cy
		if dx*dx+dy*dy <= radius*radius {
			return button.id
		}
	}
	return uiNone
}

func resizeHitTest(p point) uintptr {
	width, height := rawLogicalClientSize()
	left := p.x >= 0 && p.x < resizeBorder
	right := p.x >= width-resizeBorder && p.x < width
	top := p.y >= 0 && p.y < resizeBorder
	bottom := p.y >= height-resizeBorder && p.y < height
	switch {
	case top && left:
		return htTopLeft
	case top && right:
		return htTopRight
	case bottom && left:
		return htBottomLeft
	case bottom && right:
		return htBottomRight
	case left:
		return htLeft
	case right:
		return htRight
	case top:
		return htTop
	case bottom:
		return htBottom
	default:
		return 0
	}
}

func lowordSigned(v uintptr) int32 { return int32(int16(uint16(v & 0xffff))) }
func hiwordSigned(v uintptr) int32 { return int32(int16(uint16((v >> 16) & 0xffff))) }
func loword(v uintptr) int         { return int(v & 0xffff) }

func messageBox(text, title string, flags uintptr) {
	procMessageBoxW.Call(uintptr(hwndMain), uintptr(unsafe.Pointer(utf16(text))), uintptr(unsafe.Pointer(utf16(title))), flags)
}

func invalidateWindow() {
	if hwndMain != 0 {
		procInvalidateRect.Call(uintptr(hwndMain), 0, 0)
	}
}

func createFont(logicalSize int32, weight int32, face string) syscall.Handle {
	height := -scaleInt(logicalSize)
	h, _, _ := procCreateFontW.Call(
		uintptr(height), 0, 0, 0,
		uintptr(weight), 0, 0, 0,
		1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(utf16(face))),
	)
	return syscall.Handle(h)
}

func destroyFonts() {
	for _, font := range []syscall.Handle{fontTitle, fontValue, fontDB, fontButton, fontBody, fontSmall, fontStatus} {
		if font != 0 {
			procDeleteObject.Call(uintptr(font))
		}
	}
	fontTitle, fontValue, fontDB, fontButton, fontBody, fontSmall, fontStatus = 0, 0, 0, 0, 0, 0, 0
}

func createFonts() {
	destroyFonts()
	fontTitle = createFont(27, 600, "Segoe UI Variable Display")
	fontValue = createFont(78, 400, "Segoe UI Variable Display")
	fontDB = createFont(16, 500, "Segoe UI Variable Text")
	fontButton = createFont(15, 600, "Segoe UI Variable Text")
	fontBody = createFont(15, 500, "Segoe UI Variable Text")
	fontSmall = createFont(12, 400, "Segoe UI Variable Text")
	fontStatus = createFont(13, 500, "Segoe UI Variable Text")
}

func drawText(hdc syscall.Handle, text string, target rect, font syscall.Handle, color uintptr, flags uintptr) {
	old, _, _ := procSelectObject.Call(uintptr(hdc), uintptr(font))
	procSetBkMode.Call(uintptr(hdc), transparent)
	procSetTextColor.Call(uintptr(hdc), color)
	procDrawTextW.Call(
		uintptr(hdc),
		uintptr(unsafe.Pointer(utf16(text))),
		uintptr(^uint32(0)),
		uintptr(unsafe.Pointer(&target)),
		flags,
	)
	procSelectObject.Call(uintptr(hdc), old)
}

func fillRoundRect(hdc syscall.Handle, target rect, radius int32, fill uintptr) {
	if gdipFillRoundedRect(target, scaleInt(radius), fill) {
		return
	}
	brush, _, _ := procCreateSolidBrush.Call(fill)
	pen, _, _ := procCreatePen.Call(psSolid, 1, fill)
	oldBrush, _, _ := procSelectObject.Call(uintptr(hdc), brush)
	oldPen, _, _ := procSelectObject.Call(uintptr(hdc), pen)
	procRoundRect.Call(uintptr(hdc), uintptr(target.left), uintptr(target.top), uintptr(target.right), uintptr(target.bottom), uintptr(scaleInt(radius)), uintptr(scaleInt(radius)))
	procSelectObject.Call(uintptr(hdc), oldBrush)
	procSelectObject.Call(uintptr(hdc), oldPen)
	procDeleteObject.Call(brush)
	procDeleteObject.Call(pen)
}

func strokeRoundRect(hdc syscall.Handle, target rect, radius int32, color uintptr, width int32) {
	if gdipStrokeRoundedRect(target, scaleInt(radius), color, scaleInt(width)) {
		return
	}
	pen, _, _ := procCreatePen.Call(psSolid, uintptr(scaleInt(width)), color)
	nullBrush, _, _ := procGetStockObject.Call(5) // NULL_BRUSH
	oldBrush, _, _ := procSelectObject.Call(uintptr(hdc), nullBrush)
	oldPen, _, _ := procSelectObject.Call(uintptr(hdc), pen)
	procRoundRect.Call(uintptr(hdc), uintptr(target.left), uintptr(target.top), uintptr(target.right), uintptr(target.bottom), uintptr(scaleInt(radius)), uintptr(scaleInt(radius)))
	procSelectObject.Call(uintptr(hdc), oldBrush)
	procSelectObject.Call(uintptr(hdc), oldPen)
	procDeleteObject.Call(pen)
}

func fillStrokeRoundRect(hdc syscall.Handle, target rect, radius int32, fill, border uintptr, width int32) {
	if width <= 0 {
		fillRoundRect(hdc, target, radius, fill)
		return
	}
	fillRoundRect(hdc, target, radius, border)
	inset := scaleInt(width)
	inner := rect{
		left:   target.left + inset,
		top:    target.top + inset,
		right:  target.right - inset,
		bottom: target.bottom - inset,
	}
	if inner.right <= inner.left || inner.bottom <= inner.top {
		return
	}
	innerRadius := radius - width
	if innerRadius < 1 {
		innerRadius = 1
	}
	fillRoundRect(hdc, inner, innerRadius, fill)
}

func fillGradientRoundRect(hdc syscall.Handle, target rect, radius int32, leftColor, rightColor uintptr) {
	if gdipGradientRoundedRect(target, scaleInt(radius), leftColor, rightColor) {
		return
	}
	rgn, _, _ := procCreateRoundRectRgn.Call(uintptr(target.left), uintptr(target.top), uintptr(target.right+1), uintptr(target.bottom+1), uintptr(scaleInt(radius)), uintptr(scaleInt(radius)))
	procSelectClipRgn.Call(uintptr(hdc), rgn)
	lr, lg, lb := colorComponents(leftColor)
	rr, rg, rb := colorComponents(rightColor)
	vertices := [2]triVertex{
		{x: target.left, y: target.top, red: uint16(lr) << 8, green: uint16(lg) << 8, blue: uint16(lb) << 8, alpha: 0xffff},
		{x: target.right, y: target.bottom, red: uint16(rr) << 8, green: uint16(rg) << 8, blue: uint16(rb) << 8, alpha: 0xffff},
	}
	mesh := gradientRect{upperLeft: 0, lowerRight: 1}
	procGradientFill.Call(uintptr(hdc), uintptr(unsafe.Pointer(&vertices[0])), 2, uintptr(unsafe.Pointer(&mesh)), 1, gradientFillRectH)
	procSelectClipRgn.Call(uintptr(hdc), 0)
	procDeleteObject.Call(rgn)
}

func drawLine(hdc syscall.Handle, x1, y1, x2, y2 int32, color uintptr, width int32) {
	pen, _, _ := procCreatePen.Call(psSolid, uintptr(scaleInt(width)), color)
	oldPen, _, _ := procSelectObject.Call(uintptr(hdc), pen)
	procMoveToEx.Call(uintptr(hdc), uintptr(scaleInt(x1+clientLogicalOrigin())), uintptr(scaleInt(y1)), 0)
	procLineTo.Call(uintptr(hdc), uintptr(scaleInt(x2+clientLogicalOrigin())), uintptr(scaleInt(y2)))
	procSelectObject.Call(uintptr(hdc), oldPen)
	procDeleteObject.Call(pen)
}

func drawRawLine(hdc syscall.Handle, x1, y1, x2, y2 int32, color uintptr, width int32) {
	pen, _, _ := procCreatePen.Call(psSolid, uintptr(scaleInt(width)), color)
	oldPen, _, _ := procSelectObject.Call(uintptr(hdc), pen)
	procMoveToEx.Call(uintptr(hdc), uintptr(scaleInt(x1)), uintptr(scaleInt(y1)), 0)
	procLineTo.Call(uintptr(hdc), uintptr(scaleInt(x2)), uintptr(scaleInt(y2)))
	procSelectObject.Call(uintptr(hdc), oldPen)
	procDeleteObject.Call(pen)
}

func drawCircle(hdc syscall.Handle, cx, cy, radius int32, fill uintptr) {
	ox := clientLogicalOrigin()
	target := rect{
		left: scaleInt(cx - radius + ox), top: scaleInt(cy - radius),
		right: scaleInt(cx + radius + ox), bottom: scaleInt(cy + radius),
	}
	if gdipFillCircle(target, fill) {
		return
	}
	brush, _, _ := procCreateSolidBrush.Call(fill)
	pen, _, _ := procCreatePen.Call(psSolid, 1, fill)
	oldBrush, _, _ := procSelectObject.Call(uintptr(hdc), brush)
	oldPen, _, _ := procSelectObject.Call(uintptr(hdc), pen)
	procEllipse.Call(uintptr(hdc), uintptr(target.left), uintptr(target.top), uintptr(target.right), uintptr(target.bottom))
	procSelectObject.Call(uintptr(hdc), oldBrush)
	procSelectObject.Call(uintptr(hdc), oldPen)
	procDeleteObject.Call(brush)
	procDeleteObject.Call(pen)
}

func drawRawCircle(hdc syscall.Handle, cx, cy, radius int32, fill uintptr) {
	target := rect{
		left: scaleInt(cx - radius), top: scaleInt(cy - radius),
		right: scaleInt(cx + radius), bottom: scaleInt(cy + radius),
	}
	if gdipFillCircle(target, fill) {
		return
	}
	brush, _, _ := procCreateSolidBrush.Call(fill)
	pen, _, _ := procCreatePen.Call(psSolid, 1, fill)
	oldBrush, _, _ := procSelectObject.Call(uintptr(hdc), brush)
	oldPen, _, _ := procSelectObject.Call(uintptr(hdc), pen)
	procEllipse.Call(uintptr(hdc), uintptr(target.left), uintptr(target.top), uintptr(target.right), uintptr(target.bottom))
	procSelectObject.Call(uintptr(hdc), oldBrush)
	procSelectObject.Call(uintptr(hdc), oldPen)
	procDeleteObject.Call(brush)
	procDeleteObject.Call(pen)
}

func drawBackground(hdc syscall.Handle, client rect) {
	brush, _, _ := procCreateSolidBrush.Call(palette.background)
	procFillRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(&client)), brush)
	procDeleteObject.Call(brush)
}

func drawSlider(hdc syscall.Handle) {
	r := scaledRect(sliderRect)
	shadow := r
	shadow.top += scaleInt(5)
	shadow.bottom += scaleInt(5)
	fillRoundRect(hdc, shadow, 22, palette.shadow)

	trackLeft := mixColor(palette.cardSoft, palette.border, 0.25)
	trackRight := mixColor(palette.cardSoft, palette.accentLight, 0.18)
	trackBorder := mixColor(palette.border, palette.card, 0.35)
	fillRoundRect(hdc, r, 22, trackBorder)
	track := r
	track.left += scaleInt(1)
	track.top += scaleInt(1)
	track.right -= scaleInt(1)
	track.bottom -= scaleInt(1)
	fillGradientRoundRect(hdc, track, 21, trackLeft, trackRight)

	position := (displayPct - 100.0) / 400.0
	if position < 0 {
		position = 0
	}
	if position > 1 {
		position = 1
	}
	fillWidth := int32(math.Round(float64(track.right-track.left) * position))
	if fillWidth > 0 {
		active := track
		active.right = track.left + fillWidth
		// Keep a rounded leading/trailing cap even while dragging near either edge.
		minWidth := scaleInt(14)
		if active.right-active.left < minWidth {
			active.right = active.left + minWidth
		}
		if active.right > track.right {
			active.right = track.right
		}
		fillGradientRoundRect(hdc, active, 21, palette.accentDark, palette.accentLight)
	}

	travelLeft := sliderRect.left + sliderKnobInset
	travelRight := sliderRect.right - sliderKnobInset
	knobXLogical := travelLeft + int32(math.Round(float64(travelRight-travelLeft)*position))
	knobYLogical := (sliderRect.top + sliderRect.bottom) / 2
	drawCircle(hdc, knobXLogical, knobYLogical+3, 14, palette.shadow)
	drawCircle(hdc, knobXLogical, knobYLogical, 13, palette.card)
	knob := scaledRect(logicalRect(knobXLogical-13, knobYLogical-13, knobXLogical+13, knobYLogical+13))
	gdipStrokeCircle(knob, mixColor(palette.border, palette.accent, 0.25), scaleInt(1))

	drawText(hdc, "100%", scaledRect(logicalRect(78, 236, 150, 255)), fontSmall, palette.muted, dtLeft|dtVCenter|dtSingleLine)
	drawText(hdc, "500%", scaledRect(logicalRect(610, 236, 682, 255)), fontSmall, palette.muted, dtRight|dtVCenter|dtSingleLine)
}

func drawPresetButton(hdc syscall.Handle, index int, percent int) {
	r := scaledRect(presetRects[index])
	active := currentPct == percent
	hovered := hoverElement == uiPreset100+index
	pressed := pressedElement == uiPreset100+index

	shadow := r
	shadow.top += scaleInt(4)
	shadow.bottom += scaleInt(4)
	fillRoundRect(hdc, shadow, 15, palette.shadow)

	fill := palette.card
	if active {
		fill = mixColor(palette.cardSoft, palette.accent, 0.10)
	} else if pressed {
		fill = mixColor(palette.cardSoft, palette.border, 0.22)
	} else if hovered {
		fill = palette.cardSoft
	}
	border := palette.border
	if active {
		border = palette.accent
	}
	fillStrokeRoundRect(hdc, r, 15, fill, border, 1)
	color := palette.text
	if active {
		color = palette.accent
	}
	drawText(hdc, fmt.Sprintf("%d%%", percent), r, fontButton, color, dtCenter|dtVCenter|dtSingleLine)
}

func drawActionIcon(hdc syscall.Handle, kind int, centerX, centerY int32, color uintptr) {
	ox := clientLogicalOrigin()
	cx := centerX + ox
	if kind == uiDevice {
		r := scaledRect(logicalRect(centerX-10, centerY-8, centerX+10, centerY+6))
		strokeRoundRect(hdc, r, 4, color, 1)
		drawLine(hdc, centerX-4, centerY+9, centerX+4, centerY+9, color, 1)
		drawLine(hdc, centerX, centerY+6, centerX, centerY+9, color, 1)
		return
	}
	pen, _, _ := procCreatePen.Call(psSolid, uintptr(scaleInt(2)), color)
	oldPen, _, _ := procSelectObject.Call(uintptr(hdc), pen)
	procMoveToEx.Call(uintptr(hdc), uintptr(scaleInt(cx-7)), uintptr(scaleInt(centerY+2)), 0)
	procLineTo.Call(uintptr(hdc), uintptr(scaleInt(cx-3)), uintptr(scaleInt(centerY+7)))
	procLineTo.Call(uintptr(hdc), uintptr(scaleInt(cx+4)), uintptr(scaleInt(centerY+6)))
	procLineTo.Call(uintptr(hdc), uintptr(scaleInt(cx+8)), uintptr(scaleInt(centerY)))
	procLineTo.Call(uintptr(hdc), uintptr(scaleInt(cx+5)), uintptr(scaleInt(centerY-6)))
	procLineTo.Call(uintptr(hdc), uintptr(scaleInt(cx-2)), uintptr(scaleInt(centerY-8)))
	procLineTo.Call(uintptr(hdc), uintptr(scaleInt(cx-7)), uintptr(scaleInt(centerY-3)))
	procSelectObject.Call(uintptr(hdc), oldPen)
	procDeleteObject.Call(pen)
}

func drawActionButton(hdc syscall.Handle, target rect, id int, label string) {
	r := scaledRect(target)
	hovered := hoverElement == id
	pressed := pressedElement == id
	fill := palette.card
	if pressed {
		fill = mixColor(palette.cardSoft, palette.border, 0.25)
	} else if hovered {
		fill = palette.cardSoft
	}
	fillStrokeRoundRect(hdc, r, 13, fill, palette.border, 1)
	iconX := target.left + 28
	iconY := (target.top + target.bottom) / 2
	drawActionIcon(hdc, id, iconX, iconY, palette.muted)
	textRect := scaledRect(logicalRect(target.left+48, target.top, target.right-14, target.bottom))
	drawText(hdc, label, textRect, fontButton, palette.text, dtCenter|dtVCenter|dtSingleLine)
}

func drawToggle(hdc syscall.Handle, centerX, centerY int32, progress float64, hovered bool) {
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	target := scaledRect(logicalRect(centerX-27, centerY-15, centerX+27, centerY+15))
	offColor := mixColor(palette.border, palette.cardSoft, 0.28)
	if hovered {
		offColor = mixColor(offColor, palette.accent, 0.12)
	}
	border := mixColor(palette.border, palette.card, 0.25)
	fillStrokeRoundRect(hdc, target, 30, offColor, border, 1)
	if progress > 0.001 {
		on := target
		inset := scaleInt(1)
		on.left += inset
		on.top += inset
		on.right -= inset
		on.bottom -= inset
		on.right = on.left + int32(math.Round(float64(on.right-on.left)*progress))
		if on.right-on.left < scaleInt(18) {
			on.right = on.left + scaleInt(18)
		}
		if on.right > target.right-inset {
			on.right = target.right - inset
		}
		fillGradientRoundRect(hdc, on, 29, palette.accentDark, palette.accentLight)
	}
	knobX := centerX - 12 + int32(math.Round(24*progress))
	drawCircle(hdc, knobX, centerY+2, 12, palette.shadow)
	drawCircle(hdc, knobX, centerY, 11, palette.card)
	knob := scaledRect(logicalRect(knobX-11, centerY-11, knobX+11, centerY+11))
	gdipStrokeCircle(knob, mixColor(palette.border, palette.accent, 0.15), scaleInt(1))
}

func drawSettingRow(hdc syscall.Handle, target rect, id int, title, subtitle string, progress float64) {
	r := scaledRect(target)
	fill := palette.card
	if hoverElement == id {
		fill = palette.cardSoft
	}
	fillStrokeRoundRect(hdc, r, 15, fill, palette.border, 1)
	drawText(hdc, title, scaledRect(logicalRect(target.left+22, target.top+9, target.right-100, target.top+31)), fontBody, palette.text, dtLeft|dtVCenter|dtSingleLine)
	drawText(hdc, subtitle, scaledRect(logicalRect(target.left+22, target.top+29, target.right-100, target.bottom-5)), fontSmall, palette.muted, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
	drawToggle(hdc, target.right-49, (target.top+target.bottom)/2, progress, hoverElement == id)
}

func toneColor(tone statusKind) uintptr {
	switch tone {
	case toneActive:
		return palette.active
	case toneWarning:
		return palette.warning
	case toneError:
		return palette.error
	default:
		return palette.accent
	}
}

func drawAnimatedStatusIcon(hdc syscall.Handle, centerX, centerY int32) {
	// Intentionally empty: the in-window visualizer was removed. The tray icon keeps its animation.
}

func drawStatusCard(hdc syscall.Handle) {
	r := scaledRect(statusCardRect)
	fillStrokeRoundRect(hdc, r, 14, palette.card, palette.border, 1)
	drawCircle(hdc, 103, 627, 5, toneColor(currentStatusTone))
	drawText(hdc, statusText, scaledRect(logicalRect(120, 604, 585, 650)), fontStatus, palette.text, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)

	logs := scaledRect(logsRect)
	fill := palette.cardSoft
	if hoverElement == uiLogs {
		fill = mixColor(palette.cardSoft, palette.accent, 0.10)
	}
	fillRoundRect(hdc, logs, 10, fill)
	drawText(hdc, "Logs", logs, fontSmall, palette.muted, dtCenter|dtVCenter|dtSingleLine)
}

func drawWarning(hdc syscall.Handle) {
	r := scaledRect(warningRect)
	fillStrokeRoundRect(hdc, r, 11, palette.warningBG, palette.warningBorder, 1)
	drawCircle(hdc, 216, 680, 7, palette.warning)
	drawText(hdc, "!", scaledRect(logicalRect(209, 673, 223, 687)), fontSmall, palette.card, dtCenter|dtVCenter|dtSingleLine)
	drawText(hdc, "Protect your hearing. 300–500% may clip or distort.", scaledRect(logicalRect(230, 666, 552, 694)), fontSmall, palette.warningText, dtCenter|dtVCenter|dtSingleLine)
}

func drawTitleButton(hdc syscall.Handle, button titleButtonSpec) {
	cx := (button.target.left + button.target.right) / 2
	cy := (button.target.top + button.target.bottom) / 2
	radius := (button.target.right - button.target.left) / 2
	color := mixColor(rgb(28, 33, 28), button.hoverColor, button.progress)
	drawRawCircle(hdc, cx, cy, radius, color)
}

func drawThemeButton(hdc syscall.Handle, dark bool) {
	target := titleThemeRect()
	r := scaledRawRect(target)
	fill := palette.card
	if hoverElement == uiTheme {
		fill = palette.cardSoft
	}
	fillStrokeRoundRect(hdc, r, 18, fill, palette.border, 1)
	cx := (target.left + target.right) / 2
	cy := (target.top + target.bottom) / 2
	if dark {
		// Crescent moon.
		drawRawCircle(hdc, cx-1, cy, 7, palette.accentLight)
		drawRawCircle(hdc, cx+3, cy-3, 6, palette.card)
		return
	}
	// Sun with simple radial rays.
	drawRawCircle(hdc, cx, cy, 5, palette.accent)
	for _, d := range [][2]int32{{0, -11}, {0, 11}, {-11, 0}, {11, 0}, {-8, -8}, {8, -8}, {-8, 8}, {8, 8}} {
		x1 := cx + d[0]*7/11
		y1 := cy + d[1]*7/11
		drawRawLine(hdc, x1, y1, cx+d[0], cy+d[1], palette.accent, 1)
	}
}

func drawTitleBar(hdc syscall.Handle, client rect, dark bool) {
	titleBar := rect{left: 0, top: 0, right: client.right, bottom: scaleInt(titleBarHeight)}
	titleColor := rgb(51, 102, 102)
	titleTextColor := rgb(0, 0, 0)
	separatorColor := rgb(200, 200, 200)
	if dark {
		titleColor = rgb(61, 117, 117)
		titleTextColor = rgb(240, 240, 240)
		separatorColor = rgb(60, 60, 60)
	}
	brush, _, _ := procCreateSolidBrush.Call(titleColor)
	procFillRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(&titleBar)), brush)
	procDeleteObject.Call(brush)

	drawText(hdc, appTitle, scaledRawRect(logicalRect(22, 5, rawLogicalClientWidth()-150, titleBarHeight-5)), fontButton, titleTextColor, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
	drawThemeButton(hdc, dark)
	for _, button := range titleButtonRects() {
		drawTitleButton(hdc, button)
	}
	drawRawLine(hdc, 0, titleBarHeight, rawLogicalClientWidth(), titleBarHeight, separatorColor, 1)
}

func drawUIScene(hdc syscall.Handle, client rect, dark bool) {
	setPalette(dark)
	drawBackground(hdc, client)
	drawTitleBar(hdc, client, dark)

	drawText(hdc, "System-wide gain", scaledRect(logicalRect(0, 48, 760, 70)), fontSmall, palette.muted, dtCenter|dtVCenter|dtSingleLine)

	shownPct := int(math.Round(displayPct))
	drawText(hdc, fmt.Sprintf("%d%%", shownPct), scaledRect(logicalRect(80, 70, 680, 168)), fontValue, palette.text, dtCenter|dtVCenter|dtSingleLine)
	drawText(hdc, fmt.Sprintf("%+.2f dB", percentToDB(currentPct)), scaledRect(logicalRect(530, 104, 675, 134)), fontDB, palette.text, dtRight|dtVCenter|dtSingleLine)

	drawSlider(hdc)
	for i, pct := range []int{100, 200, 300, 400, 500} {
		drawPresetButton(hdc, i, pct)
	}

	drawLine(hdc, 80, 360, 680, 360, palette.border, 1)
	drawActionButton(hdc, deviceRect, uiDevice, "Audio devices")
	drawActionButton(hdc, repairRect, uiRepair, "Repair")

	drawSettingRow(hdc, startupRect, uiStartup, "Start with Windows", "Launch quietly in the tray", startupToggleVisual)
	drawSettingRow(hdc, closeTrayRect, uiCloseToTray, "Close to tray", "Exit still resets gain to 100%", closeToggleVisual)
	drawStatusCard(hdc)
	drawWarning(hdc)
}

func drawUI(hdc syscall.Handle, client rect) {
	if !themeTransition {
		drawUIScene(hdc, client, settings.DarkMode)
		return
	}

	// Draw the old theme first, then reveal the new theme through an expanding
	// circle originating at the theme button (native equivalent of the site's
	// circular View Transition effect).
	drawUIScene(hdc, client, themeTransitionFrom)
	elapsed := time.Since(themeTransitionStart).Seconds() / 0.46
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > 1 {
		elapsed = 1
	}
	ease := 1 - math.Pow(1-elapsed, 3)
	theme := titleThemeRect()
	centerX := scaleInt((theme.left + theme.right) / 2)
	centerY := scaleInt((theme.top + theme.bottom) / 2)
	maxRadius := int32(math.Ceil(math.Hypot(float64(client.right), float64(client.bottom))))
	radius := int32(math.Round(float64(maxRadius) * ease))
	region, _, _ := procCreateEllipticRgn.Call(
		uintptr(centerX-radius), uintptr(centerY-radius),
		uintptr(centerX+radius+1), uintptr(centerY+radius+1),
	)
	if region != 0 {
		procSelectClipRgn.Call(uintptr(hdc), region)
		setGDIPlusClip(region)
		drawUIScene(hdc, client, themeTransitionTo)
		procSelectClipRgn.Call(uintptr(hdc), 0)
		resetGDIPlusClip()
		procDeleteObject.Call(region)
	}
}

func paintWindow(hwnd syscall.Handle) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer procEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))

	var client rect
	procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&client)))
	memDC, _, _ := procCreateCompatibleDC.Call(hdc)
	bitmap, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(client.right-client.left), uintptr(client.bottom-client.top))
	oldBitmap, _, _ := procSelectObject.Call(memDC, bitmap)

	beginGDIPlus(syscall.Handle(memDC))
	drawUI(syscall.Handle(memDC), client)
	endGDIPlus()
	procBitBlt.Call(hdc, 0, 0, uintptr(client.right), uintptr(client.bottom), memDC, 0, 0, srccopy)

	procSelectObject.Call(memDC, oldBitmap)
	procDeleteObject.Call(bitmap)
	procDeleteDC.Call(memDC)
}

func hitTest(p point) int {
	if pointInRect(p, sliderRect) {
		return uiSlider
	}
	for i, r := range presetRects {
		if pointInRect(p, r) {
			return uiPreset100 + i
		}
	}
	if pointInRect(p, deviceRect) {
		return uiDevice
	}
	if pointInRect(p, repairRect) {
		return uiRepair
	}
	if pointInRect(p, startupRect) {
		return uiStartup
	}
	if pointInRect(p, closeTrayRect) {
		return uiCloseToTray
	}
	if pointInRect(p, logsRect) {
		return uiLogs
	}
	return uiNone
}

func hitTestAll(raw, content point) int {
	if id := titleHitTest(raw); id != uiNone {
		return id
	}
	return hitTest(content)
}

func percentFromSliderPoint(p point) int {
	travelLeft := sliderRect.left + sliderKnobInset
	travelRight := sliderRect.right - sliderKnobInset
	position := float64(p.x-travelLeft) / float64(travelRight-travelLeft)
	if position < 0 {
		position = 0
	}
	if position > 1 {
		position = 1
	}
	return clampPercent(100 + int(math.Round(position*400)))
}

func setPercentFromMouse(p point, forceLive bool) {
	percent := percentFromSliderPoint(p)
	if forceLive || percent != dragAppliedPct && (dragAppliedAt.IsZero() || time.Since(dragAppliedAt) >= 35*time.Millisecond) {
		applyPercent(percent, true, false)
		dragAppliedPct = percent
		dragAppliedAt = time.Now()
	} else {
		currentPct = percent
		targetPct = percent
		displayPct = float64(percent)
		statusText = fmt.Sprintf("Adjusting · %d%% · %+.2f dB", percent, percentToDB(percent))
		currentStatusTone = toneReady
	}
	invalidateWindow()
}

func executeElement(id int) {
	switch id {
	case uiPreset100:
		applyPercent(100, true, true)
	case uiPreset200:
		applyPercent(200, true, true)
	case uiPreset300:
		applyPercent(300, true, true)
	case uiPreset400:
		applyPercent(400, true, true)
	case uiPreset500:
		applyPercent(500, true, true)
	case uiDevice:
		openDeviceSetup()
	case uiRepair:
		repairIntegration()
	case uiStartup:
		toggleStartup()
	case uiCloseToTray:
		toggleCloseToTray()
	case uiLogs:
		openLogs()
	case uiTheme:
		beginThemeSwitch()
	case uiTitleMin:
		procShowWindow.Call(uintptr(hwndMain), swMinimize)
	case uiTitleMax:
		if zoomed, _, _ := procIsZoomed.Call(uintptr(hwndMain)); zoomed != 0 {
			procShowWindow.Call(uintptr(hwndMain), swRestore)
		} else {
			procShowWindow.Call(uintptr(hwndMain), swMaximize)
		}
	case uiTitleClose:
		procSendMessageW.Call(uintptr(hwndMain), wmClose, 0, 0)
	}
}

func trackMouse(hwnd syscall.Handle) {
	if mouseTracked {
		return
	}
	event := trackMouseEvent{cbSize: uint32(unsafe.Sizeof(trackMouseEvent{})), dwFlags: tmeLeave, hwndTrack: hwnd}
	procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&event)))
	mouseTracked = true
}

func onMouseMove(hwnd syscall.Handle, lParam uintptr) {
	raw := unscaleRawPoint(lowordSigned(lParam), hiwordSigned(lParam))
	p := unscalePoint(lowordSigned(lParam), hiwordSigned(lParam))
	trackMouse(hwnd)
	if isDragging {
		setPercentFromMouse(p, false)
		return
	}
	newHover := hitTestAll(raw, p)
	if newHover != hoverElement {
		hoverElement = newHover
		invalidateWindow()
	}
}

func onMouseDown(hwnd syscall.Handle, lParam uintptr) {
	raw := unscaleRawPoint(lowordSigned(lParam), hiwordSigned(lParam))
	p := unscalePoint(lowordSigned(lParam), hiwordSigned(lParam))
	pressedElement = hitTestAll(raw, p)
	if pressedElement == uiSlider {
		isDragging = true
		dragAppliedPct = 0
		dragAppliedAt = time.Time{}
		procSetCapture.Call(uintptr(hwnd))
		setPercentFromMouse(p, true)
	}
	invalidateWindow()
}

func onMouseUp(lParam uintptr) {
	raw := unscaleRawPoint(lowordSigned(lParam), hiwordSigned(lParam))
	p := unscalePoint(lowordSigned(lParam), hiwordSigned(lParam))
	if isDragging {
		setPercentFromMouse(p, true)
		isDragging = false
		procReleaseCapture.Call()
		pressedElement = uiNone
		invalidateWindow()
		return
	}
	id := pressedElement
	pressedElement = uiNone
	if id != uiNone && hitTestAll(raw, p) == id {
		executeElement(id)
	}
	invalidateWindow()
}

func beginThemeSwitch() {
	if themeTransition {
		return
	}
	themeTransition = true
	themeTransitionFrom = settings.DarkMode
	themeTransitionTo = !settings.DarkMode
	themeTransitionStart = time.Now()
	settings.DarkMode = themeTransitionTo
	if err := saveSettings(settings); err != nil {
		logEvent("theme setting save failed: %v", err)
	}
	setDWMStyle(hwndMain)
	logEvent("theme switch begin: dark=%t", settings.DarkMode)
	invalidateWindow()
}

func easeVisual(value, target float64) float64 {
	delta := target - value
	if math.Abs(delta) < 0.002 {
		return target
	}
	return value + delta*0.24
}

func easeTitleButton(value float64, active bool) float64 {
	target := 0.0
	if active {
		target = 1
	}
	if value < target {
		value += 0.2
		if value > target {
			return target
		}
	} else if value > target {
		value -= 0.2
		if value < target {
			return target
		}
	}
	return value
}

func tickAnimation() {
	animationPhase = (animationPhase + 1) % 10000
	delta := float64(targetPct) - displayPct
	if math.Abs(delta) > 0.08 {
		displayPct += delta * 0.24
	} else {
		displayPct = float64(targetPct)
	}
	startupToggleVisual = easeVisual(startupToggleVisual, boolFloat(settings.StartWithWindows))
	closeToggleVisual = easeVisual(closeToggleVisual, boolFloat(settings.CloseToTray))
	titleMinVisual = easeTitleButton(titleMinVisual, hoverElement == uiTitleMin || pressedElement == uiTitleMin)
	titleMaxVisual = easeTitleButton(titleMaxVisual, hoverElement == uiTitleMax || pressedElement == uiTitleMax)
	titleCloseVisual = easeTitleButton(titleCloseVisual, hoverElement == uiTitleClose || pressedElement == uiTitleClose)
	if themeTransition && time.Since(themeTransitionStart) >= 460*time.Millisecond {
		themeTransition = false
		setPalette(settings.DarkMode)
		setDWMStyle(hwndMain)
		logEvent("theme switch complete: dark=%t", settings.DarkMode)
	}
	updateTrayAnimation(animationPhase)
	invalidateWindow()
}

func setDWMStyle(hwnd syscall.Handle) {
	cornerPreference := uint32(2) // DWMWCP_ROUND
	procDwmSetWindowAttribute.Call(uintptr(hwnd), 33, uintptr(unsafe.Pointer(&cornerPreference)), unsafe.Sizeof(cornerPreference))
	setPalette(settings.DarkMode)
	captionColor := uint32(palette.background)
	borderColor := uint32(palette.border)
	procDwmSetWindowAttribute.Call(uintptr(hwnd), 35, uintptr(unsafe.Pointer(&captionColor)), unsafe.Sizeof(captionColor))
	procDwmSetWindowAttribute.Call(uintptr(hwnd), 34, uintptr(unsafe.Pointer(&borderColor)), unsafe.Sizeof(borderColor))
	dark := uint32(0)
	if settings.DarkMode {
		dark = 1
	}
	procDwmSetWindowAttribute.Call(uintptr(hwnd), 20, uintptr(unsafe.Pointer(&dark)), unsafe.Sizeof(dark))
	procDwmSetWindowAttribute.Call(uintptr(hwnd), 19, uintptr(unsafe.Pointer(&dark)), unsafe.Sizeof(dark))
	backdrop := uint32(2)
	procDwmSetWindowAttribute.Call(uintptr(hwnd), 38, uintptr(unsafe.Pointer(&backdrop)), unsafe.Sizeof(backdrop))
}

func hideWindowToTray() {
	procShowWindow.Call(uintptr(hwndMain), swHide)
	setStatus("Running in the system tray", toneReady)
}

func showMainWindow() {
	procShowWindow.Call(uintptr(hwndMain), swRestore)
	procSetForegroundWindow.Call(uintptr(hwndMain))
	invalidateWindow()
}

func wndProc(hwnd syscall.Handle, message uint32, wParam, lParam uintptr) (result uintptr) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logRecoveredPanic("wndProc", recovered)
			result = 0
		}
	}()

	switch message {
	case wmCreate:
		hwndMain = hwnd
		if dpi, _, _ := procGetDpiForWindow.Call(uintptr(hwnd)); dpi != 0 {
			dpiScale = float64(dpi) / 96.0
		}
		createFonts()
		setDWMStyle(hwnd)
		windowIcon = loadAppIcon()
		if windowIcon != 0 {
			procSendMessageW.Call(uintptr(hwnd), wmSetIcon, iconBig, uintptr(windowIcon))
			procSendMessageW.Call(uintptr(hwnd), wmSetIcon, iconSmall, uintptr(windowIcon))
		}
		addTrayIcon()
		startupToggleVisual = boolFloat(settings.StartWithWindows)
		closeToggleVisual = boolFloat(settings.CloseToTray)
		applyPercent(currentPct, false, false)
		procSetTimer.Call(uintptr(hwnd), animationTimerID, 16, 0)
		return 0

	case wmPaint:
		paintWindow(hwnd)
		return 0

	case wmEraseBkgnd:
		return 1

	case wmNcCalcSize:
		return 0

	case wmNcHitTest:
		screenPoint := point{x: lowordSigned(lParam), y: hiwordSigned(lParam)}
		procScreenToClient.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&screenPoint)))
		raw := unscaleRawPoint(screenPoint.x, screenPoint.y)
		if titleHitTest(raw) != uiNone {
			return htClient
		}
		if hit := resizeHitTest(raw); hit != 0 {
			return hit
		}
		if raw.y >= 0 && raw.y < titleBarHeight {
			return htCaption
		}
		r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
		return r

	case wmNcRButtonUp:
		if wParam == htCaption {
			showTrayMenu()
			return 0
		}
		r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
		return r

	case wmMouseMove:
		onMouseMove(hwnd, lParam)
		return 0

	case wmMouseLeave:
		mouseTracked = false
		hoverElement = uiNone
		invalidateWindow()
		return 0

	case wmLButtonDown:
		onMouseDown(hwnd, lParam)
		return 0

	case wmLButtonUp:
		onMouseUp(lParam)
		return 0

	case wmTimer:
		if wParam == animationTimerID {
			tickAnimation()
		}
		return 0

	case wmCommand:
		handleTrayCommand(loword(wParam))
		return 0

	case wmTrayIcon:
		handleTrayMessage(lParam)
		return 0

	case wmSize:
		if wParam == sizeMinimized && settings.CloseToTray {
			hideWindowToTray()
		}
		return 0

	case wmGetMinMaxInfo:
		info := (*minMaxInfo)(unsafe.Pointer(lParam))
		info.ptMinTrackSize.x = scaleInt(logicalClientWidth)
		info.ptMinTrackSize.y = scaleInt(logicalClientHeight)
		return 0

	case wmDpiChanged:
		newDpi := uint32(wParam & 0xffff)
		if newDpi != 0 {
			dpiScale = float64(newDpi) / 96.0
			createFonts()
		}
		if lParam != 0 {
			var suggested rect
			procRtlMoveMemory.Call(uintptr(unsafe.Pointer(&suggested)), lParam, unsafe.Sizeof(suggested))
			procSetWindowPos.Call(uintptr(hwnd), 0, uintptr(suggested.left), uintptr(suggested.top), uintptr(suggested.right-suggested.left), uintptr(suggested.bottom-suggested.top), 0x0010|0x0004)
		}
		invalidateWindow()
		return 0

	case wmClose:
		if settings.CloseToTray && !isClosing {
			hideWindowToTray()
			return 0
		}
		requestExit()
		return 0

	case wmDestroy:
		procKillTimer.Call(uintptr(hwnd), animationTimerID)
		removeTrayIcon()
		destroyTrayIcons()
		destroyFonts()
		if windowIcon != 0 {
			procDestroyIcon.Call(uintptr(windowIcon))
			windowIcon = 0
		}
		procPostQuitMessage.Call(0)
		return 0
	}

	r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return r
}

func runWindow() error {
	initGDIPlus()
	defer shutdownGDIPlus()
	// Per-monitor DPI awareness on modern Windows; fallback for older builds.
	if r, _, _ := procSetProcessDpiAwarenessContext.Call(^uintptr(3)); r == 0 { // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
		procSetProcessDPIAware.Call()
	}
	if dpi, _, _ := procGetDpiForSystem.Call(); dpi != 0 {
		dpiScale = float64(dpi) / 96.0
	}

	hInst, _, _ := procGetModuleHandleW.Call(0)
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	className := utf16(appWindowClassName)
	wndProcCallback = syscall.NewCallback(wndProc)
	classIcon := loadAppIcon()
	wc := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		style:         csHRedraw | csVRedraw | csDblClks,
		lpfnWndProc:   wndProcCallback,
		hInstance:     syscall.Handle(hInst),
		hIcon:         classIcon,
		hCursor:       syscall.Handle(cursor),
		hbrBackground: 0,
		lpszClassName: className,
		hIconSm:       classIcon,
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		if classIcon != 0 {
			procDestroyIcon.Call(uintptr(classIcon))
		}
		return fmt.Errorf("RegisterClassExW failed: %v", err)
	}

	clientWidth := scaleInt(logicalClientWidth)
	clientHeight := scaleInt(logicalClientHeight)
	style := uintptr(wsPopup | wsThickFrame | wsSysMenu | wsMinimizeBox | wsMaximizeBox | wsClipChildren)
	windowWidth := clientWidth
	windowHeight := clientHeight
	screenW, _, _ := procGetSystemMetrics.Call(0)
	screenH, _, _ := procGetSystemMetrics.Call(1)
	x := (int32(screenW) - windowWidth) / 2
	y := (int32(screenH) - windowHeight) / 2

	h, _, createErr := procCreateWindowExW.Call(
		wsExAppWindow,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(utf16(appTitle))),
		style,
		uintptr(x), uintptr(y), uintptr(windowWidth), uintptr(windowHeight),
		0, 0, hInst, 0,
	)
	if classIcon != 0 {
		procDestroyIcon.Call(uintptr(classIcon))
	}
	if h == 0 {
		return fmt.Errorf("CreateWindowExW failed: %v", createErr)
	}
	hwndMain = syscall.Handle(h)

	if !startupLaunch {
		procShowWindow.Call(h, swShow)
		procUpdateWindow.Call(h)
	} else {
		logEvent("startup launch: window created hidden in tray")
	}

	startHeartbeat()
	var message msg
	for {
		r, _, getErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		value := int32(r)
		if value == -1 {
			return fmt.Errorf("GetMessageW failed: %v", getErr)
		}
		if value == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
	}
	return nil
}

// Keep time imported in this file for deterministic animation diagnostics.
var _ = time.Second
