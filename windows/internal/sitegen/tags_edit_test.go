package sitegen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"windows/internal/store"
)

func TestNativeArticleTagsPopulatePublicSiteIndex(t *testing.T) {
	out := t.TempDir()
	site := store.Site{ID: "SITE", Name: "Site"}
	article := store.Article{ID: "POST", SiteID: "SITE", Title: "Post", Tags: map[string]string{"go": "Go"}}
	other := store.Article{ID: "OTHER", SiteID: "SITE", Title: "Other"}
	if err := Render(out, site, []store.Article{article, other}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "planet.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Tags map[string]string `json:"tags"`
	}
	if err = json.Unmarshal(b, &p); err != nil || p.Tags["go"] != "Go" {
		t.Fatalf("site tags missing: %s %v", b, err)
	}
	for _, name := range []string{"tags.html", "go.html", "POST/index.html", "index.html"} {
		b, err = os.ReadFile(filepath.Join(out, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if name == "go.html" && (!strings.Contains(string(b), "Post") || strings.Contains(string(b), "Other")) {
			t.Fatalf("tag page not filtered: %s", b)
		}
		if (name == "tags.html" || name == "POST/index.html") && !strings.Contains(string(b), "go.html") {
			t.Fatalf("missing tag link in %s: %s", name, b)
		}
		if name == "index.html" && !strings.Contains(string(b), "tags.html") {
			t.Fatalf("missing tag index link: %s", b)
		}
	}
}
