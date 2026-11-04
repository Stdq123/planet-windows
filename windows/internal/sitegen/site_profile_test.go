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
	"time"
	"windows/internal/store"
)

func TestSiteProfileAppearsInRSSAndPublicMetadata(t *testing.T) {
	dir := t.TempDir()
	site := store.Site{ID: "S", Name: "Site", Domain: "example.eth", GithubUsername: "alice-smith", TwitterUsername: "alice", DiscordLink: "https://discord.gg/xyz"}
	article := store.Article{ID: "POST", SiteID: "S", Title: "Post", Created: time.Now()}
	if err := Render(dir, site, []store.Article{article}); err != nil {
		t.Fatal(err)
	}
	rss, err := os.ReadFile(filepath.Join(dir, "rss.xml"))
	if err != nil || !strings.Contains(string(rss), "https://example.eth.limo/POST/") {
		t.Fatalf("wrong RSS base: %s %v", rss, err)
	}
	meta, err := os.ReadFile(filepath.Join(dir, "planet.json"))
	if err != nil || !strings.Contains(string(meta), `"githubUsername": "alice-smith"`) {
		t.Fatalf("profile missing: %s %v", meta, err)
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil || !strings.Contains(string(index), "https://github.com/alice-smith") {
		t.Fatalf("social link missing: %s %v", index, err)
	}
}

func TestVerifiedTemplatesRenderSocialLinksAsWorkingHTMLURLs(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("verified templates unavailable")
	}
	for _, name := range []string{"plain", "gamedb", "grid", "sepia"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			site := store.Site{ID: "S", Name: "Site", GithubUsername: "alice-smith", DiscordLink: "https://discord.gg/hello-world"}
			if err := RenderWithTemplate(dir, site, nil, filepath.Join(root, name)); err != nil {
				t.Fatal(err)
			}
			page, err := os.ReadFile(filepath.Join(dir, "index.html"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(page), `href="https://github.com/alice-smith"`) {
				t.Fatalf("broken GitHub link in %s", name)
			}
			if name != "gamedb" && name != "grid" && !strings.Contains(string(page), `href="https://discord.gg/hello-world"`) {
				t.Fatalf("broken Discord link in %s", name)
			}
		})
	}
}

func TestVerifiedTemplatesExposeAllConfiguredSocialLinks(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("verified templates unavailable")
	}
	for _, name := range []string{"plain", "gamedb", "grid", "platinum", "sepia"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			site := store.Site{ID: "S", Name: "Site", GithubUsername: "alice", MastodonUsername: "@alice@social.example", DiscordLink: "https://discord.gg/xyz"}
			if err := RenderWithTemplate(dir, site, nil, filepath.Join(root, name)); err != nil {
				t.Fatal(err)
			}
			page, err := os.ReadFile(filepath.Join(dir, "index.html"))
			if err != nil {
				t.Fatal(err)
			}
			for _, link := range []string{"https://github.com/alice", "https://social.example/@alice", "https://discord.gg/xyz"} {
				if !strings.Contains(string(page), `href="`+link+`"`) {
					t.Errorf("missing %s in %s", link, name)
				}
			}
		})
	}
}

func TestCustomDomainAppliesToTemplateSocialImage(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("verified templates unavailable")
	}
	dir := t.TempDir()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	site := store.Site{ID: "S", Name: "Site", Domain: "site.example.com", IPNS: "k51test", AvatarPNG: base64.StdEncoding.EncodeToString(encoded.Bytes())}
	if err := RenderWithTemplate(dir, site, nil, filepath.Join(root, "sepia")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil || !strings.Contains(string(b), "https://site.example.com/avatar.png") {
		t.Fatalf("wrong social image: %v", err)
	}
}
