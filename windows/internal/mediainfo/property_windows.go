//go:build windows

package mediainfo

import (
	"runtime"
	"syscall"
	"unsafe"
)

type guid struct {
	Data1        uint32
	Data2, Data3 uint16
	Data4        [8]byte
}
type propertyKey struct {
	Format guid
	ID     uint32
}
type propertyValue struct {
	Type     uint16
	Reserved [3]uint16
	Value    uint64
	Extra    uint64
}
type propertyStore struct {
	VTable *struct{ QueryInterface, AddRef, Release, GetCount, GetAt, GetValue, SetValue, Commit uintptr }
}

var (
	ole32            = syscall.NewLazyDLL("ole32.dll")
	coInitializeEx   = ole32.NewProc("CoInitializeEx")
	coUninitialize   = ole32.NewProc("CoUninitialize")
	propVariantClear = ole32.NewProc("PropVariantClear")
	getPropertyStore = syscall.NewLazyDLL("shell32.dll").NewProc("SHGetPropertyStoreFromParsingName")
	propertyStoreID  = guid{0x886d8eeb, 0x8cf2, 0x4446, [8]byte{0x8d, 0x02, 0xcd, 0xba, 0x1d, 0xbd, 0xcf, 0x99}}
	durationKey      = propertyKey{guid{0x64440490, 0x4c8b, 0x11d1, [8]byte{0x8b, 0x70, 0x08, 0x00, 0x36, 0xb1, 0x1a, 0x03}}, 3}
)

// windowsDuration uses the system's property handler for supported media types.
func windowsDuration(path string) *int {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := coInitializeEx.Call(0, 0)
	if int32(hr) < 0 {
		return nil
	}
	defer coUninitialize.Call()
	var store *propertyStore
	hr, _, _ = getPropertyStore.Call(uintptr(unsafe.Pointer(name)), 0, 0, uintptr(unsafe.Pointer(&propertyStoreID)), uintptr(unsafe.Pointer(&store)))
	if int32(hr) < 0 || store == nil {
		return nil
	}
	defer syscall.SyscallN(store.VTable.Release, uintptr(unsafe.Pointer(store)))
	var value propertyValue
	hr, _, _ = syscall.SyscallN(store.VTable.GetValue, uintptr(unsafe.Pointer(store)), uintptr(unsafe.Pointer(&durationKey)), uintptr(unsafe.Pointer(&value)))
	defer propVariantClear.Call(uintptr(unsafe.Pointer(&value)))
	if int32(hr) < 0 {
		return nil
	}
	if value.Type != 21 || value.Value/10000000 > 10*365*24*3600 {
		return nil
	}
	seconds := int(value.Value / 10000000)
	return &seconds
}
