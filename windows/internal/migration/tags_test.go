package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"windows/internal/backup"
	"windows/internal/ipfs"
	"windows/internal/store"
	"windows/internal/templateinstall"
)

func TestImportedTagsRenderFilteredPagesWithoutPrivateMetadata(t *testing.T) {
	template := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if template == "" {
		t.Skip("original template required")
	}
	const siteID = "11111111-1111-4111-8111-111111111111"
	const one = "22222222-2222-4222-8222-222222222222"
	const two = "33333333-3333-4333-8333-333333333333"
	root := t.TempDir()
	archive := filepath.Join(root, "imports", siteID)
	out := filepath.Join(root, "out")
	for _, id := range []string{one, two} {
		if err := os.MkdirAll(filepath.Join(archive, id), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archive, "planet.key"), []byte("SECRET-KEY"), 0600); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"id":"` + siteID + `","name":"Site","about":"","templateName":"Plain","plausibleAPIKey":"SECRET-TOKEN","tags":{"go":"Go & <script>bad</script>","other":"Other"},"articles":[{"id":"` + one + `","link":"/` + one + `/","title":"First","content":"First body","created":1,"starType":0,"tags":{"go":"Go & <script>bad</script>"}},{"id":"` + two + `","link":"/` + two + `/","title":"Second","content":"Second body","created":2,"starType":0,"tags":{"other":"Other"}}]}`)
	raw = []byte(strings.ReplaceAll(string(raw), `"go":"Go & <script>bad</script>"`, `"go":"Go & <script>bad</script>","中文":"中文标签"`))
	site := store.Site{ID: siteID, Name: "Site", Imported: true, Created: time.Unix(0, 0)}
	articles := []store.Article{{ID: one, SiteID: siteID, Title: "First", Content: "First body", Created: time.Unix(1, 0)}, {ID: two, SiteID: siteID, Title: "Second", Content: "Second body", Created: time.Unix(2, 0)}}
	if err := StageImportedPublic(archive, out, raw, site, articles, template); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "go.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "First") || strings.Contains(string(b), "Second") || strings.Contains(string(b), "<script>bad</script>") {
		t.Fatalf("unsafe or unfiltered tag page: %s", b)
	}
	b, err = os.ReadFile(filepath.Join(out, "tags.html"))
	if err != nil || !strings.Contains(string(b), "go.html") || !strings.Contains(string(b), "other.html") || strings.Contains(string(b), "<script>bad</script>") {
		t.Fatalf("tag index: %s %v", b, err)
	}
	if !strings.Contains(string(b), "中文.html") || (!strings.Contains(string(b), `tag-cloud-item-count">1`) && !strings.Contains(string(b), `1 post`)) {
		t.Fatalf("missing tag path or count: %s", b)
	}
	if strings.Contains(string(b), "&amp;amp;") || !strings.Contains(string(b), "Go &amp; &lt;script&gt;bad&lt;/script&gt;") {
		t.Fatalf("incorrectly escaped label: %s", b)
	}
	if b, err := os.ReadFile(filepath.Join(out, "中文.html")); err != nil || !strings.Contains(string(b), "First") {
		t.Fatalf("unicode tag page: %s %v", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(out, one, "index.html")); err != nil || strings.Contains(string(b), "<script>bad</script>") || strings.Contains(string(b), "&amp;amp;") || !strings.Contains(string(b), "Go &amp; &lt;script&gt;bad&lt;/script&gt;") {
		t.Fatalf("incorrect article tag label: %s %v", b, err)
	}
	b, err = os.ReadFile(filepath.Join(out, one, "article.json"))
	if err != nil {
		t.Fatal(err)
	}
	var public struct {
		Tags map[string]string `json:"tags"`
	}
	if err = json.Unmarshal(b, &public); err != nil || public.Tags["go"] != "Go & <script>bad</script>" {
		t.Fatalf("public tags: %s %v", b, err)
	}
	b, err = os.ReadFile(filepath.Join(out, "planet.json"))
	if err != nil || strings.Contains(string(b), "SECRET-TOKEN") {
		t.Fatalf("private data leaked: %s %v", b, err)
	}
	if _, err = os.Stat(filepath.Join(out, "planet.key")); !os.IsNotExist(err) {
		t.Fatalf("key exposed: %v", err)
	}
}

