package ipfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepoControlsRequireOwnedNodeAndPreservePinnedContent(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	c := New(exe, filepath.Join(t.TempDir(), "repo"))
	if _, err := c.RepoSize(); err == nil {
		t.Fatal("repo size started an absent node")
	}
	if err := c.GarbageCollect(); err == nil {
		t.Fatal("GC accepted absent node")
	}
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	cid, err := c.Add(dir)
	if err != nil {
		t.Fatal(err)
	}
	size, err := c.RepoSize()
	if err != nil || size <= 0 {
		t.Fatalf("invalid repo size: %d %v", size, err)
	}
	if err = c.GarbageCollect(); err != nil {
		t.Fatal(err)
	}
	b, err := c.Read(cid, "hello.txt")
	if err != nil || string(b) != "keep" {
		t.Fatalf("pinned data lost: %q %v", b, err)
	}
}
