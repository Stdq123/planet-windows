package sitecleanup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveOwnedDirectoryOnlyTouchesSelectedSite(t *testing.T) {
	root := t.TempDir()
	const site = "11111111-1111-4111-8111-111111111111"
	const other = "22222222-2222-4222-8222-222222222222"
	for _, id := range []string{site, other} {
		dir := filepath.Join(root, "attachments", id, "post")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "file"), []byte(id), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveOwnedDirectory(root, "attachments", site); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "attachments", site)); !os.IsNotExist(err) {
		t.Fatalf("site remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "attachments", other, "post", "file")); err != nil {
		t.Fatalf("other site changed: %v", err)
	}
	if err := RemoveOwnedDirectory(root, "attachments", site); err != nil {
		t.Fatal("cleanup should be idempotent", err)
	}
}

func TestRemoveOwnedDirectoryRejectsUnsafePaths(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "keep")
	if err := os.WriteFile(outside, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ kind, id string }{{"attachments", ".."}, {"../keep", "11111111-1111-4111-8111-111111111111"}, {"ipfs", "11111111-1111-4111-8111-111111111111"}} {
		if err := RemoveOwnedDirectory(root, tc.kind, tc.id); err == nil {
			t.Fatalf("unsafe path accepted: %+v", tc)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("outside file changed", err)
	}
}

func TestRemoveOwnedDirectoryRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	const site = "11111111-1111-4111-8111-111111111111"
	if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "imports", site)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := RemoveOwnedDirectory(root, "imports", site); err == nil {
		t.Fatal("site tree containing link was removed")
	}
	if _, err := os.Stat(filepath.Join(outside, "keep")); err != nil {
		t.Fatal("linked data changed", err)
	}
}

func TestKuboRepositoryPresenceDoesNotCreateOrFollowInvalidPaths(t *testing.T) {
	root := t.TempDir()
	exists, err := KuboRepositoryPresent(root)
	if err != nil || exists {
		t.Fatalf("missing repository: %v %v", exists, err)
	}
	if err := os.Mkdir(filepath.Join(root, "ipfs"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := KuboRepositoryPresent(root); err == nil {
		t.Fatal("incomplete repository accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "ipfs", "config"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	exists, err = KuboRepositoryPresent(root)
	if err != nil || !exists {
		t.Fatalf("repository missing: %v %v", exists, err)
	}
}

func TestKuboRepositoryRejectsRedirectedKeystore(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	dir := filepath.Join(root, "ipfs")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "keystore")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := KuboRepositoryPresent(root); err == nil {
		t.Fatal("redirected keystore accepted")
	}
}

func TestOfflineKuboConfigMustUseLoopbackEndpoints(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "ipfs")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config")
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"Addresses":{"API":"/ip4/127.0.0.1/tcp/0","Gateway":"/ip4/127.0.0.1/tcp/0"}}`, true},
		{`{"Addresses":{"API":"/ip4/0.0.0.0/tcp/5001","Gateway":"/ip4/127.0.0.1/tcp/0"}}`, false},
		{`{"Addresses":{"API":"/ip4/127.0.0.1/tcp/0","Gateway":"/ip4/0.0.0.0/tcp/8080"}}`, false},
	} {
		if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
			t.Fatal(err)
		}
		err := CheckOfflineKuboConfig(root)
		if (err == nil) != tc.valid {
			t.Fatalf("config %s: %v", tc.body, err)
		}
	}
}
