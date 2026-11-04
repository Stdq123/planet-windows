//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

func validatePrivateDACL(actual, expected *windows.SECURITY_DESCRIPTOR) error {
	if actual == nil || expected == nil {
		return errors.New("missing private key security descriptor")
	}
	control, _, err := actual.Control()
	if err != nil {
		return err
	}
	dacl, _, err := actual.DACL()
	if err != nil {
		return err
	}
	actualSDDL, expectedSDDL := actual.String(), expected.String()
	if control&windows.SE_DACL_PROTECTED == 0 || dacl == nil || actualSDDL == "" || expectedSDDL == "" || actualSDDL != expectedSDDL {
		return errors.New("private key DACL is not protected and restricted to the current user")
	}
	return nil
}

// Consume the handle, returning it only after verifying its actual DACL. On
// failure, delete only this newly created empty file through that same handle.
func securePrivateHandle(handle windows.Handle, path string, expected *windows.SECURITY_DESCRIPTOR) (*os.File, error) {
	actual, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err == nil {
		err = validatePrivateDACL(actual, expected)
	}
	if err == nil {
		file := os.NewFile(uintptr(handle), path)
		if file != nil {
			return file, nil
		}
		err = errors.New("cannot open new private key")
	}
	deleteFile := byte(1)
	deleteErr := windows.SetFileInformationByHandle(handle, windows.FileDispositionInfo, &deleteFile, 1)
	closeErr := windows.CloseHandle(handle)
	if deleteErr != nil || closeErr != nil {
		return nil, fmt.Errorf("private key security check failed; empty file may remain at %s: %w", path, errors.Join(err, deleteErr, closeErr))
	}
	return nil, fmt.Errorf("private key security check failed: %w", err)
}

// The protected descriptor is applied atomically when the new file is created.
func createPrivateFile(path string) (*os.File, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	attributes := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: descriptor}
	handle, err := windows.CreateFile(name, windows.GENERIC_WRITE|windows.READ_CONTROL|windows.DELETE, 0, &attributes, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	runtime.KeepAlive(descriptor)
	if err != nil {
		return nil, err
	}
	return securePrivateHandle(handle, path, descriptor)
}
