package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPendingUpgradeBlocksApplicationBeforeLibraryOpen(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "Planet Windows.exe")
	if e := checkUpdateReady(exe); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(filepath.Join(root, ".planet-update"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := checkUpdateReady(exe); e == nil {
		t.Fatal("pending upgrade allowed startup")
	}
}

func TestApplicationAndInstallerShareExclusiveLifetimeLock(t *testing.T) {
	release, e := acquireUpdateLock()
	if e != nil {
		t.Fatal(e)
	}
	if second, e := acquireUpdateLock(); e == nil {
		second()
		release()
		t.Fatal("simultaneous instance was allowed")
	}
	release()
	again, e := acquireUpdateLock()
	if e != nil {
		t.Fatal(e)
	}
	again()
}
