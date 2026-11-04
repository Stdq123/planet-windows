//go:build windows

// This is an isolated operating-system probe, not an installer implementation.
package renameprobe

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

// FILE_RENAME_INFO layout for the supported Windows amd64 release target.
func renameExclusive(h windows.Handle, destination string) error {
	name := utf16.Encode([]rune(destination))
	buf := make([]byte, 20+len(name)*2)
	binary.LittleEndian.PutUint32(buf[16:20], uint32(len(name)*2))
	for i, c := range name {
		binary.LittleEndian.PutUint16(buf[20+i*2:], c)
	}
	return windows.SetFileInformationByHandle(h, windows.FileRenameInfo, &buf[0], uint32(len(buf)))
}

func TestExclusiveHandleRenameDoesNotReplaceExistingFile(t *testing.T) {
	root, err := os.MkdirTemp("", "planet-rename-probe-")
	if err != nil {
		t.Fatal(err)
	}
	// This Wine 8 image returns "Invalid function" during Go's cleanup after
	// this rename. Its temporary prefix is discarded with the container.
	if os.Getenv("PLANET_DISPOSABLE_CONTAINER") != "yes" {
		t.Cleanup(func() {
			if err := os.RemoveAll(root); err != nil {
				t.Errorf("clean temporary probe directory: %v", err)
			}
		})
	}
	source := filepath.Join(root, "original.lnk")
	conflict := filepath.Join(root, "foreign.lnk")
	backup := filepath.Join(root, "backup.lnk")
	if err := os.WriteFile(source, []byte("owned shortcut"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conflict, []byte("foreign shortcut"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := windows.UTF16PtrFromString(source)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|0x10000, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if h != 0 {
			_ = windows.CloseHandle(h)
		}
	}()
	if err := renameExclusive(h, conflict); err == nil {
		t.Fatal("renamed over an unrelated existing file")
	}
	if data, err := os.ReadFile(conflict); err != nil || string(data) != "foreign shortcut" {
		t.Fatalf("existing file was changed: %q %v", data, err)
	}
	if err := renameExclusive(h, backup); err != nil {
		t.Fatalf("rename with exclusive handle: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("old name still exists: %v", err)
	}
	if err := windows.CloseHandle(h); err != nil {
		t.Fatal(err)
	}
	h = 0
	if data, err := os.ReadFile(backup); err != nil || string(data) != "owned shortcut" {
		t.Fatalf("backup changed: %q %v", data, err)
	}
}
