package sitegen

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"windows/internal/store"
)

func TestLocalSiteAvatarRenderedForTemplateAndFavicon(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	avatar := base64.StdEncoding.EncodeToString(b.Bytes())
	site := store.Site{ID: "SITE", Name: "Site", AvatarPNG: avatar}
	articles := []store.Article{{ID: "POST", SiteID: "SITE", Title: "Post", Tags: map[string]string{"go": "Go"}}}
	dir := t.TempDir()
	if err := Render(dir, site, articles); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"avatar.png", "favicon.ico"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil || !strings.Contains(string(index), `src="avatar.png"`) {
		t.Fatalf("avatar missing in index: %s %v", index, err)
	}
	if !strings.Contains(string(index), `href="favicon.ico"`) {
		t.Fatalf("favicon missing from home: %s", index)
	}
	page, err := os.ReadFile(filepath.Join(dir, "POST", "index.html"))
	if err != nil || !strings.Contains(string(page), `href="../favicon.ico"`) {
		t.Fatalf("favicon missing from article: %s %v", page, err)
	}
	metadata, err := os.ReadFile(filepath.Join(dir, "planet.json"))
	if err != nil || strings.Contains(string(metadata), avatar) {
		t.Fatalf("avatar data leaked in metadata: %s %v", metadata, err)
	}
}

func TestNativeSiteIdentityIsInPublicMetadata(t *testing.T) {
	dir := t.TempDir()
	site := store.Site{ID: "S", Name: "Site", Slug: "my-site", AuthorName: "Ada"}
	if err := Render(dir, site, nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "planet.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"slug": "my-site"`) || !strings.Contains(string(b), `"authorName": "Ada"`) {
		t.Fatalf("identity missing from public metadata: %s", b)
	}
}