func TestImportedTagPageTitleCannotInjectHTML(t *testing.T) {
	template := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if template == "" {
		t.Skip("original template required")
	}
	const siteID = "11111111-1111-4111-8111-111111111111"
	const post = "22222222-2222-4222-8222-222222222222"
	root := t.TempDir()
	archive := filepath.Join(root, "imports", siteID)
	out := filepath.Join(root, "out")
	if err := os.MkdirAll(filepath.Join(archive, post), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	label := `</title><script>alert('x')</script>`
	raw, _ := json.Marshal(map[string]any{"id": siteID, "name": "Site", "templateName": "Plain", "tags": map[string]string{"go": label}, "articles": []any{map[string]any{"id": post, "link": "/" + post + "/", "tags": map[string]string{"go": label}}}})
	site := store.Site{ID: siteID, Name: "Site", Imported: true}
	article := store.Article{ID: post, SiteID: siteID, Title: "Post"}
	if err := StageImportedPublic(archive, out, raw, site, []store.Article{article}, template); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "go.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "<script>alert(") || !strings.Contains(string(b), "&lt;/title&gt;&lt;script&gt;") {
		t.Fatalf("unsafe tag title: %s", b)
	}
}

func TestTaggedBackupRoundTripAfterArticleDeletion(t *testing.T) {
	exe, template := os.Getenv("PLANET_TEST_KUBO"), os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if exe == "" || template == "" {
		t.Skip("offline Kubo and original template required")
	}
	const siteID = "11111111-1111-4111-8111-111111111111"
	const one = "22222222-2222-4222-8222-222222222222"
	const two = "33333333-3333-4333-8333-333333333333"
	root := t.TempDir()
	source := ipfs.New(exe, filepath.Join(root, "source"))
	if err := source.Init(); err != nil {
		t.Fatal(err)
	}
	identity, err := source.EnsureKey(siteID)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "input.planet")
	for _, name := range []string{one, two, "first"} {
		if err := os.MkdirAll(filepath.Join(input, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := source.ExportKey(siteID, filepath.Join(input, "planet.key")); err != nil {
		t.Fatal(err)
	}
	raw := `{"id":"` + siteID + `","name":"Site","about":"","ipns":"` + identity + `","created":10,"updated":10,"templateName":"Plain","prewarmNewPost":true,"customCodeHeadEnabled":false,"customCodeHead":"<script>PRIVATE-HEAD</script>","customCodeBodyStartEnabled":false,"customCodeBodyStart":"PRIVATE-START","customCodeBodyEndEnabled":false,"customCodeBodyEnd":"PRIVATE-END","plausibleAPIKey":"SECRET-TOKEN","tags":{"go":"Go","old":"Old"},"articles":[{"id":"` + one + `","link":"/first/","slug":"first","title":"First","content":"First body","created":10,"starType":0,"tags":{"go":"Go"},"attachments":["photo.png"]},{"id":"` + two + `","link":"/` + two + `/","title":"Second","content":"Second body","created":20,"starType":0,"tags":{"old":"Old"}}]}`
	if err := os.WriteFile(filepath.Join(input, "planet.json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input, "private-note.txt"), []byte("CUSTOM PRIVATE FILE"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{one, "first"} {
		if err := os.WriteFile(filepath.Join(input, dir, "article.json"), []byte(`{"id":"`+one+`","slug":"first","tags":{"go":"Go"}}`), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(input, dir, "photo.png"), []byte("photo"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(input, two, "article.json"), []byte(`{"id":"`+two+`","tags":{"old":"Old"}}`), 0600); err != nil {
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
	posts, err := lib.ListArticles(siteID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = templateinstall.Install(app, template, site, posts, lib); err != nil {
		t.Fatal(err)
	}
	if err = lib.DeleteArticle(siteID, two); err != nil {
		t.Fatal(err)
	}
	if _, err = lib.SaveArticle(siteID, one, "Changed", "Current body"); err != nil {
		t.Fatal(err)
	}
	public := filepath.Join(root, "public-check")
	if err = os.Mkdir(public, 0700); err != nil {
		t.Fatal(err)
	}
	snapshot, currentPosts, metadata, e := lib.PublicationSnapshot(siteID)
	if e != nil {
		t.Fatal(e)
	}
	if e = StageImportedPublic(filepath.Join(app, "imports", siteID), public, metadata, snapshot, currentPosts, filepath.Join(app, "templates", snapshot.TemplateID)); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"planet.key", "private-note.txt"} {
		if _, e = os.Lstat(filepath.Join(public, name)); !os.IsNotExist(e) {
			t.Fatalf("private file in public output: %s %v", name, e)
		}
	}
	if b, e := os.ReadFile(filepath.Join(public, "planet.json")); e != nil || strings.Contains(string(b), "SECRET-TOKEN") || strings.Contains(string(b), "prewarmNewPost") {
		t.Fatalf("private metadata in public output: %s %v", b, e)
	}
	if e := filepath.WalkDir(public, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, secret := range []string{"PRIVATE-HEAD", "PRIVATE-START", "PRIVATE-END", "SECRET-TOKEN"} {
			if strings.Contains(string(b), secret) {
				t.Fatalf("private configuration in public file %s", path)
			}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	archivedCustom := filepath.Join(app, "imports", siteID, "tags.html")
	if err = os.WriteFile(archivedCustom, []byte("CUSTOM TAG PAGE"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Export(siteID, root); err == nil {
		t.Fatal("export discarded an unverified page")
	}
	if b, e := os.ReadFile(archivedCustom); e != nil || string(b) != "CUSTOM TAG PAGE" {
		t.Fatalf("archived custom page changed: %s %v", b, e)
	}
	if err = os.Remove(archivedCustom); err != nil {
		t.Fatal(err)
	}
	output, err := m.Export(siteID, root)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := backup.Read(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(parsed.Raw), `"prewarmNewPost": true`) {
		t.Fatalf("Mac prewarm setting lost: %s", parsed.Raw)
	}
	for _, secret := range []string{"PRIVATE-HEAD", "PRIVATE-START", "PRIVATE-END"} {
		if !strings.Contains(string(parsed.Raw), secret) {
			t.Fatalf("disabled code lost: %s", parsed.Raw)
		}
	}
	if b, e := os.ReadFile(filepath.Join(output, "private-note.txt")); e != nil || string(b) != "CUSTOM PRIVATE FILE" {
		t.Fatalf("unknown backup file lost: %q %v", b, e)
	}
	var p struct {
		Tags     map[string]string `json:"tags"`
		Articles []struct {
			Tags map[string]string `json:"tags"`
		} `json:"articles"`
	}
	if err = json.Unmarshal(parsed.Raw, &p); err != nil || len(p.Tags) != 1 || p.Tags["go"] != "Go" || len(p.Articles) != 1 || p.Articles[0].Tags["go"] != "Go" {
		t.Fatalf("backup tags: %s %v", parsed.Raw, err)
	}
	for _, name := range []string{two, "old.html"} {
		if _, e := os.Stat(filepath.Join(output, name)); !os.IsNotExist(e) {
			t.Fatalf("deleted content %s retained: %v", name, e)
		}
	}
	for _, dir := range []string{one, "first"} {
		if b, e := os.ReadFile(filepath.Join(output, dir, "photo.png")); e != nil || string(b) != "photo" {
			t.Fatalf("%s attachment: %s %v", dir, b, e)
		}
	}
	if b, e := os.ReadFile(filepath.Join(output, "go.html")); e != nil || !strings.Contains(string(b), "Changed") || strings.Contains(string(b), "Second") || strings.Contains(string(b), "SECRET-TOKEN") {
		t.Fatalf("tag page: %s %v", b, e)
	}
	check := ipfs.New(exe, filepath.Join(root, "check"))
	if err = check.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err = check.ImportKey(siteID, filepath.Join(output, "planet.key")); err != nil {
		t.Fatal(err)
	}
	if matches, e := check.KeyMatches(siteID, identity); e != nil || !matches {
		t.Fatalf("key mismatch: %v", e)
	}
	secondApp := filepath.Join(root, "second-app")
	secondLib, err := store.Open(secondApp)
	if err != nil {
		t.Fatal(err)
	}
	secondNode := ipfs.New(exe, filepath.Join(secondApp, "ipfs"))
	if err = secondNode.Init(); err != nil {
		t.Fatal(err)
	}
	secondManager := New(secondApp, exe, secondLib, secondNode)
	secondSite, err := secondManager.Import(output)
	if err != nil {
		t.Fatalf("reimport: %v", err)
	}
	secondPosts, err := secondLib.ListArticles(siteID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = templateinstall.Install(secondApp, template, secondSite, secondPosts, secondLib); err != nil {
		t.Fatal(err)
	}
	if _, err = secondLib.SaveArticleWithTags(siteID, one, "Changed again", "Current body edited", `{"changed":"Changed label"}`); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(secondApp, "imports", siteID, "changed.html")
	if err = os.WriteFile(unknown, []byte("USER-CUSTOM"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = secondManager.Export(siteID, root); err == nil {
		t.Fatal("new tag overwrote an unknown archived file")
	}
	if b, e := os.ReadFile(unknown); e != nil || string(b) != "USER-CUSTOM" {
		t.Fatalf("unknown archive file changed: %s %v", b, e)
	}
	if err = os.Remove(unknown); err != nil {
		t.Fatal(err)
	}
	secondOutput, err := secondManager.Export(siteID, root)
	if err != nil {
		t.Fatalf("second export: %v", err)
	}
	if b, e := os.ReadFile(filepath.Join(secondOutput, "changed.html")); e != nil || !strings.Contains(string(b), "Changed again") {
		t.Fatalf("reimported tag page: %s %v", b, e)
	}
	if _, e := os.Stat(filepath.Join(secondOutput, "go.html")); !os.IsNotExist(e) {
		t.Fatalf("removed tag page retained: %v", e)
	}
	updated, err := backup.Read(secondOutput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated.Raw), `"prewarmNewPost": true`) {
		t.Fatalf("Mac setting lost after reimport: %s", updated.Raw)
	}
	for _, secret := range []string{"PRIVATE-HEAD", "PRIVATE-START", "PRIVATE-END"} {
		if !strings.Contains(string(updated.Raw), secret) {
			t.Fatalf("disabled code lost after reimport: %s", updated.Raw)
		}
	}
	if b, e := os.ReadFile(filepath.Join(secondOutput, "private-note.txt")); e != nil || string(b) != "CUSTOM PRIVATE FILE" {
		t.Fatalf("unknown backup file lost after reimport: %q %v", b, e)
	}
	if len(updated.Articles) != 1 || len(updated.Articles[0].Tags) == 0 {
		t.Fatalf("edited tags missing: %+v", updated.Articles)
	}
	if b, e := os.ReadFile(filepath.Join(secondOutput, "first", "photo.png")); e != nil || string(b) != "photo" {
		t.Fatalf("reimported attachment: %s %v", b, e)
	}
	if err = secondLib.DeleteArticle(siteID, one); err != nil {
		t.Fatal(err)
	}
	deletedOutput, err := secondManager.Export(siteID, root)
	if err != nil {
		t.Fatalf("export after tag deletion: %v", err)
	}
	if _, err = os.Stat(filepath.Join(deletedOutput, "go.html")); !os.IsNotExist(err) {
		t.Fatalf("deleted tag page retained: %v", err)
	}
	if b, e := os.ReadFile(filepath.Join(deletedOutput, "tags.html")); e != nil || strings.Contains(string(b), "go.html") || strings.Contains(string(b), "changed.html") {
		t.Fatalf("deleted tag remains in index: %s %v", b, e)
	}
	if b, e := os.ReadFile(filepath.Join(input, "planet.json")); e != nil || string(b) != raw {
		t.Fatalf("source changed: %s %v", b, e)
	}
}

func TestNativeTaggedBackupRestoresArticleLabels(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("offline Kubo required")
	}
	root := t.TempDir()
	app := filepath.Join(root, "app")
	lib, err := store.Open(app)
	if err != nil {
		t.Fatal(err)
	}
	node := ipfs.New(exe, filepath.Join(app, "ipfs"))
	if err = node.Init(); err != nil {
		t.Fatal(err)
	}
	site, err := lib.CreateSite("Native", "")
	if err != nil {
		t.Fatal(err)
	}
	post, err := lib.SaveArticleWithTags(site.ID, "", "Post", "Body", `{"中文":"中文标签","go":"Go"}`)
	if err != nil {
		t.Fatal(err)
	}
	output, err := New(app, exe, lib, node).Export(site.ID, root)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := backup.Read(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Articles) != 1 || parsed.Articles[0].Tags["中文"] != "中文标签" || parsed.Articles[0].Tags["go"] != "Go" {
		t.Fatalf("native backup tags: %+v", parsed.Articles)
	}
	if b, e := os.ReadFile(filepath.Join(output, "中文.html")); e != nil || !strings.Contains(string(b), "Post") {
		t.Fatalf("native tag page: %s %v", b, e)
	}
	if b, e := os.ReadFile(filepath.Join(output, post.ID, "index.html")); e != nil || !strings.Contains(string(b), "中文标签") {
		t.Fatalf("native post tags: %s %v", b, e)
	}
	second := filepath.Join(root, "second")
	secondLib, err := store.Open(second)
	if err != nil {
		t.Fatal(err)
	}
	secondNode := ipfs.New(exe, filepath.Join(second, "ipfs"))
	if err = secondNode.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err = New(second, exe, secondLib, secondNode).Import(output); err != nil {
		t.Fatal(err)
	}
	posts, err := secondLib.ListArticles(site.ID)
	if err != nil || len(posts) != 1 || posts[0].Tags["中文"] != "中文标签" {
		t.Fatalf("native tags after import: %+v %v", posts, err)
	}
}

func TestBackupRetainsByteIdenticalTagPages(t *testing.T) {
	const post = "22222222-2222-4222-8222-222222222222"
	stage, generated := t.TempDir(), t.TempDir()
	raw := json.RawMessage(`{"tags":{"go":"Go"},"articles":[{"id":"` + post + `","tags":{"go":"Go"}}]}`)
	for _, name := range []string{"go.html", "tags.html"} {
		for _, dir := range []string{stage, generated} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("same"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := removeObsoleteImportedFiles(stage, raw, []store.Article{{ID: post}}, generated); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"go.html", "tags.html"} {
		if b, err := os.ReadFile(filepath.Join(stage, name)); err != nil || string(b) != "same" {
			t.Fatalf("page changed: %s %v", b, err)
		}
	}
}

func TestSepiaOriginalTagPagesAllowOnlyBuildTimestampDifferences(t *testing.T) {
	const post = "22222222-2222-4222-8222-222222222222"
	raw := json.RawMessage(`{"templateName":"Sepia","tags":{"go":"Go"},"articles":[{"id":"` + post + `","tags":{"go":"Go"}}]}`)
	expected := `<link rel="apple-touch-icon" href="./avatar.png?t=" /><link rel="shortcut icon" href="./favicon.ico?t=" /><script src="./assets/sepia.js?t=" type="text/javascript"></script><p>Go</p>`
	for _, tc := range []struct {
		name, page string
		allowed    bool
	}{
		{"timestamp", strings.ReplaceAll(expected, `?t=`, `?t=1700000000`), true},
		{"other-content", strings.ReplaceAll(expected, `?t=`, `?t=1700000000`) + "PRIVATE CUSTOM", false},
		{"injection", strings.Replace(expected, `?t=`, `?t=1&amp;secret=1`, 1), false},
		{"mixed", strings.Replace(strings.ReplaceAll(expected, `?t=`, `?t=1700000000`), `1700000000`, `1700000001`, 1), false},
		{"unexpected-url", strings.ReplaceAll(strings.ReplaceAll(expected, `sepia.js`, `custom.js`), `?t=`, `?t=1700000000`), false},
		{"private-comment", strings.ReplaceAll(expected, `?t=`, `?t=1700000000`) + "<!--SECRET-->", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stage, current, original, marked := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
			path := filepath.Join(stage, "go.html")
			if err := os.WriteFile(path, []byte(tc.page), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(original, "go.html"), []byte(expected), 0600); err != nil {
				t.Fatal(err)
			}
			const marker = "TESTTIMESTAMPMARKER"
			if err := os.WriteFile(filepath.Join(marked, "go.html"), []byte(strings.ReplaceAll(expected, `?t=`, `?t=`+marker)), 0600); err != nil {
				t.Fatal(err)
			}
			err := removeObsoleteImportedFiles(stage, raw, []store.Article{{ID: post}}, current, original, marked, marker)
			if (err == nil) != tc.allowed {
				t.Fatalf("unexpected result: %v", err)
			}
			if !tc.allowed {
				if b, e := os.ReadFile(path); e != nil || string(b) != tc.page {
					t.Fatalf("original changed: %q %v", b, e)
				}
			}
		})
	}
}

func TestSepiaTimestampRecognitionDoesNotRewriteBodyOrDataAttributes(t *testing.T) {
	const post = "22222222-2222-4222-8222-222222222222"
	raw := json.RawMessage(`{"templateName":"Sepia","tags":{"go":"Go"},"articles":[{"id":"` + post + `","tags":{"go":"Go"}}]}`)
	template := `<link rel="apple-touch-icon" href="./avatar.png?t=" />`
	for _, tc := range []struct {
		name, body, archivedBody string
		allowed                  bool
	}{
		{"unchanged-data-src", `<img data-src="./avatar.png?t=123">`, `<img data-src="./avatar.png?t=123">`, true},
		{"modified-data-src", `<img data-src="./avatar.png?t=">`, `<img data-src="./avatar.png?t=1700000000">`, false},
		{"modified-body-href", `<a href="./avatar.png?t=">user</a>`, `<a href="./avatar.png?t=1700000000">user</a>`, false},
		{"marker-collision", `TESTTIMESTAMPMARKER`, `TESTTIMESTAMPMARKER`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stage, current, original, marked := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
			archived := strings.Replace(template, `?t=`, `?t=1700000000`, 1) + tc.archivedBody
			path := filepath.Join(stage, "go.html")
			if e := os.WriteFile(path, []byte(archived), 0600); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(filepath.Join(original, "go.html"), []byte(template+tc.body), 0600); e != nil {
				t.Fatal(e)
			}
			const marker = "TESTTIMESTAMPMARKER"
			if e := os.WriteFile(filepath.Join(marked, "go.html"), []byte(strings.Replace(template, `?t=`, `?t=`+marker, 1)+tc.body), 0600); e != nil {
				t.Fatal(e)
			}
			err := removeObsoleteImportedFiles(stage, raw, []store.Article{{ID: post}}, current, original, marked, marker)
			if (err == nil) != tc.allowed {
				t.Fatalf("unexpected result: %v", err)
			}
			if !tc.allowed {
				if b, e := os.ReadFile(path); e != nil || string(b) != archived {
					t.Fatalf("archived file changed: %q %v", b, e)
				}
			}
		})
	}
}

func TestSepiaTimestampedTagPagesKuboBackupRoundTrip(t *testing.T) {
	exe, templates := os.Getenv("PLANET_TEST_KUBO"), os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if exe == "" || templates == "" {
		t.Skip("offline Kubo and verified templates required")
	}
	template := filepath.Join(templates, "sepia")
	const siteID = "11111111-1111-4111-8111-111111111111"
	const postID = "22222222-2222-4222-8222-222222222222"
	root := t.TempDir()
	source := ipfs.New(exe, filepath.Join(root, "source"))
	if err := source.Init(); err != nil {
		t.Fatal(err)
	}
	identity, err := source.EnsureKey(siteID)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "input.planet")
	if err = os.MkdirAll(filepath.Join(input, postID), 0700); err != nil {
		t.Fatal(err)
	}
	if err = source.ExportKey(siteID, filepath.Join(input, "planet.key")); err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"id":"` + siteID + `","name":"Site","ipns":"` + identity + `","created":10,"updated":10,"templateName":"Sepia","plausibleAPIKey":"PRIVATE-TOKEN","tags":{"go":"Go"},"articles":[{"id":"` + postID + `","link":"/` + postID + `/","title":"Post","content":"[user link](./avatar.png?t=123)","created":10,"starType":0,"tags":{"go":"Go"},"attachments":["photo.png"]}]}`)
	if err = os.WriteFile(filepath.Join(input, "planet.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(input, postID, "photo.png"), []byte("attachment"), 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err := backup.ParseMetadata(raw)
	if err != nil {
		t.Fatal(err)
	}
	pages := filepath.Join(root, "original-pages")
	if err = os.Mkdir(pages, 0700); err != nil {
		t.Fatal(err)
	}
	if err = StageImportedPublic(input, pages, raw, parsed.Site, parsed.Articles, template); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tags.html", "go.html"} {
		data, e := os.ReadFile(filepath.Join(pages, name))
		if e != nil {
			t.Fatal(e)
		}
		page := strings.ReplaceAll(string(data), "?t=\"", "?t=1700000000\"")
		if page == string(data) {
			t.Fatalf("%s has no build timestamps", name)
		}
		if e = os.WriteFile(filepath.Join(input, name), []byte(page), 0600); e != nil {
			t.Fatal(e)
		}
		if strings.Contains(page, "PRIVATE-TOKEN") {
			t.Fatal("private token in public page")
		}
		if name == "go.html" && !strings.Contains(page, `href="./avatar.png?t=123"`) {
			t.Fatal("original article link missing")
		}
	}
	if _, err = os.Lstat(filepath.Join(pages, "planet.key")); !os.IsNotExist(err) {
		t.Fatalf("key in public output: %v", err)
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
	posts, err := lib.ListArticles(siteID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = templateinstall.Install(app, template, site, posts, lib); err != nil {
		t.Fatal(err)
	}
	if _, err = lib.SaveArticle(siteID, postID, "Updated", "After"); err != nil {
		t.Fatal(err)
	}
	output, err := m.Export(siteID, root)
	if err != nil {
		t.Fatal(err)
	}
	if b, e := os.ReadFile(filepath.Join(output, postID, "photo.png")); e != nil || string(b) != "attachment" {
		t.Fatalf("attachment: %q %v", b, e)
	}
	if b, e := os.ReadFile(filepath.Join(output, "go.html")); e != nil || !strings.Contains(string(b), "Updated") || strings.Contains(string(b), "PRIVATE-TOKEN") {
		t.Fatalf("tag page: %s %v", b, e)
	}
	check := ipfs.New(exe, filepath.Join(root, "check"))
	if err = check.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err = check.ImportKey(siteID, filepath.Join(output, "planet.key")); err != nil {
		t.Fatal(err)
	}
	if matches, e := check.KeyMatches(siteID, identity); e != nil || !matches {
		t.Fatalf("key mismatch: %v", e)
	}
	second := filepath.Join(root, "second")
	lib2, err := store.Open(second)
	if err != nil {
		t.Fatal(err)
	}
	node2 := ipfs.New(exe, filepath.Join(second, "ipfs"))
	if err = node2.Init(); err != nil {
		t.Fatal(err)
	}
	m2 := New(second, exe, lib2, node2)
	site2, err := m2.Import(output)
	if err != nil {
		t.Fatal(err)
	}
	posts2, err := lib2.ListArticles(siteID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = templateinstall.Install(second, template, site2, posts2, lib2); err != nil {
		t.Fatal(err)
	}
	if err = lib2.DeleteArticle(siteID, postID); err != nil {
		t.Fatal(err)
	}
	deleted, err := m2.Export(siteID, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(deleted, "go.html")); !os.IsNotExist(err) {
		t.Fatalf("deleted tag retained: %v", err)
	}
	if b, e := os.ReadFile(filepath.Join(input, "planet.json")); e != nil || string(b) != string(raw) {
		t.Fatalf("source changed: %s %v", b, e)
	}
	for _, name := range []string{"tags.html", "go.html"} {
		original, e := os.ReadFile(filepath.Join(input, name))
		if e != nil {
			t.Fatal(e)
		}
		retained, e := os.ReadFile(filepath.Join(app, "imports", siteID, name))
		if e != nil || string(original) != string(retained) {
			t.Fatalf("archived page changed: %s %v", name, e)
		}
	}
}

func TestRebuiltBackupRejectsUnknownTagPagesWithoutTouchingKey(t *testing.T) {
	const one = "22222222-2222-4222-8222-222222222222"
	const two = "33333333-3333-4333-8333-333333333333"
	stage := t.TempDir()
	for _, name := range []string{"old.html", "go.html", "planet.key"} {
		if err := os.WriteFile(filepath.Join(stage, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	raw := json.RawMessage(`{"tags":{"go":"Go","old":"Old"},"articles":[{"id":"` + one + `","tags":{"go":"Go"}},{"id":"` + two + `","tags":{"old":"Old"}}]}`)
	if err := removeObsoleteImportedFiles(stage, raw, []store.Article{{ID: one}}); err == nil {
		t.Fatal("unrecognized tag page accepted")
	}
	for _, name := range []string{"old.html", "go.html"} {
		if b, err := os.ReadFile(filepath.Join(stage, name)); err != nil || string(b) != name {
			t.Fatalf("tag page %s changed: %s %v", name, b, err)
		}
	}
	if b, err := os.ReadFile(filepath.Join(stage, "planet.key")); err != nil || string(b) != "planet.key" {
		t.Fatalf("key modified: %s %v", b, err)
	}
}

func TestBackupRejectsUnverifiedCustomTagPagesWithoutDeletingThem(t *testing.T) {
	const post = "22222222-2222-4222-8222-222222222222"
	raw := json.RawMessage(`{"tags":{"go":"Go"},"articles":[{"id":"` + post + `","tags":{"go":"Go"}}]}`)
	for _, name := range []string{"tags.html", "go.html"} {
		t.Run(name, func(t *testing.T) {
			stage := t.TempDir()
			path := filepath.Join(stage, name)
			if err := os.WriteFile(path, []byte("USER-CUSTOM-DATA"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := removeObsoleteImportedFiles(stage, raw, []store.Article{{ID: post}}); err == nil {
				t.Fatal("unverified user file was accepted")
			}
			if b, err := os.ReadFile(path); err != nil || string(b) != "USER-CUSTOM-DATA" {
				t.Fatalf("user file changed: %q %v", b, err)
			}
		})
	}
}
