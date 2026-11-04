package ipfs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOrdinaryKeyOperationsRejectRedirectedKeystore(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	root := t.TempDir()
	local := New(exe, filepath.Join(root, "local"))
	foreign := New(exe, filepath.Join(root, "foreign"))
	if err := local.Init(); err != nil {
		t.Fatal(err)
	}
	if err := foreign.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := foreign.EnsureKey("source"); err != nil {
		t.Fatal(err)
	}
	keyfile := filepath.Join(root, "key.bin")
	if err := foreign.ExportKey("source", keyfile); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(root, "local", "keystore")
	if err := os.Remove(junction); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if out, err := exec.Command("cmd.exe", "/c", "mklink", "/J", junction, filepath.Join(root, "foreign", "keystore")).CombinedOutput(); err != nil {
		t.Fatalf("create junction: %s %v", out, err)
	}
	t.Cleanup(func() { _ = os.Remove(junction) })
	if _, err := local.EnsureKey("wrong"); err == nil {
		t.Fatal("key generation accepted redirected keystore")
	}
	if _, err := local.ImportKey("wrong", keyfile); err == nil {
		t.Fatal("key import accepted redirected keystore")
	}
	if err := local.Init(); err == nil {
		t.Fatal("init accepted redirected keystore")
	}
	if key, err := foreign.LookupKey("wrong"); err != nil || key != "" {
		t.Fatalf("foreign keystore changed: %s %v", key, err)
	}
}
