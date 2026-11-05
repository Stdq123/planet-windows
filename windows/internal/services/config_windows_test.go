package services

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestConfigPathAcceptsWindowsShortNames(t *testing.T) {
	root := t.TempDir()
	p, err := windows.UTF16PtrFromString(root)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, 32768)
	n, err := windows.GetShortPathName(p, &buf[0], uint32(len(buf)))
	if err != nil {
		t.Fatal(err)
	}
	short := windows.UTF16ToString(buf[:n])
	if strings.EqualFold(short, root) {
		t.Skip("filesystem does not provide short names")
	}
	t.Logf("short root: %s; long root: %s", short, root)
	if err := SaveConfig(short, "site", "filebase", Config{Name: "site", Token: "test-token"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig(short, "site", "filebase")
	if err != nil || loaded.Token != "test-token" {
		t.Fatalf("short-path round trip: %#v, %v", loaded, err)
	}
}

func TestConfigPathRejectsWindowsSymlinkAncestor(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		if errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) {
			t.Skip("creating symlinks requires Windows developer mode or privilege")
		}
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(target, "child"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := configPath(filepath.Join(link, "child"), "site", "filebase", true); err == nil {
		t.Fatal("accepted symlink ancestor")
	}
}

func TestConfigPathRejectsWindowsJunctions(t *testing.T) {
	for _, location := range []string{"root", "ancestor", "integrations", "kind"} {
		t.Run(location, func(t *testing.T) {
			base := t.TempDir()
			target := filepath.Join(base, "target")
			if err := os.MkdirAll(target, 0700); err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(base, "root")
			link := root
			if location != "ancestor" && location != "root" {
				if err := os.Mkdir(root, 0700); err != nil {
					t.Fatal(err)
				}
				link = filepath.Join(root, "integrations")
				if location == "kind" {
					if err := os.Mkdir(link, 0700); err != nil {
						t.Fatal(err)
					}
					link = filepath.Join(link, "filebase")
				}
			}
			if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
				t.Fatalf("create junction: %v: %s", err, out)
			}
			if location == "ancestor" {
				if err := os.Mkdir(filepath.Join(target, "child"), 0700); err != nil {
					t.Fatal(err)
				}
				root = filepath.Join(root, "child")
			}
			if _, err := configPath(root, "site", "filebase", true); err == nil {
				t.Fatal("accepted directory junction")
			}
		})
	}
}
