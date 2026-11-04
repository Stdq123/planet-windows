//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestRemoveMatchingFilePreservesChangedAndRemovesOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shortcut.lnk")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("original"))
	expected := hex.EncodeToString(digest[:])
	if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := removeMatchingFile(path, expected); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "changed" {
		t.Fatalf("changed file removed: %q %v", data, err)
	}
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := removeMatchingFile(path, expected); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("original still exists: %v", err)
	}
}

func TestRejectsJunctionInAncestor(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(root, "junction")
	if out, err := exec.Command("cmd.exe", "/c", "mklink", "/J", junction, target).CombinedOutput(); err != nil {
		t.Fatalf("create junction: %s %v", out, err)
	}
	t.Cleanup(func() { _ = os.Remove(junction) })
	if _, err := safeDirectory(filepath.Join(junction, "new")); err == nil {
		t.Fatal("accepted junction ancestor")
	}
}

func TestHashFileRejectsExclusiveLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owned.txt")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	if _, err := hashFile(path, true); err == nil {
		t.Fatal("accepted locked file")
	}
}
