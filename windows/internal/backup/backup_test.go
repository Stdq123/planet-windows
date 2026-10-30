package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"windows/internal/store"
)

const fixture = `{"id":"11111111-1111-4111-8111-111111111111","name":"站点","about":"About","ipns":"k51example","created":10,"updated":12,"templateName":"Plain","unknownField":{"value":42},"articles":[{"id":"22222222-2222-4222-8222-222222222222","link":"/22222222-2222-4222-8222-222222222222/","title":"Title","content":"Body","created":11,"starType":0,"unknownArticle":"preserve"}]}`

func TestMergeUsesCurrentPublicationState(t *testing.T) {
	published := time.Unix(978307300, 0).UTC()
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "站点", About: "About", IPNS: "k51example", Created: time.Unix(978307210, 0), LastPublishedCID: "new-cid", LastPublished: &published}
	article := store.Article{ID: "22222222-2222-4222-8222-222222222222", SiteID: site.ID, Title: "Title", Content: "Body", Created: time.Unix(978307211, 0), Modified: time.Unix(978307211, 0)}
	input := strings.Replace(fixture, `"templateName":"Plain"`, `"templateName":"Plain","lastPublished":12,"lastPublishedCID":"old-cid"`, 1)
	b, err := Merge([]byte(input), site, []store.Article{article})
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err = json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out["lastPublishedCID"] != "new-cid" || out["lastPublished"] != float64(100) {
		t.Fatalf("stale publication metadata: %s", b)
	}
}

func TestDecodeAndMergePreservesUnknownFields(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Sample.planet")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "planet.json"), []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "planet.key"), []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "22222222-2222-4222-8222-222222222222"), 0700); err != nil {
		t.Fatal(err)
	}
	parsed, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Site.Name != "站点" || parsed.Site.Created.Unix() != 978307210 || len(parsed.Articles) != 1 {
		t.Fatalf("parsed = %#v", parsed)
	}
	parsed.Articles[0].Title = "Changed"
	encoded, err := Merge(parsed.Raw, parsed.Site, parsed.Articles)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result["unknownField"].(map[string]any)["value"] != float64(42) {
		t.Fatal("site metadata lost")
	}
	article := result["articles"].([]any)[0].(map[string]any)
	if article["unknownArticle"] != "preserve" || article["title"] != "Changed" {
		t.Fatalf("article=%#v", article)
	}
}

func TestRejectsUnsafeBackupPaths(t *testing.T) {
	for _, name := range []string{"..", "CON", "name.", "bad:name", "dir/file", "NUL.txt"} {
		if SafeName(name) {
			t.Fatalf("accepted %q", name)
		}
	}
}

func TestRequiresKeyAndArticleDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Sample.planet")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "planet.json"), []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir); err == nil {
		t.Fatal("missing key accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "planet.key"), []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir); err == nil {
		t.Fatal("missing article directory accepted")
	}
}

func TestCopyTreeKeepsAssetsButNotPrivateKey(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(src, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "assets", "style.css"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "planet.key"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CopyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dst, "assets", "style.css"))
	if err != nil || string(b) != "ok" {
		t.Fatalf("asset=%q err=%v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "planet.key")); !os.IsNotExist(err) {
		t.Fatalf("private key was copied: %v", err)
	}
}

func TestSimplePlainRebuildRejectsUnsupportedMetadata(t *testing.T) {
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Site", About: "About"}
	article := store.Article{ID: "22222222-2222-4222-8222-222222222222", SiteID: site.ID, Title: "Updated", Content: "Text"}
	valid := `{"id":"11111111-1111-4111-8111-111111111111","name":"Site","about":"About","templateName":"Plain","articles":[{"id":"22222222-2222-4222-8222-222222222222","link":"/22222222-2222-4222-8222-222222222222/","title":"Before","content":"Text","created":1,"starType":0}]}`
	if err := CheckSimplePlain([]byte(valid), site, []store.Article{article}); err != nil {
		t.Fatal(err)
	}
	unsafe := strings.Replace(valid, `"templateName":"Plain"`, `"templateName":"Plain","customCodeHeadEnabled":true`, 1)
	if err := CheckSimplePlain([]byte(unsafe), site, []store.Article{article}); err == nil {
		t.Fatal("custom code accepted")
	}
	unknown := strings.Replace(valid, `"templateName":"Plain"`, `"templateName":"Plain","unknownFlag":false`, 1)
	if err := CheckSimplePlain([]byte(unknown), site, []store.Article{article}); err == nil {
		t.Fatal("unknown field accepted")
	}
	article.Content = "<iframe>unsafe</iframe>"
	if err := CheckSimplePlain([]byte(valid), site, []store.Article{article}); err == nil {
		t.Fatal("raw HTML was accepted")
	}
}

func TestOverlayReplacesGeneratedFilesAndRetainsOtherFiles(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	for _, dir := range []string{src, dst} {
		if err := os.Mkdir(filepath.Join(dir, "article"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(src, "article", "index.html"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "article", "index.html"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "article", "photo.txt"), []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := OverlayTree(src, dst); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(dst, "article", "index.html"))
	if err != nil || string(page) != "new" {
		t.Fatalf("page=%q err=%v", page, err)
	}
	asset, err := os.ReadFile(filepath.Join(dst, "article", "photo.txt"))
	if err != nil || string(asset) != "preserve" {
		t.Fatalf("asset=%q err=%v", asset, err)
	}
}

func TestMergeIncludesNewArticleAttachments(t *testing.T) {
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Site", Created: time.Now()}
	post := store.Article{ID: "22222222-2222-4222-8222-222222222222", SiteID: site.ID, Title: "Post", Created: time.Now(), Modified: time.Now(), Attachments: []string{"photo.png"}}
	b, err := Merge(nil, site, []store.Article{post})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"attachments"`) || !strings.Contains(string(b), `"photo.png"`) {
		t.Fatalf("backup missing attachment metadata: %s", b)
	}
}
