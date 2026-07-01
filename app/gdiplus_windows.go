//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

const (
	gdipSmoothingAntiAlias = 4
	gdipLinearHorizontal   = 0
	gdipWrapModeTile       = 0
	gdipCombineReplace     = 0
)

type gdipStartupInput struct {
	version                  uint32
	debugEventCallback       uintptr
	suppressBackgroundThread int32
	suppressExternalCodecs   int32
}

type gdipRectI struct {
	x, y, width, height int32
}

var (
	gdiplus = syscall.NewLazyDLL("gdiplus.dll")

	procGdiplusStartup           = gdiplus.NewProc("GdiplusStartup")
	procGdiplusShutdown          = gdiplus.NewProc("GdiplusShutdown")
	procGdipCreateFromHDC        = gdiplus.NewProc("GdipCreateFromHDC")
	procGdipDeleteGraphics       = gdiplus.NewProc("GdipDeleteGraphics")
	procGdipSetSmoothingMode     = gdiplus.NewProc("GdipSetSmoothingMode")
	procGdipCreateSolidFill      = gdiplus.NewProc("GdipCreateSolidFill")
	procGdipCreateLineBrushRectI = gdiplus.NewProc("GdipCreateLineBrushFromRectI")
	procGdipDeleteBrush          = gdiplus.NewProc("GdipDeleteBrush")
	procGdipFillRectangleI       = gdiplus.NewProc("GdipFillRectangleI")
	procGdipFillEllipseI         = gdiplus.NewProc("GdipFillEllipseI")
	procGdipSetClipHrgn          = gdiplus.NewProc("GdipSetClipHrgn")
	procGdipResetClip            = gdiplus.NewProc("GdipResetClip")

	gdipToken     uintptr
	paintGraphics uintptr
)

func initGDIPlus() bool {
	if gdipToken != 0 {
		return true
	}
	input := gdipStartupInput{version: 1}
	status, _, _ := procGdiplusStartup.Call(uintptr(unsafe.Pointer(&gdipToken)), uintptr(unsafe.Pointer(&input)), 0)
	if status != 0 {
		gdipToken = 0
		logEvent("GDI+ initialization failed: status=%d", status)
		return false
	}
	logEvent("GDI+ initialized for anti-aliased rendering")
	return true
}

func shutdownGDIPlus() {
	if gdipToken != 0 {
		procGdiplusShutdown.Call(gdipToken)
		gdipToken = 0
	}
}

func beginGDIPlus(hdc syscall.Handle) {
	paintGraphics = 0
	if gdipToken == 0 {
		return
	}
	status, _, _ := procGdipCreateFromHDC.Call(uintptr(hdc), uintptr(unsafe.Pointer(&paintGraphics)))
	if status != 0 || paintGraphics == 0 {
		paintGraphics = 0
		return
	}
	procGdipSetSmoothingMode.Call(paintGraphics, gdipSmoothingAntiAlias)
}

func endGDIPlus() {
	if paintGraphics != 0 {
		procGdipDeleteGraphics.Call(paintGraphics)
		paintGraphics = 0
	}
}

func argbFromColor(color uintptr) uintptr {
	r, g, b := colorComponents(color)
	return uintptr(0xff000000 | uint32(r)<<16 | uint32(g)<<8 | uint32(b))
}

func argbWithAlpha(color uintptr, alpha uint8) uintptr {
	r, g, b := colorComponents(color)
	return uintptr(uint32(alpha)<<24 | uint32(r)<<16 | uint32(g)<<8 | uint32(b))
}

// gdipFillRoundedRectAlpha fills a translucent rounded rectangle. Soft shadows
// and glows are built from stacked translucent layers; there is no GDI fallback
// because plain GDI cannot blend, so callers simply lose the effect if GDI+ is
// unavailable.
func gdipFillRoundedRectAlpha(target rect, radius int32, color uintptr, alpha uint8) bool {
	if paintGraphics == 0 {
		return false
	}
	var brush uintptr
	status, _, _ := procGdipCreateSolidFill.Call(argbWithAlpha(color, alpha), uintptr(unsafe.Pointer(&brush)))
	if status != 0 || brush == 0 {
		return false
	}
	defer procGdipDeleteBrush.Call(brush)
	return fillRoundedWithBrush(target, radius, brush)
}

func gdipFillCircleAlpha(target rect, color uintptr, alpha uint8) bool {
	if paintGraphics == 0 {
		return false
	}
	var brush uintptr
	status, _, _ := procGdipCreateSolidFill.Call(argbWithAlpha(color, alpha), uintptr(unsafe.Pointer(&brush)))
	if status != 0 || brush == 0 {
		return false
	}
	defer procGdipDeleteBrush.Call(brush)
	procGdipFillEllipseI.Call(
		paintGraphics, brush,
		uintptr(target.left), uintptr(target.top),
		uintptr(target.right-target.left), uintptr(target.bottom-target.top),
	)
	return true
}

