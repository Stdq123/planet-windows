package sitegen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"windows/internal/store"
)

func TestRobotsSettingAppliesAndCanBeReversed(t *testing.T) {
	dir := t.TempDir()
	site := store.Site{ID: "SITE", Name: "Example", DoNotIndex: true}
	if err := Render(dir, site, nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "robots.txt"))
	if err != nil || !strings.Contains(string(b), "Disallow: /") {
		t.Fatalf("missing robots restriction: %q %v", b, err)
	}
	site.DoNotIndex = false
	if err := Render(dir, site, nil); err != nil {
		t.Fatal(err)
	}
	b, err = os.ReadFile(filepath.Join(dir, "robots.txt"))
	if err != nil || strings.Contains(string(b), "Disallow: /") {
		t.Fatalf("stale robots restriction: %q %v", b, err)
	}
}
