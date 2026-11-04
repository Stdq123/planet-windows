//go:build windows

// lock-holder is a disposable test-only process. It takes an exclusive read
// lock and signals readiness by creating the specified marker file.
package main

import (
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func main() {
	if len(os.Args) != 4 {
		os.Exit(2)
	}
	p, err := windows.UTF16PtrFromString(os.Args[1])
	if err != nil {
		os.Exit(3)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		os.Exit(4)
	}
	defer windows.CloseHandle(h)
	if err := os.WriteFile(os.Args[2], []byte("ready"), 0600); err != nil {
		os.Exit(5)
	}
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(os.Args[3]); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	os.Exit(6)
}
