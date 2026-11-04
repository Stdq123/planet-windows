package migration

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"windows/internal/attachments"
	"windows/internal/backup"
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
	if _, ok := item["articleNumber"]; ok {
		t.Fatal("unchanged backup unexpectedly rewrote article numbers")
	}
	if _, ok := parsed["nextArticleNumber"]; ok {
		t.Fatal("unchanged backup unexpectedly rewrote number counter")
	}
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

func TestSepiaBackupSettingsSurviveEditedExport(t *testing.T) {
	exe, templates := os.Getenv("PLANET_TEST_KUBO"), os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if exe == "" || templates == "" {
		t.Skip("Kubo and original templates required")
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
	if err = os.MkdirAll(filepath.Join(input, post), 0700); err != nil {
		t.Fatal(err)
	}
	if err = source.ExportKey(id, filepath.Join(input, "planet.key")); err != nil {
		t.Fatal(err)
	}
	raw := `{"id":"` + id + `","name":"Site","about":"","ipns":"` + name + `","created":10,"templateName":"Sepia","pinnableEnabled":true,"pinnableAPIEndpoint":"https://pinnable.xyz/pin/PRIVATE-ENDPOINT","filebaseEnabled":true,"filebasePinName":"site","filebaseAPIToken":"PRIVATE-FILEBASE-TOKEN","articles":[{"id":"` + post + `","link":"/` + post + `/","title":"Old","content":"Body","created":10,"starType":0}]}`
	if err = os.WriteFile(filepath.Join(input, "planet.json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(input, "templateSettings.json"), []byte(`{"backgroundColor":"#123456","showRef":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(root, "app")
	lib, err := store.Open(app)
	if err != nil {
		t.Fatal(err)
	}
	node := ipfs.New(exe, filepath.Join(app, "ipfs"))
	if err = node.Init(); err != nil {
		t.Fatal(err)
	}
	m := New(app, exe, lib, node)
	site, err := m.Import(input)
	if err != nil {
		t.Fatal(err)
	}
	posts, err := lib.ListArticles(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = templateinstall.Install(app, filepath.Join(templates, "sepia"), site, posts, lib); err != nil {
		t.Fatal(err)
	}
	if _, err = lib.SaveArticle(id, post, "Updated", "Body"); err != nil {
		t.Fatal(err)
	}
	if _, err = lib.SetPlausibleTrackingScript(id, true, "example.com", "pa-Test123"); err != nil {
		t.Fatal(err)
	}
	output, err := m.Export(id, root)
	if err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(output, "index.html"))
	if err != nil || !strings.Contains(string(page), "--background-color: #123456;") {
		t.Fatalf("settings not rendered: %v", err)
	}
	if strings.Contains(string(page), "PRIVATE-ENDPOINT") || strings.Contains(string(page), "PRIVATE-FILEBASE-TOKEN") {
		t.Fatal("pinning credentials leaked into the public page")
	}
	if !strings.Contains(string(page), "https://plausible.io/js/pa-Test123.js") || strings.Contains(string(page), "plausible.local.js") {
		t.Fatal("site-specific tracking not rendered in backup")
	}
	parsed, err := backup.Read(output)
	if err != nil || parsed.Site.PlausibleOverride == nil || parsed.Site.PlausibleOverride.ScriptID != "pa-Test123" {
		t.Fatalf("site-specific tracking lost in backup: %+v %v", parsed.Site.PlausibleOverride, err)
	}
	metadata, err := os.ReadFile(filepath.Join(output, "planet.json"))
	if err != nil || !strings.Contains(string(metadata), "PRIVATE-ENDPOINT") || !strings.Contains(string(metadata), "PRIVATE-FILEBASE-TOKEN") {
		t.Fatalf("original pinning backup configuration lost: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(output, "templateSettings.json"))
	if err != nil || !strings.Contains(string(b), "#123456") {
		t.Fatalf("settings lost: %s %v", b, err)
	}
	if _, err := lib.SetTemplateSettings(id, map[string]string{"backgroundColor": "#654321", "showRef": "false"}); err != nil {
		t.Fatal(err)
	}
	secondParent := filepath.Join(root, "second")
	if err := os.Mkdir(secondParent, 0700); err != nil {
		t.Fatal(err)
	}
	output, err = m.Export(id, secondParent)
	if err != nil {
		t.Fatal(err)
	}
	page, err = os.ReadFile(filepath.Join(output, "index.html"))
	if err != nil || !strings.Contains(string(page), "--background-color: #654321;") {
		t.Fatalf("edited setting not rendered: %v", err)
	}
	b, err = os.ReadFile(filepath.Join(output, "templateSettings.json"))
	if err != nil || !strings.Contains(string(b), "#654321") {
		t.Fatalf("edited setting lost: %s %v", b, err)
	}
	current, posts, _, err := lib.PublicationSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = templateinstall.Install(app, filepath.Join(templates, "plain"), current, posts, lib); err != nil {
		t.Fatal(err)
	}
	thirdParent := filepath.Join(root, "third")
	if err = os.Mkdir(thirdParent, 0700); err != nil {
		t.Fatal(err)
	}
	output, err = m.Export(id, thirdParent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(output, "templateSettings.json")); !os.IsNotExist(err) {
		t.Fatalf("old settings retained after template switch: %v", err)
	}
	if b, err = os.ReadFile(filepath.Join(app, "imports", id, "templateSettings.json")); err != nil || !strings.Contains(string(b), "#123456") {
		t.Fatalf("original settings changed: %s %v", b, err)
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
	if err := os.WriteFile(filepath.Join(input, "templateSettings.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	avatar, err := os.Create(filepath.Join(input, "avatar.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(avatar, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if err := avatar.Close(); err != nil {
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
	if _, err := library.UpdateSite(site.ID, "Updated site", "Updated description"); err != nil {
		t.Fatal(err)
	}
	output, err := m.Export(site.ID, root)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(output, "templateSettings.json")); err != nil || strings.TrimSpace(string(b)) != "{}" {
		t.Fatalf("template settings lost: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(output, "avatar.png")); err != nil {
		t.Fatalf("avatar lost: %v", err)
	}
	index, err := os.ReadFile(filepath.Join(output, "index.html"))
	if err != nil || !strings.Contains(string(index), `src="./avatar.png"`) || !strings.Contains(string(index), "Updated site") || !strings.Contains(string(index), "Updated description") {
		t.Fatalf("avatar missing from rebuilt index: %v", err)
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
	var updatedName string
	if err := json.Unmarshal(metadataCheck["name"], &updatedName); err != nil || updatedName != "Updated site" {
		t.Fatalf("site metadata not updated: %s", metadataOutput)
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

func TestRebuiltImportedBackupReflectsArticleAndAttachmentEdits(t *testing.T) {
	exe, template := os.Getenv("PLANET_TEST_KUBO"), os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
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
	if err = os.MkdirAll(filepath.Join(input, post), 0700); err != nil {
		t.Fatal(err)
	}
	if err = source.ExportKey(id, filepath.Join(input, "planet.key")); err != nil {
		t.Fatal(err)
	}
	raw := `{"id":"` + id + `","name":"Site","about":"","ipns":"` + name + `","created":10,"templateName":"Plain","articles":[{"id":"` + post + `","link":"/` + post + `/","title":"Old","content":"Body","created":10,"starType":0,"attachments":["old.txt"]}]}`
	os.WriteFile(filepath.Join(input, "planet.json"), []byte(raw), 0600)
	os.WriteFile(filepath.Join(input, post, "old.txt"), []byte("old"), 0600)
	app := filepath.Join(root, "app")
	lib, err := store.Open(app)
	if err != nil {
		t.Fatal(err)
	}
	node := ipfs.New(exe, filepath.Join(app, "ipfs"))
	if err = node.Init(); err != nil {
		t.Fatal(err)
	}
	m := New(app, exe, lib, node)
	site, err := m.Import(input)
	if err != nil {
		t.Fatal(err)
	}
	posts, _ := lib.ListArticles(id)
	if _, err = templateinstall.Install(app, template, site, posts, lib); err != nil {
		t.Fatal(err)
	}
	if _, err = attachments.Remove(app, lib, id, post, "old.txt"); err != nil {
		t.Fatal(err)
	}
	newFile := filepath.Join(t.TempDir(), "new.txt")
	os.WriteFile(newFile, []byte("new"), 0600)
	if _, err = attachments.Add(app, lib, id, post, newFile); err != nil {
		t.Fatal(err)
	}
	added, err := lib.SaveArticle(id, "", "Added", "New article")
	if err != nil {
		t.Fatal(err)
	}
	output, err := m.Export(id, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(output, post, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("removed attachment retained: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(output, post, "new.txt")); err != nil || string(b) != "new" {
		t.Fatalf("new file: %q %v", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(output, added.ID, "index.html")); err != nil || !strings.Contains(string(b), "New article") {
		t.Fatalf("new article: %q %v", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(output, "planet.json")); err != nil || strings.Contains(string(b), "old.txt") || !strings.Contains(string(b), "new.txt") {
		t.Fatalf("metadata: %s %v", b, err)
	}
	metadataBytes, err := os.ReadFile(filepath.Join(output, "planet.json"))
	if err != nil {
		t.Fatal(err)
	}
	var numbered struct {
		Next     int `json:"nextArticleNumber"`
		Articles []struct {
			ID     string `json:"id"`
			Number int    `json:"articleNumber"`
		} `json:"articles"`
	}
	if err := json.Unmarshal(metadataBytes, &numbered); err != nil {
		t.Fatal(err)
	}
	if numbered.Next != 3 {
		t.Fatalf("wrong next number: %s", metadataBytes)
	}
	for _, a := range numbered.Articles {
		want := 1
		if a.ID == added.ID {
			want = 2
		}
		if a.Number != want {
			t.Fatalf("wrong backup article number: %s", metadataBytes)
		}
		publicBytes, err := os.ReadFile(filepath.Join(output, a.ID, "article.json"))
		if err != nil {
			t.Fatal(err)
		}
		var public struct {
			Number    int    `json:"articleNumber"`
			Reference string `json:"articleReference"`
		}
		if err := json.Unmarshal(publicBytes, &public); err != nil {
			t.Fatal(err)
		}
		if public.Number != want || public.Reference != fmt.Sprintf("11111111-%d", want) {
			t.Fatalf("public and backup numbers disagree: %s", publicBytes)
		}
	}
	// The Mac importer copies the entire article directory into the public site.
	// Refuse an edited backup if an unregistered archived file would be resurrected.
	unregistered := filepath.Join(app, "imports", id, post, "unregistered.txt")
	if err := os.WriteFile(unregistered, []byte("unregistered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Export(id, root); err == nil {
		t.Fatal("export retained an unregistered article file")
	}
	if err := os.Remove(unregistered); err != nil {
		t.Fatal(err)
	}
	assetDir := filepath.Join(app, "imports", id, "assets")
	if err := os.MkdirAll(assetDir, 0700); err != nil {
		t.Fatal(err)
	}
	asset := filepath.Join(assetDir, "unexpected.txt")
	if err := os.WriteFile(asset, []byte("unregistered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Export(id, root); err == nil {
		t.Fatal("export retained an unrecognized template asset")
	}
	if err := os.Remove(asset); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(app, "imports", id, "templateSettings.json")
	if err := os.WriteFile(settings, []byte(`{"custom":"value"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Export(id, root); err == nil {
		t.Fatal("export retained unrecognized template settings")
	}
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if err = lib.DeleteArticle(id, post); err != nil {
		t.Fatal(err)
	}
	output, err = m.Export(id, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(output, post)); !os.IsNotExist(err) {
		t.Fatalf("deleted article retained: %v", err)
	}
	if _, err = os.Stat(filepath.Join(app, "imports", id, post, "old.txt")); err != nil {
		t.Fatalf("archive changed: %v", err)
	}
}

func TestMediaImportOfflineIPFSAndBackup(t *testing.T) {
	exe, template := os.Getenv("PLANET_TEST_KUBO"), os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if exe == "" || template == "" {
		t.Skip("Kubo and Plain template required")
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
	input := filepath.Join(root, "source.planet")
	os.MkdirAll(filepath.Join(input, post), 0700)
	if err = source.ExportKey(id, filepath.Join(input, "planet.key")); err != nil {
		t.Fatal(err)
	}
	raw := `{"id":"` + id + `","name":"Site","about":"","ipns":"` + name + `","created":10,"templateName":"Plain","articles":[{"id":"` + post + `","link":"/` + post + `/","title":"Media","content":"Body","created":10,"starType":0,"audioFilename":"song.mp3","videoFilename":"clip.mp4","attachments":["song.mp3","clip.mp4"]}]}`
	os.WriteFile(filepath.Join(input, "planet.json"), []byte(raw), 0600)
	os.WriteFile(filepath.Join(input, post, "song.mp3"), []byte("synthetic audio"), 0600)
	os.WriteFile(filepath.Join(input, post, "clip.mp4"), []byte("synthetic video"), 0600)
	os.WriteFile(filepath.Join(input, post, "_videoThumbnail.png"), []byte("thumbnail"), 0600)
	os.WriteFile(filepath.Join(input, post, "article.json"), []byte(`{"id":"`+post+`","hasAudio":true,"audioFilename":"song.mp3","audioDuration":12,"audioByteLength":15,"hasVideo":true,"videoFilename":"clip.mp4","attachments":["song.mp3","clip.mp4"]}`), 0600)
	app := filepath.Join(root, "app")
	lib, err := store.Open(app)
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(app, "ipfs")
	node := ipfs.New(exe, repo)
	if err = node.Init(); err != nil {
		t.Fatal(err)
	}
	manager := New(app, exe, lib, node)
	site, err := manager.Import(input)
	if err != nil {
		t.Fatal(err)
	}
	posts, _ := lib.ListArticles(id)
	if _, err = templateinstall.Install(app, template, site, posts, lib); err != nil {
		t.Fatal(err)
	}
	if _, err = lib.SaveArticle(id, post, "Edited", "Body"); err != nil {
		t.Fatal(err)
	}
	output, err := manager.Export(id, root)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(output, post, "article.json"))
	if err != nil || !strings.Contains(string(data), `"audioDuration": 12`) || !strings.Contains(string(data), `"videoFilename": "clip.mp4"`) {
		t.Fatalf("metadata=%s err=%v", data, err)
	}
	backupData, err := os.ReadFile(filepath.Join(output, "planet.json"))
	if err != nil || !strings.Contains(string(backupData), `"audioFilename": "song.mp3"`) {
		t.Fatalf("backup=%s err=%v", backupData, err)
	}
	if b, err := os.ReadFile(filepath.Join(output, post, "_videoThumbnail.png")); err != nil || string(b) != "thumbnail" {
		t.Fatalf("thumbnail=%q err=%v", b, err)
	}
	site, posts, original, err := lib.PublicationSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	public := filepath.Join(root, "public")
	os.Mkdir(public, 0700)
	if err = StageImportedPublic(filepath.Join(app, "imports", id), public, original, site, posts, filepath.Join(app, "templates", site.TemplateID)); err != nil {
		t.Fatal(err)
	}
	if err = node.Start(false); err != nil {
		t.Fatal(err)
	}
	defer node.Close()
	cid, err := node.Add(public)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "cat", "/ipfs/"+cid+"/"+post+"/song.mp3")
	cmd.Env = append(os.Environ(), "IPFS_PATH="+repo)
	b, err := cmd.Output()
	if err != nil || string(b) != "synthetic audio" {
		t.Fatalf("IPFS file=%q err=%v", b, err)
	}
}
