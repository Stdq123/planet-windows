package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"windows/internal/store"
)

func TestImportedPublicationRebuildsPagesAndCopiesOnlyDeclaredAttachments(t *testing.T) {
	template := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if template == "" {
		t.Skip("verified original Plain template required")
	}
	root := t.TempDir()
	archive := filepath.Join(root, "archive")
	out := filepath.Join(root, "public")
	const id = "11111111-1111-4111-8111-111111111111"
	const post = "22222222-2222-4222-8222-222222222222"
	if err := os.MkdirAll(filepath.Join(archive, post), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(archive, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	raw := `{"id":"` + id + `","name":"Site","about":"","ipns":"k51test","created":10,"updated":20,"templateName":"Plain","filebaseEnabled":false,"filebaseAPIToken":"SECRET_TOKEN","articles":[{"id":"` + post + `","link":"/` + post + `/","title":"Old","content":"Old body","created":10,"starType":0,"articleNumber":7,"attachments":["picture.png"]}]}`
	write("planet.json", raw)
	write("index.html", "STALE SITE")
	write(filepath.Join(post, "index.html"), "STALE POST")
	write(filepath.Join(post, "picture.png"), "image bytes")
	write(filepath.Join(post, "credentials.json"), "SECRET_EXTRA")
	write(filepath.Join(post, "article.json"), `{"id":"`+post+`","title":"Old","content":"Old body","articleReference":"BLOG-7","attachments":["picture.png"]}`)
	site := store.Site{ID: id, Name: "Site", IPNS: "k51test", Imported: true, Created: time.Now()}
	articles := []store.Article{{ID: post, SiteID: id, Title: "New title", Content: "New body", Created: time.Now(), Modified: time.Now()}}
	if err := StageImportedPublic(archive, out, json.RawMessage(raw), site, articles, template); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(out, post, "index.html"))
	if err != nil || !strings.Contains(string(page), "New body") || strings.Contains(string(page), "STALE POST") {
		t.Fatalf("stale page=%s err=%v", page, err)
	}
	b, err := os.ReadFile(filepath.Join(out, "planet.json"))
	if err != nil || strings.Contains(string(b), "SECRET_TOKEN") {
		t.Fatalf("private metadata=%s err=%v", b, err)
	}
	if !strings.Contains(string(b), `"articleNumber": 7`) || !strings.Contains(string(b), `"articleReference": "BLOG-7"`) {
		t.Fatalf("article number lost: %s", b)
	}
	b, err = os.ReadFile(filepath.Join(out, post, "picture.png"))
	if err != nil || string(b) != "image bytes" {
		t.Fatalf("attachment=%q err=%v", b, err)
	}
	if _, err = os.Stat(filepath.Join(out, post, "credentials.json")); !os.IsNotExist(err) {
		t.Fatalf("unregistered file exposed: %v", err)
	}
	if _, err = os.Stat(filepath.Join(out, "planet.key")); !os.IsNotExist(err) {
		t.Fatalf("key exposed: %v", err)
	}
	for _, tc := range []struct{ name, metadata string }{
		{"enabled integration", strings.Replace(raw, `"filebaseEnabled":false`, `"filebaseEnabled":true`, 1)},
		{"unsafe attachment", strings.Replace(raw, `"picture.png"`, `"../secret"`, 1)},
		{"unsupported slug", strings.Replace(raw, `"starType":0`, `"starType":0,"slug":"custom"`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "out")
			if err := os.Mkdir(target, 0700); err != nil {
				t.Fatal(err)
			}
			if err := StageImportedPublic(archive, target, json.RawMessage(tc.metadata), site, articles, template); err == nil {
				t.Fatal("unsafe input accepted")
			}
		})
	}
	os.Remove(filepath.Join(archive, post, "picture.png"))
	other := filepath.Join(t.TempDir(), "out")
	os.Mkdir(other, 0700)
	if err := StageImportedPublic(archive, other, json.RawMessage(raw), site, articles, template); err == nil {
		t.Fatal("missing attachment accepted")
	}
}
