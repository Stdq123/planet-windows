//go:build windows

package main

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestParentLockTransferKeepsExactObjectAlive(t *testing.T) {
	if os.Getenv("PLANET_PARENT_LOCK_CHILD") == "yes" {
		pid, _ := strconv.ParseUint(os.Getenv("PLANET_PARENT_PID"), 10, 32)
		value, _ := strconv.ParseUint(os.Getenv("PLANET_PARENT_HANDLE"), 10, 64)
		held, e := retainInstallerLock(uint32(pid), windows.Handle(value))
		if e != nil {
			t.Fatal(e)
		}
		defer windows.CloseHandle(held)
		ready := os.Getenv("PLANET_PARENT_READY")
		if e = os.WriteFile(ready, []byte("ready"), 0600); e != nil {
			t.Fatal(e)
		}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if _, e = os.Stat(ready + ".release"); e == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("parent transfer test timed out")
		return
	}
	root := t.TempDir()
	ready := filepath.Join(root, "ready")
	name, _ := windows.UTF16PtrFromString(`Local\PlanetWindows-Test-ParentLock-` + strconv.Itoa(os.Getpid()))
	held, e := windows.CreateMutex(nil, false, name)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if held != 0 {
			windows.CloseHandle(held)
		}
	}()
	cmd := exec.Command(os.Args[0], "-test.run=^TestParentLockTransferKeepsExactObjectAlive$")
	cmd.Env = append(os.Environ(), "PLANET_PARENT_LOCK_CHILD=yes", "PLANET_PARENT_PID="+strconv.Itoa(os.Getpid()), "PLANET_PARENT_HANDLE="+strconv.FormatUint(uint64(held), 10), "PLANET_PARENT_READY="+ready)
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { os.WriteFile(ready+".release", []byte("done"), 0600); cmd.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, e = os.Stat(ready); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not retain parent lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	windows.CloseHandle(held)
	held = 0
	probe, e := windows.CreateMutex(nil, false, name)
	if probe != 0 {
		windows.CloseHandle(probe)
	}
	if !errors.Is(e, windows.ERROR_ALREADY_EXISTS) {
		t.Fatalf("child failed to preserve original kernel object: %v", e)
	}
	if wrong, e := retainInstallerLock(uint32(os.Getpid()), probe); e == nil {
		windows.CloseHandle(wrong)
		t.Fatal("unrelated parent identity accepted")
	}
}

func TestNSISPointerWireFormat(t *testing.T) {
	for _, s := range []string{"140", "0x0000008c", "0X8C"} {
		h, e := parseMutexHandle(s)
		if e != nil || h != 140 {
			t.Fatalf("NSIS handle %s: %d %v", s, h, e)
		}
	}
}
