//go:build windows

package screen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"syscall"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	gdi32  = syscall.NewLazyDLL("gdi32.dll")

	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procCreateCompatibleDC  = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatBitmap  = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject        = gdi32.NewProc("SelectObject")
	procBitBlt              = gdi32.NewProc("BitBlt")
	procGetDIBits           = gdi32.NewProc("GetDIBits")
	procDeleteObject        = gdi32.NewProc("DeleteObject")
	procDeleteDC            = gdi32.NewProc("DeleteDC")
	procMapVirtualKey       = user32.NewProc("MapVirtualKeyW")

	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
)

const (
	smCXScreen = 0
	smCYScreen = 1

	srcCopy     = 0x00CC0020
	captureBlt  = 0x40000000
	dibRGBColor = 0
)

// bitmapInfoHeader mirrors BITMAPINFOHEADER (40 bytes).
type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// init declares DPI awareness before any device context exists.
//
// Without it GDI reports virtualised pixels: on a 125% scaled display a
// 1920x1080 screen yields a 1536x864 bitmap which the compositor then stretches
// back up — every glyph is rendered at 80% and resampled, which is exactly the
// "blurry, can barely read it" symptom.
func init() {
	if err := procSetProcessDpiAwarenessContext.Find(); err == nil {
		// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 == -4
		var perMonitorV2 int64 = -4
		if r, _, _ := procSetProcessDpiAwarenessContext.Call(uintptr(perMonitorV2)); r != 0 {
			return
		}
	}
	_, _, _ = procSetProcessDPIAware.Call()
}

func capture(ctx context.Context, maxWidth, quality int) ([]byte, int, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, 0, err
	}
	if quality <= 0 || quality > 100 {
		quality = 60
	}

	sw, _, _ := procGetSystemMetrics.Call(smCXScreen)
	sh, _, _ := procGetSystemMetrics.Call(smCYScreen)
	if sw == 0 || sh == 0 {
		return nil, 0, 0, errors.New("无法读取屏幕尺寸")
	}
	w, h := int(int32(sw)), int(int32(sh))

	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return nil, 0, 0, errors.New("GetDC 失败")
	}
	defer procReleaseDC.Call(0, screenDC)

	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, 0, 0, errors.New("CreateCompatibleDC 失败")
	}
	defer procDeleteDC.Call(memDC)

	bitmap, _, _ := procCreateCompatBitmap.Call(screenDC, uintptr(w), uintptr(h))
	if bitmap == 0 {
		return nil, 0, 0, errors.New("CreateCompatibleBitmap 失败")
	}
	defer procDeleteObject.Call(bitmap)

	old, _, _ := procSelectObject.Call(memDC, bitmap)
	if old == 0 {
		return nil, 0, 0, errors.New("SelectObject 失败")
	}
	defer procSelectObject.Call(memDC, old)

	r, _, _ := procBitBlt.Call(memDC, 0, 0, uintptr(w), uintptr(h), screenDC, 0, 0,
		uintptr(srcCopy|captureBlt))
	if r == 0 {
		// CAPTUREBLT fails on some drivers; plain SRCCOPY usually works.
		r, _, _ = procBitBlt.Call(memDC, 0, 0, uintptr(w), uintptr(h), screenDC, 0, 0, uintptr(srcCopy))
		if r == 0 {
			return nil, 0, 0, errors.New("BitBlt 失败")
		}
	}

	bi := bitmapInfoHeader{
		Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:       int32(w),
		Height:      -int32(h), // negative: top-down rows
		Planes:      1,
		BitCount:    32,
		Compression: 0, // BI_RGB
	}
	buf := make([]byte, w*h*4)
	r, _, _ = procGetDIBits.Call(memDC, bitmap, 0, uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&bi)), dibRGBColor)
	if r == 0 {
		return nil, 0, 0, errors.New("GetDIBits 失败")
	}

	// 32bpp BI_RGB is BGRA, one pixel per 4 bytes with no row padding.
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		src := buf[y*w*4:]
		dst := img.Pix[y*img.Stride:]
		for x := 0; x < w; x++ {
			dst[x*4+0] = src[x*4+2] // R <- B
			dst[x*4+1] = src[x*4+1] // G
			dst[x*4+2] = src[x*4+0] // B <- R
			dst[x*4+3] = 255
		}
	}

	out := img
	if maxWidth > 0 && w > maxWidth {
		out = downscale(img, maxWidth)
	}

	var b bytes.Buffer
	if err := jpeg.Encode(&b, out, &jpeg.Options{Quality: quality}); err != nil {
		return nil, 0, 0, fmt.Errorf("编码 JPEG 失败：%w", err)
	}
	bnd := out.Bounds()
	return b.Bytes(), bnd.Dx(), bnd.Dy(), nil
}

// downscale shrinks by nearest-neighbour sampling. A box filter would look
// better but costs more CPU than the bandwidth it saves at these sizes.
func downscale(src *image.RGBA, targetW int) *image.RGBA {
	sw := src.Bounds().Dx()
	sh := src.Bounds().Dy()
	if targetW >= sw {
		return src
	}
	ratio := float64(sw) / float64(targetW)
	th := int(float64(sh) / ratio)
	if th < 1 {
		th = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, targetW, th))
	for y := 0; y < th; y++ {
		sy := int(float64(y) * ratio)
		if sy >= sh {
			sy = sh - 1
		}
		for x := 0; x < targetW; x++ {
			sx := int(float64(x) * ratio)
			if sx >= sw {
				sx = sw - 1
			}
			si := sy*src.Stride + sx*4
			di := y*dst.Stride + x*4
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}
