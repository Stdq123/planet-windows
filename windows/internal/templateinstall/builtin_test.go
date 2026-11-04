package templateinstall

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"windows/internal/store"
)

func TestBuiltinTemplatesCanBeVerifiedInstalledAndPreviewed(t *testing.T) {
	items := Builtins()
	if len(items) != 5 {
		t.Fatalf("expected five built-in templates, got %d", len(items))
	}
	for _, item := range items {
		t.Run(item.ID, func(t *testing.T) {
			root := t.TempDir()
			source, cleanup, err := ExtractBuiltin(item.ID)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			if err := Verify(source); err != nil {
				t.Fatal(err)
			}
			lib, err := store.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			site, err := lib.CreateSite("Example", "")
			if err != nil {
				t.Fatal(err)
			}
			installed, err := Install(root, source, site, nil, lib)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(installed, "hooks")); !os.IsNotExist(err) {
				t.Fatal("unexpected template hook")
			}
			page, err := BuiltinPreview(item.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(page) < 200 {
				t.Fatalf("empty preview: %q", page)
			}
			if !strings.Contains(page, "My Planet") || !strings.Contains(page, "data:text/css;base64,") {
				t.Fatal("preview is missing sample content or embedded stylesheet")
			}
			if regexp.MustCompile(`data:text/css;base64,[A-Za-z0-9+/=]+\?`).MatchString(page) {
				t.Fatal("stylesheet query was appended to data URL")
			}
		})
	}
	if _, _, err := ExtractBuiltin("../plain"); err == nil {
		t.Fatal("invalid template ID accepted")
	}
}
