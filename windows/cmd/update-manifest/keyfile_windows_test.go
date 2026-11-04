//go:build windows

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestValidatePrivateDACLRejectsUnprotectedOrAdditionalACEs(t *testing.T) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sid := user.User.Sid.String()
	expected, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + sid + ")")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, sddl string
		allowed    bool
	}{
		{"expected", "D:P(A;;FA;;;" + sid + ")", true},
		{"unprotected", "D:(A;;FA;;;" + sid + ")", false},
		{"extra-everyone", "D:P(A;;FA;;;" + sid + ")(A;;FR;;;WD)", false},
		{"wrong-sid", "D:P(A;;FA;;;WD)", false},
		{"inherited", "D:P(A;ID;FA;;;" + sid + ")", false},
		{"insufficient", "D:P(A;;FR;;;" + sid + ")", false},
		{"null-dacl", "D:NO_ACCESS_CONTROL", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual, e := windows.SecurityDescriptorFromString(tc.sddl)
			if e != nil {
				t.Fatal(e)
			}
			e = validatePrivateDACL(actual, expected)
			if (e == nil) != tc.allowed {
				t.Fatalf("unexpected validation result: %v", e)
			}
		})
	}
}

func TestInsecureCreatedHandleIsNotReturnedAndEmptyFileIsRemoved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unsafe.key")
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	expected, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		t.Fatal(err)
	}
	weak, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	attrs := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: weak}
	handle, err := windows.CreateFile(name, windows.GENERIC_WRITE|windows.DELETE, 0, &attrs, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	file, err := securePrivateHandle(handle, path, expected)
	if err == nil || file != nil {
		if file != nil {
			file.Close()
		}
		t.Fatal("insecure handle accepted")
	}
	if strings.Contains(err.Error(), "may remain") {
		t.Fatalf("handle deletion failed: %v", err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unsafe empty file remains: %v", err)
	}
}

func TestInsecureHandleReportsRemainingEmptyFileWhenDeleteDenied(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unsafe.key")
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	expected, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		t.Fatal(err)
	}
	weak, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	attrs := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: weak}
	handle, err := windows.CreateFile(name, windows.GENERIC_WRITE, 0, &attrs, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	file, err := securePrivateHandle(handle, path, expected)
	if err == nil || file != nil || !strings.Contains(err.Error(), "empty file may remain") || !strings.Contains(err.Error(), path) {
		t.Fatalf("missing residual file warning: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != 0 {
		t.Fatalf("residual file is not empty: %v", err)
	}
}

func TestKeygenCreatesProtectedUserOnlyDACL(t *testing.T) {
	dir := t.TempDir()
	parent, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := parent.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err = windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	expected, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "signing.key")
	var output bytes.Buffer
	if err = run([]string{"keygen", "-out", path}, &output); err != nil {
		t.Fatal(err)
	}
	actual, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := actual.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 || actual.String() != expected.String() {
		t.Fatalf("private key DACL is not protected or user-only: got %s want %s", actual.String(), expected.String())
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = run([]string{"keygen", "-out", path}, &output); err == nil {
		t.Fatal("existing key overwritten")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("existing key changed: %v", err)
	}
	actualAfter, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil || actualAfter.String() != actual.String() {
		t.Fatalf("existing key DACL changed: %v", err)
	}
}

func TestPrivateKeyDACLIsProtectedBeforeAnyWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.key")
	file, err := createPrivateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() != 0 {
		t.Fatalf("file not empty: %v", err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	expected, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := actual.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 || actual.String() != expected.String() {
		t.Fatalf("unsafe DACL before write: %s", actual.String())
	}
}
