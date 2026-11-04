package sitegen

import (
	"os"
	"path/filepath"
	"testing"

	"windows/internal/store"
)

func TestRendererRejectsUnicodeFoldCollisionsBeforeWriting(t *testing.T) {
	template := t.TempDir()
	if err := os.WriteFile(filepath.Join(template, "template.json"), []byte(`{"name":"Plain","generateTagPages":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	site := store.Site{ID: "SITE", Name: "Site", Tags: map[string]string{"σ": "Sigma", "ς": "Final sigma"}}
	if err := RenderWithTemplate(out, site, nil, template); err == nil {
		t.Fatal("colliding tag names were accepted")
	}
	entries, err := os.ReadDir(out)
	if err != nil || len(entries) != 0 {
		t.Fatalf("renderer wrote unsafe output: %v %v", entries, err)
	}
}
