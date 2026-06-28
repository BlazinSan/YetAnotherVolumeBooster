//go:build windows

package main

import (
	"math"
	"syscall"
	"unsafe"
)

const (
	imageIcon      = 1
	lrLoadFromFile = 0x00000010
	lrDefaultSize  = 0x00000040
	dibRGBColors   = 0
	biRGB          = 0
)

type bitmapInfoHeader struct {
	size          uint32
	width         int32
	height        int32
	planes        uint16
	bitCount      uint16
	compression   uint32
	sizeImage     uint32
	xPelsPerMeter int32
	yPelsPerMeter int32
	clrUsed       uint32
	clrImportant  uint32
}

type bitmapInfo struct {
	header bitmapInfoHeader
	colors [1]uint32
}

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  syscall.Handle
	hbmColor syscall.Handle
}

var (
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procCreateBitmap       = gdi32.NewProc("CreateBitmap")
	procCreateIconIndirect = user32.NewProc("CreateIconIndirect")
)

func loadAppIcon() syscall.Handle {
	if fileExists(iconFile()) {
		h, _, _ := procLoadImageW.Call(
			0,
			uintptr(unsafe.Pointer(utf16(iconFile()))),
			imageIcon,
			0,
			0,
			lrLoadFromFile|lrDefaultSize,
		)
		if h != 0 {
			return syscall.Handle(h)
		}
	}
	return createVolumeIcon(64, 0, false)
}

func roundedAlpha(x, y, size float64) float64 {
	radius := size * 0.24
	left, top := 1.0, 1.0
	right, bottom := size-1.0, size-1.0
	cx := math.Max(left+radius, math.Min(x, right-radius))
	cy := math.Max(top+radius, math.Min(y, bottom-radius))
	dx, dy := x-cx, y-cy
	distance := math.Sqrt(dx*dx+dy*dy) - radius
	if distance <= -0.5 {
		return 1
	}
	if distance >= 0.8 {
		return 0
	}
	return math.Max(0, math.Min(1, 0.8-distance))
}

func mixByte(a, b uint8, t float64) uint8 {
	return uint8(math.Round(float64(a)*(1-t) + float64(b)*t))
}

func createVolumeIcon(size int, frame int, idle bool) syscall.Handle {
	pixels := make([]byte, size*size*4)
	s := float64(size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			alpha := roundedAlpha(float64(x)+0.5, float64(y)+0.5, s)
			if alpha <= 0 {
				continue
			}
			t := float64(x) / math.Max(1, float64(size-1))
			var r, g, b uint8
			if idle {
				r, g, b = mixByte(46, 69, t), mixByte(78, 116, t), mixByte(80, 108, t)
			} else {
				r, g, b = mixByte(10, 90, t), mixByte(92, 209, t), mixByte(62, 168, t)
			}
			idx := (y*size + x) * 4
			pixels[idx+0] = b
			pixels[idx+1] = g
			pixels[idx+2] = r
			pixels[idx+3] = uint8(math.Round(alpha * 255))
		}
	}

	// Animated equalizer bars are the shared status/tray motif.
	barCount := 4
	barW := math.Max(2, float64(size)/10)
	gap := math.Max(1, float64(size)/13)
	totalW := float64(barCount)*barW + float64(barCount-1)*gap
	startX := (s - totalW) / 2
	baseHeights := []float64{0.30, 0.52, 0.70, 0.42}
	for i := 0; i < barCount; i++ {
		wave := 0.0
		if !idle {
			wave = 0.11 * math.Sin(float64(frame+i*2)*math.Pi/4)
		}
		height := s * math.Max(0.22, math.Min(0.78, baseHeights[i]+wave))
		x0 := int(math.Round(startX + float64(i)*(barW+gap)))
		x1 := int(math.Round(float64(x0) + barW))
		y0 := int(math.Round((s - height) / 2))
		y1 := int(math.Round((s + height) / 2))
		for y := y0; y < y1; y++ {
			if y < 0 || y >= size {
				continue
			}
			for x := x0; x < x1; x++ {
				if x < 0 || x >= size {
					continue
				}
				idx := (y*size + x) * 4
				pixels[idx+0] = 255
				pixels[idx+1] = 255
				pixels[idx+2] = 255
				pixels[idx+3] = 245
			}
		}
	}

	bmi := bitmapInfo{header: bitmapInfoHeader{
		size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		width:       int32(size),
		height:      -int32(size),
		planes:      1,
		bitCount:    32,
		compression: biRGB,
		sizeImage:   uint32(len(pixels)),
	}}
	var bits uintptr
	colorBitmap, _, _ := procCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bmi)), dibRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if colorBitmap == 0 || bits == 0 {
		return 0
	}
	procRtlMoveMemory.Call(bits, uintptr(unsafe.Pointer(&pixels[0])), uintptr(len(pixels)))

	maskBytes := make([]byte, ((size+15)/16)*2*size)
	maskBitmap, _, _ := procCreateBitmap.Call(uintptr(size), uintptr(size), 1, 1, uintptr(unsafe.Pointer(&maskBytes[0])))
	if maskBitmap == 0 {
		procDeleteObject.Call(colorBitmap)
		return 0
	}
	info := iconInfo{fIcon: 1, hbmMask: syscall.Handle(maskBitmap), hbmColor: syscall.Handle(colorBitmap)}
	hIcon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&info)))
	procDeleteObject.Call(maskBitmap)
	procDeleteObject.Call(colorBitmap)
	return syscall.Handle(hIcon)
}
