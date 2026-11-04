package migration

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"windows/internal/ipfs"
	"windows/internal/services"
	"windows/internal/store"
)

func TestNativeBackupIncludesPageAliasAndAvatar(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	root := t.TempDir()
	library, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	node := ipfs.New(exe, filepath.Join(root, "ipfs"))
	if err = node.Init(); err != nil {
		t.Fatal(err)
	}
	site, err := library.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	a, err := library.SaveArticle(site.ID, "", "About", "Hello")
	if err != nil {
		t.Fatal(err)
	}
	a, err = library.UpdateArticleSettings(site.ID, a.ID, "about", 1, "https://example.com/", true, 3)
	if err != nil {
		t.Fatal(err)
	}
	var avatar bytes.Buffer
	if err = png.Encode(&avatar, image.NewRGBA(image.Rect(0, 0, 3, 3))); err != nil {
		t.Fatal(err)
	}
	if _, err = library.SetSiteAvatar(site.ID, base64.StdEncoding.EncodeToString(avatar.Bytes())); err != nil {
		t.Fatal(err)
	}
	if _, err = library.UpdateSiteProfile(site.ID, store.SiteProfile{Domain: "site.example.com", GithubUsername: "alice"}); err != nil {
		t.Fatal(err)
	}
	if _, err = library.UpdatePodcastSettings(site.ID, "zh-CN", true, []string{"Technology"}); err != nil {
		t.Fatal(err)
	}
	if _, err = library.SetPodcastCover(site.ID, base64.StdEncoding.EncodeToString(avatar.Bytes())); err != nil {
		t.Fatal(err)
	}
	if err = services.SaveConfig(root, site.ID, "cloudflare", services.Config{Name: "my-site", Server: "0123456789abcdef0123456789abcdef", Token: "super-secret-token"}); err != nil {
		t.Fatal(err)
	}
	identity, err := node.EnsureKey(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = library.SetIdentity(site.ID, identity); err != nil {
		t.Fatal(err)
	}
	dir, err := New(root, exe, library, node).Export(site.ID, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"avatar.png", "favicon.ico", "podcastCoverArt.png", filepath.Join(a.ID, "index.html"), filepath.Join("about", "index.html")} {
		if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
			t.Fatalf("missing %s: %v", file, err)
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "planet.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("super-secret-token")) || !bytes.Contains(b, []byte("site.example.com")) || !bytes.Contains(b, []byte("Technology")) {
		t.Fatalf("backup omitted public settings or exposed a secret: %s", b)
	}
	var info struct {
		Articles []struct {
			Slug        string `json:"slug"`
			ArticleType int    `json:"articleType"`
			InNav       bool   `json:"isIncludedInNavigation"`
			Weight      int    `json:"navigationWeight"`
		} `json:"articles"`
	}
	if err = json.Unmarshal(b, &info); err != nil || len(info.Articles) != 1 || info.Articles[0].Slug != "about" || info.Articles[0].ArticleType != 1 || !info.Articles[0].InNav || info.Articles[0].Weight != 3 {
		t.Fatalf("settings missing: %s %v", b, err)
	}
}
