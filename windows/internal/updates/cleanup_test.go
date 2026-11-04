package updates

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifiedCleanupPreservesChangedFilesAndRemovesOnlyOwnedBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "setup.exe")
	b := []byte("verified")
	sum := sha256.Sum256(b)
	r := Release{Size: int64(len(b)), SHA256: hex.EncodeToString(sum[:])}
	if e := os.WriteFile(path, []byte("modified"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := RemoveVerifiedDownload(path, r); e == nil {
		t.Fatal("changed file was treated as disposable")
	}
	if got, _ := os.ReadFile(path); string(got) != "modified" {
		t.Fatal("changed file lost")
	}
	if e := os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	if e := RemoveVerifiedDownload(path, r); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("owned cache remained", e)
	}
}
