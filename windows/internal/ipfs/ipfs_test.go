package ipfs

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitPreservesExistingDNSAndUnknownConfig(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	repo := filepath.Join(t.TempDir(), "repo")
	c := New(exe, repo)
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, "config")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err = json.Unmarshal(b, &config); err != nil {
		t.Fatal(err)
	}
	config["FutureSetting"] = map[string]any{"keep": "value"}
	dns := config["DNS"].(map[string]any)
	dns["Resolvers"] = map[string]any{"eth.": "https://example.test/dns-query", "custom.": "https://example.test/custom"}
	b, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err = c.Init(); err != nil {
		t.Fatal(err)
	}
	b, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &config); err != nil {
		t.Fatal(err)
	}
	if config["FutureSetting"].(map[string]any)["keep"] != "value" {
		t.Fatal("unknown config lost")
	}
	resolvers := config["DNS"].(map[string]any)["Resolvers"].(map[string]any)
	if resolvers["eth."] != "https://example.test/dns-query" || resolvers["custom."] != "https://example.test/custom" {
		t.Fatalf("resolvers changed: %+v", resolvers)
	}
}

func TestInitEnablesIPv4AndIPv6TCPAndQUICListeners(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	repo := filepath.Join(t.TempDir(), "repo")
	client := New(exe, repo)
	if err := client.Init(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(repo, "config"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Addresses struct {
			Swarm []string `json:"Swarm"`
		} `json:"Addresses"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"/ip4/0.0.0.0/tcp/0": false, "/ip6/::/tcp/0": false, "/ip4/0.0.0.0/udp/0/quic-v1": false, "/ip6/::/udp/0/quic-v1": false}
	for _, addr := range config.Addresses.Swarm {
		if _, ok := want[addr]; ok {
			want[addr] = true
		}
	}
	for addr, found := range want {
		if !found {
			t.Errorf("missing swarm listener %s", addr)
		}
	}
}

func TestRemoveKeyIfMatchesPreservesOtherOrUnexpectedKeys(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	c := New(exe, filepath.Join(t.TempDir(), "ipfs"))
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	id, err := c.EnsureKey("site-one")
	if err != nil {
		t.Fatal(err)
	}
	other, err := c.EnsureKey("site-two")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveKeyIfMatches("site-one", id); err == nil {
		t.Fatal("offline key deletion accepted without owned daemon")
	}
	if err := c.Start(true); err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.RemoveKeyIfMatches("site-one", other); err == nil {
		t.Fatal("mismatched key deleted")
	}
	if err := c.RemoveKeyIfMatches("site-one", ""); err == nil {
		t.Fatal("unverified key deleted")
	}
	if got, err := c.LookupKey("site-one"); err != nil || got != id {
		t.Fatalf("key changed: %s %v", got, err)
	}
	if err := c.RemoveKeyIfMatches("site-one", id); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveKeyIfMatches("site-one", id); err != nil {
		t.Fatal("retry failed", err)
	}
	if got, err := c.LookupKey("site-two"); err != nil || got != other {
		t.Fatalf("other key changed: %s %v", got, err)
	}
}

func TestCloseNeverShutsDownForeignDaemonFromTamperedAPIFile(t *testing.T) {
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
	if err := local.Start(true); err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	if err := foreign.Start(true); err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	foreignAPI, err := os.ReadFile(filepath.Join(root, "foreign", "api"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "local", "api"), foreignAPI, 0600); err != nil {
		t.Fatal(err)
	}
	_ = local.Close()
	if !foreign.Alive() {
		t.Fatal("foreign daemon was shut down")
	}
	if _, err = foreign.EnsureKey("still-running"); err != nil {
		t.Fatalf("foreign daemon unavailable: %v", err)
	}
}

func TestExplicitOwnedAPIKeyCommandIgnoresReplacedRepositoryAPIFile(t *testing.T) {
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
	identity, err := foreign.EnsureKey("site")
	if err != nil {
		t.Fatal(err)
	}
	keyfile := filepath.Join(root, "key.bin")
	if err = foreign.ExportKey("site", keyfile); err != nil {
		t.Fatal(err)
	}
	if _, err = local.ImportKey("site", keyfile); err != nil {
		t.Fatal(err)
	}
	if err = foreign.Start(true); err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	if err = local.Start(true); err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	api, err := os.ReadFile(filepath.Join(root, "foreign", "api"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "local", "api"), api, 0600); err != nil {
		t.Fatal(err)
	}
	local.mu.Lock()
	_, err = local.run(local.ownedAPIArgs("key", "rm", "site")...)
	local.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if key, err := foreign.LookupKey("site"); err != nil || key != identity {
		t.Fatalf("foreign key changed: %s %v", key, err)
	}
	if key, err := local.LookupKey("site"); err != nil || key != "" {
		t.Fatalf("local key retained: %s %v", key, err)
	}
}

func TestOfflineCommandsIgnoreForeignRepositoryAPIFile(t *testing.T) {
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
	if err := foreign.Start(true); err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	api, err := os.ReadFile(filepath.Join(root, "foreign", "api"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "local", "api"), api, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = local.EnsureKey("local-only"); err == nil {
		t.Fatal("foreign API was accepted")
	}
	if key, err := foreign.LookupKey("local-only"); err != nil || key != "" {
		t.Fatalf("foreign repo changed: %s %v", key, err)
	}
	if err = os.Remove(filepath.Join(root, "local", "api")); err != nil {
		t.Fatal(err)
	}
	if _, err = local.EnsureKey("local-only"); err != nil {
		t.Fatal(err)
	}
	if key, err := local.LookupKey("local-only"); err != nil || key == "" {
		t.Fatalf("local repo unchanged: %s %v", key, err)
	}
}

func TestLocalKeyImportExportCLIRejectsForeignAPI(t *testing.T) {
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
	if _, err := foreign.EnsureKey("site"); err != nil {
		t.Fatal(err)
	}
	keyfile := filepath.Join(root, "key.bin")
	exportCmd := exec.Command(exe, "--offline", "key", "export", "site", "-o", keyfile)
	foreign.configure(exportCmd)
	if out, err := exportCmd.CombinedOutput(); err != nil {
		t.Fatalf("prepare key: %s %v", out, err)
	}
	if err := foreign.Start(true); err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	api, err := os.ReadFile(filepath.Join(root, "foreign", "api"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "local", "api"), api, 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out.bin")
	cmd := exec.Command(exe, "--offline", "key", "export", "site", "-o", out)
	local.configure(cmd)
	if data, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("remote export unexpectedly succeeded: %s", data)
	}
	if _, err := local.ImportKey("another", keyfile); err == nil {
		t.Fatal("import accepted foreign API")
	}
	if key, err := foreign.LookupKey("another"); err != nil || key != "" {
		t.Fatalf("foreign repo changed: %s %v", key, err)
	}
}

func TestKeyExportFromRunningDaemonRestartsItAndKeepsIdentity(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	root := t.TempDir()
	node := New(exe, filepath.Join(root, "repo"))
	if err := node.Init(); err != nil {
		t.Fatal(err)
	}
	identity, err := node.EnsureKey("site")
	if err != nil {
		t.Fatal(err)
	}
	if err = node.Start(true); err != nil {
		t.Fatal(err)
	}
	defer node.Close()
	if err = node.ExportKey("site", filepath.Join(root, "key.bin")); err != nil {
		t.Fatal(err)
	}
	if !node.Alive() {
		t.Fatal("owned daemon not restarted")
	}
	if got, err := node.LookupKey("site"); err != nil || got != identity {
		t.Fatalf("identity changed: %s %v", got, err)
	}
}

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
	t.Cleanup(func() { _ = client.Close() })
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
	if got, err := client.run("pin", "ls", "--type=recursive", cid); err != nil || got != cid+" recursive" {
		t.Fatalf("site directory is not recursively pinned: %q: %v", got, err)
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
	listeners, err := first.run("swarm", "addrs", "listen")
	if err != nil || !strings.Contains(listeners, "/udp/") || !strings.Contains(listeners, "/quic-v1") {
		t.Fatalf("QUIC listener not active: %q: %v", listeners, err)
	}
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

func TestResolveReadAndPinFromOfflineRepository(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	c := New(exe, filepath.Join(t.TempDir(), "repo"))
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.EnsureKey("test-site"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "planet.json"), []byte(`{"name":"offline"}`), 0600)
	cid, err := c.Add(dir)
	if err != nil {
		t.Fatal(err)
	}
	name, err := c.PublishOffline("test-site", cid)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Start(true); err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	resolved, err := c.Resolve(name)
	if err != nil || resolved != cid {
		t.Fatalf("resolved=%s err=%v", resolved, err)
	}
	data, err := c.Read(cid, "planet.json")
	if err != nil || string(data) != `{"name":"offline"}` {
		t.Fatalf("data=%s err=%v", data, err)
	}
	if _, err = c.run("pin", "rm", cid); err != nil {
		t.Fatal(err)
	}
	if err = c.Pin(cid); err != nil {
		t.Fatal(err)
	}
	if _, err = c.run("repo", "gc"); err != nil {
		t.Fatal(err)
	}
	if data, err = c.Read(cid, "planet.json"); err != nil || string(data) != `{"name":"offline"}` {
		t.Fatalf("pinned data lost after GC: %s %v", data, err)
	}
	if _, err = c.Read(cid, "../planet.key"); err == nil {
		t.Fatal("path traversal accepted")
	}
}

func TestResolveAndReadIPNSContentSubdirectoryOffline(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	c := New(exe, filepath.Join(t.TempDir(), "repo"))
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.EnsureKey("site"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "nested", "site"), 0700)
	os.WriteFile(filepath.Join(dir, "nested", "site", "planet.json"), []byte(`{"name":"Nested"}`), 0600)
	cid, err := c.Add(dir)
	if err != nil {
		t.Fatal(err)
	}
	name, err := c.run("--offline", "name", "publish", "--allow-offline", "--quieter", "--key=site", "/ipfs/"+cid+"/nested/site")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Start(true); err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	path, err := c.Resolve(name)
	if err != nil || path != cid+"/nested/site" {
		t.Fatalf("path=%q err=%v", path, err)
	}
	b, err := c.Read(cid, "nested/site/planet.json")
	if err != nil || string(b) != `{"name":"Nested"}` {
		t.Fatalf("data=%q err=%v", b, err)
	}
}
