package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("PLANET_TEST_RSYNC_HELPER") == "1" {
		b, _ := json.Marshal(os.Args[1:])
		_ = os.WriteFile(os.Getenv("PLANET_TEST_RSYNC_ARGS"), b, 0600)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestRsyncArgumentsProtectDestinationAndOnlyDeleteWhenEnabled(t *testing.T) {
	args, err := rsyncArgs(`D:\temp\site`, `alice@example.com:/var/www/blog`, `C:\Keys\site key`, false)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, "|")
	if strings.Contains(joined, "--delete") || !strings.Contains(joined, "--protect-args") || !strings.Contains(joined, "StrictHostKeyChecking=yes") || !strings.Contains(joined, "site key") {
		t.Fatalf("invalid arguments: %v", args)
	}
	args, err = rsyncArgs(`D:\temp\site`, `alice@example.com:/var/www/blog`, "", true)
	if err != nil || !strings.Contains(strings.Join(args, "|"), "--delete") {
		t.Fatalf("delete not configured: %v %v", args, err)
	}
	for _, bad := range []string{"alice@example.com:/", "alice@example.com:/var/../etc", "-x@example.com:/var/www", "alice@example.com:/var/www;touch", "alice@example.com:/var/www site"} {
		if _, err = rsyncArgs(`D:\temp\site`, bad, "", true); err == nil {
			t.Errorf("unsafe target accepted: %q", bad)
		}
	}
}

func TestDeployRsyncInvokesInstalledExecutableWithoutShell(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows executable behavior")
	}
	current, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "rsync.exe")
	if err = os.WriteFile(exe, binary, 0700); err != nil {
		t.Fatal(err)
	}
	argsFile := filepath.Join(dir, "arguments.json")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PLANET_TEST_RSYNC_HELPER", "1")
	t.Setenv("PLANET_TEST_RSYNC_ARGS", argsFile)
	if err = DeployRsync(t.TempDir(), "alice@example.com:/var/www/site", "", false); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	if err = json.Unmarshal(b, &args); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, "|")
	if !strings.Contains(joined, "--protect-args") || !strings.Contains(joined, "alice@example.com:/var/www/site") || strings.Contains(joined, "--delete") {
		t.Fatalf("unexpected args: %v", args)
	}
}
