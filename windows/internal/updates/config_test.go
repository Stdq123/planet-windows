package updates

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestTrustIdentityBindsHighestVersionAndResetsExplicitly(t *testing.T) {
	root := t.TempDir()
	cfg := Config{FeedURL: "https://updates.example/feed.json", PublicKey: base64.StdEncoding.EncodeToString(make([]byte, 32)), AutoCheck: true}
	if _, e := SaveSettings(root, cfg); e != nil {
		t.Fatal(e)
	}
	r := Release{Version: "0.4.0", SHA256: strings.Repeat("a", 64)}
	if e := RecordSeen(root, cfg, r); e != nil {
		t.Fatal(e)
	}
	saved, e := LoadConfig(root)
	if e != nil || saved.HighestVersion != "0.4.0" {
		t.Fatal(saved, e)
	}
	cfg.AutoCheck = false
	if _, e = SaveSettings(root, cfg); e != nil {
		t.Fatal(e)
	}
	saved, _ = LoadConfig(root)
	if saved.HighestVersion != "0.4.0" {
		t.Fatal("checkbox reset replay protection")
	}
	changed := cfg
	changed.FeedURL = "https://new.example/feed.json"
	if _, e = SaveSettings(root, changed); e != nil {
		t.Fatal(e)
	}
	saved, _ = LoadConfig(root)
	if saved.HighestVersion != "" {
		t.Fatal("new source retained old identity history")
	}
	if e = RecordSeen(root, cfg, r); e == nil {
		t.Fatal("stale check changed new trust configuration")
	}
}
