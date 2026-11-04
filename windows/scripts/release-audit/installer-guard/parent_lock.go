//go:build windows

package main

import (
	"errors"
	"golang.org/x/sys/windows"
	"strconv"
	"strings"
	"unsafe"
)

func parseMutexHandle(s string) (windows.Handle, error) {
	base := 10
	if strings.HasPrefix(strings.ToLower(s), "0x") {
		s = s[2:]
		base = 16
	}
	v, e := strconv.ParseUint(s, base, 64)
	return windows.Handle(v), e
}

// Duplicate the exact kernel object from the actual NSIS parent, rather than
// opening whatever object happens to have the same name after a parent crash.
func retainInstallerLock(pid uint32, source windows.Handle) (windows.Handle, error) {
	var info windows.PROCESS_BASIC_INFORMATION
	if e := windows.NtQueryInformationProcess(windows.CurrentProcess(), windows.ProcessBasicInformation, unsafe.Pointer(&info), uint32(unsafe.Sizeof(info)), nil); e != nil {
		return 0, e
	}
	if info.InheritedFromUniqueProcessId != uintptr(pid) {
		return 0, errors.New("installer parent identity differs")
	}
	parent, e := windows.OpenProcess(windows.PROCESS_DUP_HANDLE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if e != nil {
		return 0, e
	}
	defer windows.CloseHandle(parent)
	var parentBorn, childBorn, exit, kernel, user windows.Filetime
	if e = windows.GetProcessTimes(parent, &parentBorn, &exit, &kernel, &user); e != nil {
		return 0, e
	}
	if e = windows.GetProcessTimes(windows.CurrentProcess(), &childBorn, &exit, &kernel, &user); e != nil {
		return 0, e
	}
	stamp := func(f windows.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }
	if stamp(parentBorn) > stamp(childBorn) {
		return 0, errors.New("installer parent process ID was reused")
	}
	if status, e := windows.WaitForSingleObject(parent, 0); e != nil || status != uint32(windows.WAIT_TIMEOUT) {
		return 0, errors.New("installer parent exited before handshake")
	}
	var held windows.Handle
	if e = windows.DuplicateHandle(parent, source, windows.CurrentProcess(), &held, windows.SYNCHRONIZE, false, 0); e != nil {
		return 0, e
	}
	return held, nil
}
