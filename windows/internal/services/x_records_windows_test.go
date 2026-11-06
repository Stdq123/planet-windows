package services

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestXPostRecordsRefuseDirectoryJunction(t *testing.T) {
	base := t.TempDir()
	root, outside := filepath.Join(base, "root"), filepath.Join(base, "outside")
	if err := os.MkdirAll(filepath.Join(root, "integrations"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(outside, "keep.dat")
	if err := os.WriteFile(marker, []byte("outside data"), 0600); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(root, "integrations", "x-post")
	if output, err := exec.Command("cmd", "/c", "mklink", "/J", junction, outside).CombinedOutput(); err != nil {
		t.Fatalf("create junction: %v: %s", err, output)
	}
	if err := SaveXPost(root, "site-1", "article-1", XPostRecord{Status: "pending"}); err == nil {
		t.Fatal("saved through junction")
	}
	if _, err := LoadXPost(root, "site-1", "article-1"); err == nil {
		t.Fatal("loaded through junction")
	}
	if err := DeleteXPostsForSite(root, "site-1"); err == nil {
		t.Fatal("deleted through junction")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "outside data" {
		t.Fatalf("outside data modified: %v", err)
	}
}
