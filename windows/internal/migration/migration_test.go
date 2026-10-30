package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"windows/internal/attachments"
	"windows/internal/ipfs"
	"windows/internal/store"
	"windows/internal/templateinstall"
)

func TestImportExportKeepsKeyAndUnknownMetadata(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	root := t.TempDir()
	source := ipfs.New(exe, filepath.Join(root, "source"))
	if err := source.Init(); err != nil {
		t.Fatal(err)
	}
	const id = "11111111-1111-4111-8111-111111111111"
	const post = "22222222-2222-4222-8222-222222222222"
	name, err := source.EnsureKey(id)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "input.planet")
	if err := os.Mkdir(input, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(input, post), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input, post, "image.txt"), []byte("attachment"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := source.ExportKey(id, filepath.Join(input, "planet.key")); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{"id": id, "name": "Imported", "about": "about", "ipns": name, "created": 10, "updated": 20, "templateName": "Plain", "unknown": "keep", "articles": []any{map[string]any{"id": post, "link": "/" + post + "/", "title": "Post", "content": "Body", "created": 10, "starType": 0, "extra": "keep"}}}
	b, _ := json.Marshal(metadata)
	if err := os.WriteFile(filepath.Join(input, "planet.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	appDir := filepath.Join(root, "app")
	s, err := store.Open(appDir)
	if err != nil {
		t.Fatal(err)
	}
	node := ipfs.New(exe, filepath.Join(appDir, "ipfs"))
	if err := node.Init(); err != nil {
		t.Fatal(err)
	}
	m := New(appDir, exe, s, node)
	site, err := m.Import(input)
	if err != nil {
		t.Fatal(err)
	}
	if site.IPNS != name {
		t.Fatalf("identity changed: %s", site.IPNS)
	}
	if _, err := m.Import(input); err == nil {
		t.Fatal("duplicate import accepted")
	}
	output, err := m.Export(site.ID, root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := os.ReadFile(filepath.Join(output, "planet.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}
	item := parsed["articles"].([]any)[0].(map[string]any)
	if parsed["unknown"] != "keep" || item["extra"] != "keep" || item["title"] != "Post" {
		t.Fatalf("metadata=%s", result)
	}
	if content, err := os.ReadFile(filepath.Join(output, post, "image.txt")); err != nil || string(content) != "attachment" {
		t.Fatalf("asset=%q err=%v", content, err)
	}
	verify := ipfs.New(exe, filepath.Join(root, "verify"))
	if err := verify.Init(); err != nil {
		t.Fatal(err)
	}
	imported, err := verify.ImportKey(id, filepath.Join(output, "planet.key"))
	if err != nil || imported != name {
		t.Fatalf("imported=%q err=%v", imported, err)
	}
	posts, err := s.ListArticles(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveArticle(site.ID, posts[0].ID, "Changed", "Body"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Export(site.ID, root); err == nil {
		t.Fatal("modified imported site exported with stale article pages")
	}
}

func TestImportRejectsMismatchedPrivateKey(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	root := t.TempDir()
	source := ipfs.New(exe, filepath.Join(root, "source"))
	if err := source.Init(); err != nil {
		t.Fatal(err)
	}
	const id = "11111111-1111-4111-8111-111111111111"
	if _, err := source.EnsureKey(id); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "wrong.planet")
	if err := os.Mkdir(input, 0700); err != nil {
		t.Fatal(err)
	}
	if err := source.ExportKey(id, filepath.Join(input, "planet.key")); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{"id": id, "name": "Site", "about": "", "ipns": "k51wrong", "created": 10, "updated": 10, "templateName": "Plain", "articles": []any{}})
	if err := os.WriteFile(filepath.Join(input, "planet.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	appDir := filepath.Join(root, "app")
	s, err := store.Open(appDir)
	if err != nil {
		t.Fatal(err)
	}
	node := ipfs.New(exe, filepath.Join(appDir, "ipfs"))
	if err := node.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := New(appDir, exe, s, node).Import(input); err == nil {
		t.Fatal("mismatched key was accepted")
	}
	sites, err := s.ListSites()
	if err != nil || len(sites) != 0 {
		t.Fatalf("sites=%#v err=%v", sites, err)
	}
	key, err := node.LookupKey(id)
	if err != nil || key != "" {
		t.Fatalf("unexpected key=%q err=%v", key, err)
	}
}

func TestModifiedSimplePlainBackupRebuildsPagesAndPreservesFiles(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	template := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if exe == "" || template == "" {
		t.Skip("Kubo and original Plain template required")
	}
	root := t.TempDir()
	source := ipfs.New(exe, filepath.Join(root, "source"))
	if err := source.Init(); err != nil {
		t.Fatal(err)
	}
	const id = "11111111-1111-4111-8111-111111111111"
	const post = "22222222-2222-4222-8222-222222222222"
	name, err := source.EnsureKey(id)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "input.planet")
	if err := os.MkdirAll(filepath.Join(input, post), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input, post, "attachment.txt"), []byte("original attachment"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input, post, "index.html"), []byte("OLD PAGE"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input, post, "article.md"), []byte("OLD MARKDOWN"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input, post, "article.json"), []byte(`{"title":"Old","articleNumber":7,"hasVideo":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input, "page2.html"), []byte("STALE PAGE"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := source.ExportKey(id, filepath.Join(input, "planet.key")); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{"id": id, "name": "Imported", "about": "About", "ipns": name, "created": 10, "updated": 20, "lastPublished": 19, "lastPublishedCID": "stale-cid", "templateName": "Plain", "articles": []any{map[string]any{"id": post, "link": "/" + post + "/", "title": "Old", "content": "Old body", "created": 10, "starType": 0, "attachments": []string{"attachment.txt"}}}}
	b, _ := json.Marshal(metadata)
	if err := os.WriteFile(filepath.Join(input, "planet.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(root, "app")
	library, err := store.Open(app)
	if err != nil {
		t.Fatal(err)
	}
	node := ipfs.New(exe, filepath.Join(app, "ipfs"))
	if err := node.Init(); err != nil {
		t.Fatal(err)
	}
	m := New(app, exe, library, node)
	site, err := m.Import(input)
	if err != nil {
		t.Fatal(err)
	}
	articles, err := library.ListArticles(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := templateinstall.Install(app, template, site, articles, library); err != nil {
		t.Fatal(err)
	}
	if _, err := library.SaveArticle(site.ID, post, "New title", "**Updated text**"); err != nil {
		t.Fatal(err)
	}
	output, err := m.Export(site.ID, root)
	if err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(output, post, "index.html"))
	if err != nil || !strings.Contains(string(page), "New title") || !strings.Contains(string(page), "<strong>Updated text</strong>") {
		t.Fatalf("page=%q err=%v", page, err)
	}
	attachment, err := os.ReadFile(filepath.Join(output, post, "attachment.txt"))
	if err != nil || string(attachment) != "original attachment" {
		t.Fatalf("attachment=%q err=%v", attachment, err)
	}
	if _, err := os.Stat(filepath.Join(output, "page2.html")); !os.IsNotExist(err) {
		t.Fatalf("stale generated page was retained: %v", err)
	}
	metadataOutput, err := os.ReadFile(filepath.Join(output, "planet.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Articles []struct {
			Rendered string `json:"contentRendered"`
		} `json:"articles"`
	}
	if err = json.Unmarshal(metadataOutput, &result); err != nil || len(result.Articles) != 1 || !strings.Contains(result.Articles[0].Rendered, "<strong>Updated text</strong>") {
		t.Fatalf("rendered metadata=%s err=%v", metadataOutput, err)
	}
	var metadataCheck map[string]json.RawMessage
	if err = json.Unmarshal(metadataOutput, &metadataCheck); err != nil {
		t.Fatal(err)
	}
	if _, ok := metadataCheck["lastPublishedCID"]; ok {
		t.Fatal("stale published CID survived rebuild")
	}
	if _, ok := metadataCheck["lastPublished"]; ok {
		t.Fatal("stale publication date survived rebuild")
	}
	markdown, err := os.ReadFile(filepath.Join(output, post, "article.md"))
	if err != nil || string(markdown) != "New title\n\n**Updated text**" {
		t.Fatalf("stale markdown=%q err=%v", markdown, err)
	}
	articleJSON, err := os.ReadFile(filepath.Join(output, post, "article.json"))
	if err != nil || !strings.Contains(string(articleJSON), "New title") || !strings.Contains(string(articleJSON), "articleNumber") || !strings.Contains(string(articleJSON), "attachment.txt") {
		t.Fatalf("stale article metadata=%s err=%v", articleJSON, err)
	}
	oldPublic := filepath.Join(app, "imports", id, post, "article.json")
	if err := os.WriteFile(oldPublic, []byte(`{"hasVideo":true,"videoFilename":"movie.mp4"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Export(site.ID, root); err == nil {
		t.Fatal("inconsistent video metadata was accepted")
	}
	if err := os.WriteFile(oldPublic, []byte(`{"title":"Old","articleNumber":7,"hasVideo":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "imports", id, post, "nft.json"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Export(site.ID, root); err == nil {
		t.Fatal("stale NFT metadata was silently retained")
	}
	original, err := os.ReadFile(filepath.Join(input, post, "index.html"))
	if err != nil || string(original) != "OLD PAGE" {
		t.Fatalf("original changed: %q %v", original, err)
	}
}

func TestExportWindowsSiteContainsAttachments(t *testing.T) {
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
	post, err := library.SaveArticle(site.ID, "", "Title", "See attachment")
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "photo.png")
	if err = os.WriteFile(input, []byte("image data"), 0600); err != nil {
		t.Fatal(err)
	}
	post, err = attachments.Add(root, library, site.ID, post.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := node.EnsureKey(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = library.SetIdentity(site.ID, identity); err != nil {
		t.Fatal(err)
	}
	if err = library.SetPublished(site.ID, identity, "older-cid"); err != nil {
		t.Fatal(err)
	}
	if _, err = library.SaveArticle(site.ID, post.ID, "Updated", "Changed after publication"); err != nil {
		t.Fatal(err)
	}
	output, err := New(root, exe, library, node).Export(site.ID, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(output, post.ID, "photo.png"))
	if err != nil || string(data) != "image data" {
		t.Fatalf("attachment=%q err=%v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(output, "planet.json"))
	if err != nil || !strings.Contains(string(data), `"photo.png"`) {
		t.Fatalf("metadata=%s err=%v", data, err)
	}
	if strings.Contains(string(data), "older-cid") {
		t.Fatalf("stale publication retained in backup: %s", data)
	}
}
