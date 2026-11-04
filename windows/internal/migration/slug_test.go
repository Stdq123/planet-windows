package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"windows/internal/attachments"
	"windows/internal/backup"
	"windows/internal/ipfs"
	"windows/internal/store"
	"windows/internal/templateinstall"
)

func TestImportedSlugRebuildsBothArticlePathsWithoutPrivateFiles(t *testing.T) {
	template := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if template == "" {
		t.Skip("original Plain template required")
	}
	const id = "11111111-1111-4111-8111-111111111111"
	const post = "22222222-2222-4222-8222-222222222222"
	root := t.TempDir()
	archive := filepath.Join(root, "imports", id)
	if err := os.MkdirAll(filepath.Join(archive, post), 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"article.json": `{"id":"` + post + `","link":"/hello-world/","slug":"hello-world"}`, "photo.png": "photo", "private.txt": "secret"} {
		if err := os.WriteFile(filepath.Join(archive, post, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(archive, "planet.key"), []byte("secret-key"), 0600); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"id":"` + id + `","name":"Site","about":"","templateName":"Plain","articles":[{"id":"` + post + `","link":"/hello-world/","slug":"hello-world","title":"Old","content":"Old","created":10,"starType":0,"attachments":["photo.png"]}]}`)
	article := store.Article{ID: post, SiteID: id, Title: "New", Content: "Updated", Created: time.Unix(10, 0), Modified: time.Unix(20, 0), Attachments: []string{"photo.png"}}
	site := store.Site{ID: id, Name: "Site", Imported: true, Created: time.Unix(0, 0)}
	out := filepath.Join(root, "out")
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	if err := StageImportedPublic(archive, out, raw, site, []store.Article{article}, template); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{post, "hello-world"} {
		b, err := os.ReadFile(filepath.Join(out, dir, "index.html"))
		if err != nil || !strings.Contains(string(b), "Updated") {
			t.Fatalf("%s page: %s %v", dir, b, err)
		}
		b, err = os.ReadFile(filepath.Join(out, dir, "photo.png"))
		if err != nil || string(b) != "photo" {
			t.Fatalf("%s attachment: %s %v", dir, b, err)
		}
		if _, err := os.Stat(filepath.Join(out, dir, "private.txt")); !os.IsNotExist(err) {
			t.Fatalf("private file exposed: %v", err)
		}
	}
	var public struct{ Articles []struct{ Link, Slug string } }
	b, err := os.ReadFile(filepath.Join(out, "planet.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &public); err != nil || len(public.Articles) != 1 || public.Articles[0].Link != "/hello-world/" || public.Articles[0].Slug != "hello-world" {
		t.Fatalf("public metadata %s: %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(out, "planet.key")); !os.IsNotExist(err) {
		t.Fatalf("key exposed: %v", err)
	}
}

func TestSlugBackupRoundTripPreservesKeyAndOnlyCurrentAttachments(t *testing.T) {
	exe, template := os.Getenv("PLANET_TEST_KUBO"), os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if exe == "" || template == "" {
		t.Skip("offline Kubo and original template required")
	}
	const id = "11111111-1111-4111-8111-111111111111"
	const post = "22222222-2222-4222-8222-222222222222"
	root := t.TempDir()
	source := ipfs.New(exe, filepath.Join(root, "source"))
	if err := source.Init(); err != nil {
		t.Fatal(err)
	}
	name, err := source.EnsureKey(id)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "input.planet")
	for _, dir := range []string{post, "hello"} {
		if err := os.MkdirAll(filepath.Join(input, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := source.ExportKey(id, filepath.Join(input, "planet.key")); err != nil {
		t.Fatal(err)
	}
	raw := `{"id":"` + id + `","name":"Site","about":"","ipns":"` + name + `","created":10,"updated":10,"templateName":"Plain","articles":[{"id":"` + post + `","link":"/hello/","slug":"hello","title":"Old","content":"Old","created":10,"starType":0,"attachments":["old.txt"]}]}`
	if err := os.WriteFile(filepath.Join(input, "planet.json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{post, "hello"} {
		if err := os.WriteFile(filepath.Join(input, dir, "article.json"), []byte(`{"id":"`+post+`","slug":"hello","link":"/hello/"}`), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(input, dir, "old.txt"), []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
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
	if _, err = templateinstall.Install(app, template, site, posts, lib); err != nil {
		t.Fatal(err)
	}
	if _, err = attachments.Remove(app, lib, id, post, "old.txt"); err != nil {
		t.Fatal(err)
	}
	newFile := filepath.Join(t.TempDir(), "new.txt")
	if err = os.WriteFile(newFile, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = attachments.Add(app, lib, id, post, newFile); err != nil {
		t.Fatal(err)
	}
	if _, err = lib.SaveArticle(id, post, "New", "Updated"); err != nil {
		t.Fatal(err)
	}
	output, err := m.Export(id, root)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := backup.Read(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Articles) != 1 || parsed.Articles[0].Content != "Updated" {
		t.Fatalf("backup metadata: %+v", parsed.Articles)
	}
	var decoded struct{ Articles []struct{ Slug, Link string } }
	if err = json.Unmarshal(parsed.Raw, &decoded); err != nil || decoded.Articles[0].Slug != "hello" || decoded.Articles[0].Link != "/hello/" {
		t.Fatalf("slug metadata: %s %v", parsed.Raw, err)
	}
	for _, dir := range []string{post, "hello"} {
		if b, e := os.ReadFile(filepath.Join(output, dir, "new.txt")); e != nil || string(b) != "new" {
			t.Fatalf("%s attachment: %q %v", dir, b, e)
		}
		if _, e := os.Stat(filepath.Join(output, dir, "old.txt")); !os.IsNotExist(e) {
			t.Fatalf("%s stale attachment: %v", dir, e)
		}
	}
	check := ipfs.New(exe, filepath.Join(root, "check"))
	if err = check.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err = check.ImportKey(id, filepath.Join(output, "planet.key")); err != nil {
		t.Fatal(err)
	}
	if matches, err := check.KeyMatches(id, name); err != nil || !matches {
		t.Fatalf("backup key mismatch: %v", err)
	}
	if err = lib.DeleteArticle(id, post); err != nil {
		t.Fatal(err)
	}
	output, err = m.Export(id, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{post, "hello"} {
		if _, err = os.Stat(filepath.Join(output, dir)); !os.IsNotExist(err) {
			t.Fatalf("deleted %s remains: %v", dir, err)
		}
	}
	if b, err := os.ReadFile(filepath.Join(input, "hello", "old.txt")); err != nil || string(b) != "old" {
		t.Fatalf("source changed: %q %v", b, err)
	}
}

func TestBackupSlugCopiesAreCheckedAndRemovedBeforeRebuild(t *testing.T) {
	const post = "22222222-2222-4222-8222-222222222222"
	raw := json.RawMessage(`{"articles":[{"id":"` + post + `","slug":"hello","attachments":["photo.png"]}]}`)
	for _, tc := range []struct {
		name, metadata string
		extra          bool
		valid          bool
	}{
		{"known", `{"id":"` + post + `"}`, false, true},
		{"unknown file", `{"id":"` + post + `"}`, true, false},
		{"wrong owner", `{"id":"33333333-3333-4333-8333-333333333333"}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stage := t.TempDir()
			alias := filepath.Join(stage, "hello")
			if err := os.Mkdir(alias, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(alias, "article.json"), []byte(tc.metadata), 0600); err != nil {
				t.Fatal(err)
			}
			if tc.extra {
				if err := os.WriteFile(filepath.Join(alias, "secret.txt"), []byte("private"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := removeObsoleteImportedFiles(stage, raw, nil)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if tc.valid {
				if _, err := os.Stat(alias); !os.IsNotExist(err) {
					t.Fatalf("obsolete alias remains: %v", err)
				}
			}
		})
	}
}

func TestRebuiltPublicMetadataDropsStaleArchivedSlug(t *testing.T) {
	const post = "22222222-2222-4222-8222-222222222222"
	dir := t.TempDir()
	old, fresh := filepath.Join(dir, "old.json"), filepath.Join(dir, "fresh.json")
	if err := os.WriteFile(old, []byte(`{"id":"`+post+`","slug":"old-path","link":"/old-path/"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fresh, []byte(`{"id":"`+post+`","link":"/`+post+`/"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := mergeArticleMetadata(old, fresh, post); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(fresh)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["slug"]; ok {
		t.Fatalf("stale slug retained: %s", b)
	}
}
