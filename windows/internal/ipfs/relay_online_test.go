package ipfs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The reader disables mDNS and dials the supplied address.
func TestRetrieveFromExplicitPeer(t *testing.T) {
	exe, address, target := os.Getenv("PLANET_TEST_KUBO"), os.Getenv("PLANET_TEST_PEER_ADDR"), os.Getenv("PLANET_TEST_PEER_PATH")
	if exe == "" || address == "" || target == "" {
		t.Skip("set PLANET_TEST_KUBO, PLANET_TEST_PEER_ADDR and PLANET_TEST_PEER_PATH")
	}
	index := strings.LastIndex(address, "/p2p/")
	if index < 0 {
		t.Fatal("PLANET_TEST_PEER_ADDR must contain a peer ID")
	}
	repo := filepath.Join(t.TempDir(), "reader")
	reader := New(exe, repo)
	if err := reader.Init(); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(repo, "config")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	config["Bootstrap"] = []string{}
	config["Discovery"] = map[string]any{"MDNS": map[string]any{"Enabled": false}}
	raw, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := reader.Start(false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	if out, err := reader.runTimeout(45*time.Second, "swarm", "connect", "--timeout=40s", address); err != nil {
		t.Fatalf("peer connection failed: %s: %v", out, err)
	}
	peers, err := reader.run("swarm", "peers")
	peer := address[index+len("/p2p/"):]
	if err != nil || !strings.Contains(peers, "/p2p/"+peer) {
		t.Fatalf("no peer connection: %q: %v", peers, err)
	}
	t.Logf("peer connections: %s", peers)
	content, err := reader.runTimeout(45*time.Second, "cat", "--timeout=40s", target)
	if err != nil || content == "" {
		t.Fatalf("retrieval failed: %v", err)
	}
	t.Logf("retrieved %d bytes from the peer", len(content))
}

// Test retrieval from routing without sharing a repository or mDNS discovery.
func TestRetrievePublicCIDFromIndependentRepo(t *testing.T) {
	exe, target := os.Getenv("PLANET_TEST_KUBO"), os.Getenv("PLANET_TEST_PUBLIC_PATH")
	if exe == "" || target == "" {
		t.Skip("set PLANET_TEST_KUBO and PLANET_TEST_PUBLIC_PATH")
	}
	repo := filepath.Join(t.TempDir(), "reader")
	reader := New(exe, repo)
	if err := reader.Init(); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(repo, "config")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	config["Discovery"] = map[string]any{"MDNS": map[string]any{"Enabled": false}}
	raw, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := reader.Start(false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	content, err := reader.runTimeout(100*time.Second, "cat", "--timeout=90s", target)
	if err != nil || content == "" {
		t.Fatalf("retrieval failed: %v", err)
	}
	t.Logf("retrieved %d bytes from public routing", len(content))
	if peer := os.Getenv("PLANET_TEST_PEER_ID"); peer != "" {
		peers, err := reader.run("swarm", "peers")
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(peers, "\n") {
			if strings.Contains(line, "/p2p/"+peer) {
				t.Logf("provider connection: %s", line)
			}
		}
	}
}
