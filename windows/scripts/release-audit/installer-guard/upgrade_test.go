//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func digest(b string) string { h := sha256.Sum256([]byte(b)); return hex.EncodeToString(h[:]) }

func TestUpgradeResumesEveryInterruptedTransition(t *testing.T) {
	for stop := 0; stop < 13; stop++ {
		t.Run(string(rune('A'+stop)), func(t *testing.T) {
			root, source := t.TempDir(), t.TempDir()
			old, newFiles := map[string]string{}, map[string]string{}
			for _, name := range []string{"Planet Windows.exe", "ipfs.exe", "Uninstall.exe"} {
				os.WriteFile(filepath.Join(root, name), []byte("old:"+name), 0600)
				os.WriteFile(filepath.Join(source, name), []byte("new:"+name), 0600)
				old[name] = digest("old:" + name)
				newFiles[name] = digest("new:" + name)
			}
			os.WriteFile(filepath.Join(root, "custom.txt"), []byte("private unknown"), 0600)
			commits := 0
			j := upgradeJournal{Format: 1, Old: old, New: newFiles, Version: "0.2.0"}
			step := 0
			injected := errors.New("interrupted")
			err := applyPayload(root, source, j, func() error { commits++; return nil }, func() error {
				step++
				if step == stop {
					return injected
				}
				return nil
			})
			if err != nil && !errors.Is(err, injected) {
				t.Fatal(err)
			}
			if err != nil {
				if err = applyPayload(root, source, j, func() error { commits++; return nil }, nil); err != nil {
					t.Fatal(err)
				}
			}
			for name, want := range newFiles {
				got, e := hashFile(filepath.Join(root, name), false)
				if e != nil || got != want {
					filepath.Walk(root, func(p string, i os.FileInfo, e error) error { t.Log(p); return nil })
					t.Fatalf("new payload %s: %s %v", name, got, e)
				}
			}
			if _, e := os.Stat(filepath.Join(root, ".planet-update")); !os.IsNotExist(e) {
				t.Fatalf("pending update remains: %v", e)
			}
			b, _ := os.ReadFile(filepath.Join(root, "custom.txt"))
			if string(b) != "private unknown" {
				t.Fatal("unknown file changed")
			}
			if commits < 1 {
				t.Fatal("registration not committed")
			}
		})
	}
}

func TestUpgradeRejectsChangedLockedAndForeignPaths(t *testing.T) {
	for _, kind := range []string{"changed", "missing", "locked", "collision"} {
		t.Run(kind, func(t *testing.T) {
			root, source := t.TempDir(), t.TempDir()
			name := "Planet Windows.exe"
			os.WriteFile(filepath.Join(root, name), []byte("old"), 0600)
			os.WriteFile(filepath.Join(source, name), []byte("new"), 0600)
			switch kind {
			case "changed":
				os.WriteFile(filepath.Join(root, name), []byte("user modified"), 0600)
			case "missing":
				os.Remove(filepath.Join(root, name))
			case "locked":
				f, e := openOwned(filepath.Join(root, name), digest("old"))
				if e != nil {
					t.Fatal(e)
				}
				defer f.Close()
			case "collision":
				os.Mkdir(filepath.Join(root, ".planet-update"), 0700)
				os.WriteFile(filepath.Join(root, ".planet-update", "unknown"), []byte("keep"), 0600)
			}
			j := upgradeJournal{Format: 1, Old: map[string]string{name: digest("old")}, New: map[string]string{name: digest("new")}, Version: "0.2.0"}
			if e := applyPayload(root, source, j, func() error { t.Fatal("committed rejected update"); return nil }, nil); e == nil {
				t.Fatal("unsafe update accepted")
			}
			if kind == "collision" {
				b, _ := os.ReadFile(filepath.Join(root, ".planet-update", "unknown"))
				if string(b) != "keep" {
					t.Fatal("collision damaged")
				}
			}
		})
	}
}