func clampRoundRadius(target rect, radius int32) int32 {
	w := target.right - target.left
	h := target.bottom - target.top
	if w <= 0 || h <= 0 {
		return 0
	}
	limit := w / 2
	if h/2 < limit {
		limit = h / 2
	}
	if radius > limit {
		radius = limit
	}
	if radius < 1 {
		radius = 1
	}
	return radius
}

func fillRoundedWithBrush(target rect, radius int32, brush uintptr) bool {
	if paintGraphics == 0 || brush == 0 {
		return false
	}
	radius = clampRoundRadius(target, radius)
	if radius == 0 {
		return false
	}
	w := target.right - target.left
	h := target.bottom - target.top
	d := radius * 2
	// Two rectangles plus four anti-aliased corner ellipses produce a rounded
	// rectangle without any floating-point ABI calls.
	procGdipFillRectangleI.Call(paintGraphics, brush, uintptr(target.left+radius), uintptr(target.top), uintptr(w-d), uintptr(h))
	procGdipFillRectangleI.Call(paintGraphics, brush, uintptr(target.left), uintptr(target.top+radius), uintptr(w), uintptr(h-d))
	procGdipFillEllipseI.Call(paintGraphics, brush, uintptr(target.left), uintptr(target.top), uintptr(d), uintptr(d))
	procGdipFillEllipseI.Call(paintGraphics, brush, uintptr(target.right-d), uintptr(target.top), uintptr(d), uintptr(d))
	procGdipFillEllipseI.Call(paintGraphics, brush, uintptr(target.left), uintptr(target.bottom-d), uintptr(d), uintptr(d))
	procGdipFillEllipseI.Call(paintGraphics, brush, uintptr(target.right-d), uintptr(target.bottom-d), uintptr(d), uintptr(d))
	return true
}

func gdipFillRoundedRect(target rect, radius int32, color uintptr) bool {
	if paintGraphics == 0 {
		return false
	}
	var brush uintptr
	status, _, _ := procGdipCreateSolidFill.Call(argbFromColor(color), uintptr(unsafe.Pointer(&brush)))
	if status != 0 || brush == 0 {
		return false
	}
	defer procGdipDeleteBrush.Call(brush)
	return fillRoundedWithBrush(target, radius, brush)
}

func gdipGradientRoundedRect(target rect, radius int32, leftColor, rightColor uintptr) bool {
	if paintGraphics == 0 {
		return false
	}
	r := gdipRectI{x: target.left, y: target.top, width: target.right - target.left, height: target.bottom - target.top}
	var brush uintptr
	status, _, _ := procGdipCreateLineBrushRectI.Call(
		uintptr(unsafe.Pointer(&r)),
		argbFromColor(leftColor), argbFromColor(rightColor),
		gdipLinearHorizontal, gdipWrapModeTile,
		uintptr(unsafe.Pointer(&brush)),
	)
	if status != 0 || brush == 0 {
		return false
	}
	defer procGdipDeleteBrush.Call(brush)
	return fillRoundedWithBrush(target, radius, brush)
}

// Outlines remain on the GDI fallback path. The filled surfaces and moving
// controls—the visually prominent parts—are anti-aliased by GDI+.
func gdipStrokeRoundedRect(target rect, radius int32, color uintptr, width int32) bool {
	return false
}

func gdipFillCircle(target rect, color uintptr) bool {
	if paintGraphics == 0 {
		return false
	}
	var brush uintptr
	status, _, _ := procGdipCreateSolidFill.Call(argbFromColor(color), uintptr(unsafe.Pointer(&brush)))
	if status != 0 || brush == 0 {
		return false
	}
	defer procGdipDeleteBrush.Call(brush)
	procGdipFillEllipseI.Call(
		paintGraphics, brush,
		uintptr(target.left), uintptr(target.top),
		uintptr(target.right-target.left), uintptr(target.bottom-target.top),
	)
	return true
}

func gdipStrokeCircle(target rect, color uintptr, width int32) bool {
	return false
}

func setGDIPlusClip(region uintptr) {
	if paintGraphics != 0 && region != 0 {
		procGdipSetClipHrgn.Call(paintGraphics, region, gdipCombineReplace)
	}
}

func resetGDIPlusClip() {
	if paintGraphics != 0 {
		procGdipResetClip.Call(paintGraphics)
	}
}
