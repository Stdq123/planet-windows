//go:build windows

package mediainfo

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"runtime"
	"syscall"
	"unsafe"
)

type imageFactory struct {
	VTable *struct{ QueryInterface, AddRef, Release, GetImage uintptr }
}
type bitmap struct {
	Type, Width, Height, WidthBytes int32
	Planes, BitsPixel               uint16
	Bits                            uintptr
}
type bitmapInfo struct {
	Size                         uint32
	Width, Height                int32
	Planes, BitCount             uint16
	Compression, SizeImage       uint32
	XPelsPerMeter, YPelsPerMeter int32
	ClrUsed, ClrImportant        uint32
	Colors                       [4]byte
}

var (
	createShellItem = syscall.NewLazyDLL("shell32.dll").NewProc("SHCreateItemFromParsingName")
	imageFactoryID  = guid{0xbcc18b79, 0xba16, 0x442f, [8]byte{0x80, 0xc4, 0x8a, 0x59, 0xc3, 0x0c, 0x46, 0x3b}}
	gdi             = syscall.NewLazyDLL("gdi32.dll")
	getObject       = gdi.NewProc("GetObjectW")
	deleteObject    = gdi.NewProc("DeleteObject")
	createDC        = gdi.NewProc("CreateCompatibleDC")
	deleteDC        = gdi.NewProc("DeleteDC")
	getDIBits       = gdi.NewProc("GetDIBits")
)

// VideoThumbnail returns a PNG thumbnail when Windows has a suitable handler.
func VideoThumbnail(path string) ([]byte, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := coInitializeEx.Call(0, 0)
	if int32(hr) < 0 {
		return nil, errors.New("COM initialization failed")
	}
	defer coUninitialize.Call()
	var factory *imageFactory
	hr, _, _ = createShellItem.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&imageFactoryID)), uintptr(unsafe.Pointer(&factory)))
	if int32(hr) < 0 || factory == nil {
		return nil, errors.New("video thumbnail handler unavailable")
	}
	defer syscall.SyscallN(factory.VTable.Release, uintptr(unsafe.Pointer(factory)))
	var handle uintptr
	const width = 640
	const height = 640
	size := uint64(width) | (uint64(height) << 32)
	hr, _, _ = syscall.SyscallN(factory.VTable.GetImage, uintptr(unsafe.Pointer(factory)), uintptr(size), 8, uintptr(unsafe.Pointer(&handle)))
	if int32(hr) < 0 || handle == 0 {
		return nil, errors.New("video thumbnail unavailable")
	}
	defer deleteObject.Call(handle)
	var b bitmap
	n, _, _ := getObject.Call(handle, unsafe.Sizeof(b), uintptr(unsafe.Pointer(&b)))
	if n == 0 || b.Width <= 0 || b.Height <= 0 || b.Width > 4096 || b.Height > 4096 {
		return nil, errors.New("video thumbnail dimensions invalid")
	}
	dc, _, _ := createDC.Call(0)
	if dc == 0 {
		return nil, errors.New("video thumbnail context failed")
	}
	defer deleteDC.Call(dc)
	info := bitmapInfo{Size: 40, Width: b.Width, Height: -b.Height, Planes: 1, BitCount: 32}
	pix := make([]byte, int(b.Width)*int(b.Height)*4)
	count, _, _ := getDIBits.Call(dc, handle, 0, uintptr(b.Height), uintptr(unsafe.Pointer(&pix[0])), uintptr(unsafe.Pointer(&info)), 0)
	if count != uintptr(b.Height) {
		return nil, errors.New("video thumbnail pixels unavailable")
	}
	img := image.NewNRGBA(image.Rect(0, 0, int(b.Width), int(b.Height)))
	for i := 0; i < len(pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = pix[i+2], pix[i+1], pix[i], 255
	}
	var out bytes.Buffer
	if err = png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
