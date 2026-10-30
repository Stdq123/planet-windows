package ipfs

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPublishOnlineRejectsUnsafeInput(t *testing.T) {
	client := New("missing", filepath.Join(t.TempDir(), "repo"))
	if _, err := client.PublishOnline("../bad", "bafy123"); err == nil {
		t.Fatal("expected invalid key rejection")
	}
}

func TestPublishRejectsExitedDaemon(t *testing.T) {
	exited := make(chan error, 1)
	exited <- errors.New("stopped")
	client := &Client{process: &exec.Cmd{}, exited: exited}
	if _, err := client.PublishOnline("site", "bafy123"); err == nil || err.Error() != "Kubo daemon is not running" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOfflinePublishRoundTrip(t *testing.T) {
	executable := os.Getenv("PLANET_TEST_KUBO")
	if executable == "" {
		t.Skip("set PLANET_TEST_KUBO to run the real Kubo integration test")
	}
	client := New(executable, filepath.Join(t.TempDir(), "repo"))
	if err := client.Init(); err != nil {
		t.Fatal(err)
	}
	key, err := client.EnsureKey("test-site")
	if err != nil {
		t.Fatal(err)
	}
	same, err := client.EnsureKey("test-site")
	if err != nil || key != same {
		t.Fatalf("key changed: %q -> %q: %v", key, same, err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("test site"), 0600); err != nil {
		t.Fatal(err)
	}
	cid, err := client.Add(dir)
	if err != nil {
		t.Fatal(err)
	}
	name, err := client.PublishOffline("test-site", cid)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := client.ResolveOffline(name)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != "/ipfs/"+cid {
		t.Fatalf("resolved %q; want /ipfs/%s", resolved, cid)
	}
}

func TestIsolatedDaemonStartsAndStops(t *testing.T) {
	executable := os.Getenv("PLANET_TEST_KUBO")
	if executable == "" {
		t.Skip("set PLANET_TEST_KUBO to run the real Kubo integration test")
	}
	client := New(executable, filepath.Join(t.TempDir(), "repo"))
	if err := client.Init(); err != nil {
		t.Fatal(err)
	}
	if err := client.Start(true); err != nil {
		t.Fatal(err)
	}
	key, err := client.EnsureKey("daemon-test")
	if err != nil || key == "" {
		t.Fatalf("key=%q err=%v", key, err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("daemon test"), 0600); err != nil {
		t.Fatal(err)
	}
	cid, err := client.Add(dir)
	if err != nil {
		t.Fatal(err)
	}
	name, err := client.PublishOffline("daemon-test", cid)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := client.ResolveOffline(name)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != "/ipfs/"+cid {
		t.Fatalf("resolved %q; want /ipfs/%s", resolved, cid)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPublicPublishFromIndependentRepository(t *testing.T) {
	if os.Getenv("PLANET_TEST_ONLINE") != "1" {
		t.Skip("public network integration is opt-in")
	}
	executable := os.Getenv("PLANET_TEST_KUBO")
	if executable == "" {
		t.Fatal("PLANET_TEST_KUBO is required")
	}
	first := New(executable, filepath.Join(t.TempDir(), "publisher"))
	if err := first.Init(); err != nil {
		t.Fatal(err)
	}
	if err := first.Start(false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second := New(executable, filepath.Join(t.TempDir(), "reader"))
	if err := second.Init(); err != nil {
		t.Fatal(err)
	}
	if err := second.Start(false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	if _, err := first.EnsureKey("test-site"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("Planet Windows public network test"), 0600); err != nil {
		t.Fatal(err)
	}
	cid, err := first.Add(dir)
	if err != nil {
		t.Fatal(err)
	}
	name, err := first.PublishOnline("test-site", cid)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("test IPNS: %s, CID: %s", name, cid)
	resolved, err := second.run("name", "resolve", "--nocache", "/ipns/"+name)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != "/ipfs/"+cid {
		t.Fatalf("resolved %q; want /ipfs/%s", resolved, cid)
	}
	content, err := second.run("cat", "/ipfs/"+cid+"/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if content != "Planet Windows public network test" {
		t.Fatalf("content = %q", content)
	}
}

func TestKeyExportImportKeepsIdentity(t *testing.T) {
	executable := os.Getenv("PLANET_TEST_KUBO")
	if executable == "" {
		t.Skip("Kubo required")
	}
	source := New(executable, filepath.Join(t.TempDir(), "source"))
	if err := source.Init(); err != nil {
		t.Fatal(err)
	}
	identity, err := source.EnsureKey("site")
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(t.TempDir(), "planet.key")
	if err := source.ExportKey("site", keyFile); err != nil {
		t.Fatal(err)
	}
	destination := New(executable, filepath.Join(t.TempDir(), "dest"))
	if err := destination.Init(); err != nil {
		t.Fatal(err)
	}
	imported, err := destination.ImportKey("site", keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if imported != identity {
		t.Fatalf("key identity changed: %q != %q", imported, identity)
	}
	match, err := destination.KeyMatches("site", identity)
	if err != nil || !match {
		t.Fatalf("matching key rejected: %v", err)
	}
	match, err = destination.KeyMatches("site", "not-the-key")
	if err != nil || match {
		t.Fatalf("wrong key accepted: %v", err)
	}
}
