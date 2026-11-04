package sitecleanup

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestKuboRepositoryRejectsWindowsJunction(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	repo := filepath.Join(root, "ipfs")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "config"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(repo, "keystore")
	if out, err := exec.Command("cmd.exe", "/c", "mklink", "/J", junction, outside).CombinedOutput(); err != nil {
		t.Fatalf("create test junction: %s %v", out, err)
	}
	t.Cleanup(func() { _ = os.Remove(junction) })
	if _, err := KuboRepositoryPresent(root); err == nil {
		t.Fatal("junction accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "keep")); err != nil {
		t.Fatal("junction target changed", err)
	}
}
