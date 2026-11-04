package migration

import (
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"windows/internal/backup"
	"windows/internal/store"
)

func TestImportedEmptyTemplateSettingsArePublicButUnknownSettingsAreRejected(t *testing.T) {
	template := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if template == "" {
		t.Skip("verified original template required")
	}
	const id = "11111111-1111-4111-8111-111111111111"
	raw := []byte(`{"id":"` + id + `","name":"Site","about":"","templateName":"Plain","articles":[]}`)
	site := store.Site{ID: id, Name: "Site", Imported: true, Created: time.Now()}
	for _, tc := range []struct {
		name, data string
		accepted   bool
	}{
		{"empty", "{ \n }", true}, {"unknown", `{"token":"SECRET"}`, false}, {"null", "null", false}, {"array", "[]", false}, {"malformed", "{", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			archive := filepath.Join(root, "imports", id)
			if err := os.MkdirAll(archive, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(archive, "templateSettings.json"), []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(root, "out")
			if err := os.Mkdir(out, 0700); err != nil {
				t.Fatal(err)
			}
			err := StageImportedPublic(archive, out, raw, site, nil, template)
			if !tc.accepted {
				if err == nil {
					t.Fatal("unsafe settings accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(filepath.Join(out, "templateSettings.json"))
			if err != nil || strings.TrimSpace(string(b)) != "{}" {
				t.Fatalf("settings=%q err=%v", b, err)
			}
		})
	}
}

func TestImportedSepiaSettingsApplyAndRejectUnknownOrUnsafeValues(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("original templates unavailable")
	}
	const id = "11111111-1111-4111-8111-111111111111"
	raw := []byte(`{"id":"` + id + `","name":"Site","about":"","templateName":"Sepia","articles":[]}`)
	site := store.Site{ID: id, Name: "Site", Imported: true, Created: time.Now()}
	for _, tc := range []struct {
		name, settings string
		accepted       bool
	}{
		{"valid", `{"backgroundColor":"#123456","showRef":true}`, true},
		{"unknown", `{"token":"SECRET"}`, false},
		{"injection", `{"backgroundColor":"#fff;}</style><script>evil()</script>"}`, false},
		{"invalidBoolean", `{"showRef":"maybe"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "imports", id)
			if err := os.MkdirAll(archive, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(archive, "templateSettings.json"), []byte(tc.settings), 0600); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, "out")
			if err := os.Mkdir(out, 0700); err != nil {
				t.Fatal(err)
			}
			err := StageImportedPublic(archive, out, raw, site, nil, filepath.Join(root, "sepia"))
			if !tc.accepted {
				if err == nil {
					t.Fatal("unsafe settings accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			index, err := os.ReadFile(filepath.Join(out, "index.html"))
			if err != nil || !strings.Contains(string(index), "--background-color: #123456;") {
				t.Fatalf("settings not applied: %v", err)
			}
			settings, err := os.ReadFile(filepath.Join(out, "templateSettings.json"))
			if err != nil || !strings.Contains(string(settings), "#123456") {
				t.Fatalf("settings not exported: %s %v", settings, err)
			}
		})
	}
}

func TestSwitchingImportedSepiaTemplateDoesNotPublishOldSettings(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("original templates unavailable")
	}
	const id = "11111111-1111-4111-8111-111111111111"
	raw := []byte(`{"id":"` + id + `","name":"Site","about":"","templateName":"Sepia","articles":[]}`)
	site := store.Site{ID: id, Name: "Site", Imported: true, Created: time.Now()}
	dir := t.TempDir()
	archive := filepath.Join(dir, "imports", id)
	if err := os.MkdirAll(archive, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(archive, "templateSettings.json")
	if err := os.WriteFile(path, []byte(`{"backgroundColor":"#123456"}`), 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	if err := StageImportedPublic(archive, out, raw, site, nil, filepath.Join(root, "plain")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "templateSettings.json")); !os.IsNotExist(err) {
		t.Fatalf("old settings exposed: %v", err)
	}
	if b, err := os.ReadFile(path); err != nil || !strings.Contains(string(b), "#123456") {
		t.Fatalf("archive changed: %s %v", b, err)
	}
}

func TestImportedAvatarAndFaviconAppearInRebuiltSite(t *testing.T) {
	template := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if template == "" {
		t.Skip("verified original Plain template required")
	}
	root := t.TempDir()
	const id = "11111111-1111-4111-8111-111111111111"
	archive, out := filepath.Join(root, "imports", id), filepath.Join(root, "out")
	if err := os.MkdirAll(archive, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"avatar.png", "favicon.ico"} {
		f, err := os.Create(filepath.Join(archive, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	raw := `{"id":"` + id + `","name":"Site","about":"","templateName":"Plain","articles":[]}`
	site := store.Site{ID: id, Name: "Site", Imported: true, Created: time.Now()}
	if err := StageImportedPublic(archive, out, []byte(raw), site, nil, template); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"avatar.png", "favicon.ico"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatal(err)
		}
	}
	page, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil || !strings.Contains(string(page), `src="./avatar.png"`) {
		t.Fatalf("avatar missing from page: %v", err)
	}
}

func TestImportedSiteSlugProducesOriginalArticleReference(t *testing.T) {
	template := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if template == "" {
		t.Skip("verified original Plain template required")
	}
	root := t.TempDir()
	const id = "11111111-1111-4111-8111-111111111111"
	const post = "22222222-2222-4222-8222-222222222222"
	archive, out := filepath.Join(root, "imports", id), filepath.Join(root, "out")
	if err := os.MkdirAll(filepath.Join(archive, post), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archive, post, "article.json"), []byte(`{"id":"`+post+`","articleReference":"OLD-7"}`), 0600); err != nil {
		t.Fatal(err)
	}
	raw := `{"id":"` + id + `","name":"Site","about":"","slug":"journal","templateName":"Plain","articles":[{"id":"` + post + `","link":"/` + post + `/","title":"Post","content":"Body","created":10,"starType":0,"articleNumber":7}]}`
	site := store.Site{ID: id, Name: "Site", Imported: true, Created: time.Now()}
	article := store.Article{ID: post, SiteID: id, Title: "Post", Content: "Body", Created: time.Now(), Modified: time.Now()}
	if err := StageImportedPublic(archive, out, []byte(raw), site, []store.Article{article}, template); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, post, "article.json"))
	if err != nil || !strings.Contains(string(b), `"articleReference": "JOURNAL-7"`) {
		t.Fatalf("missing article reference: %s %v", b, err)
	}
}

func TestImportedSiteCanRebuildUpdatedNameAndDescription(t *testing.T) {
	template := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if template == "" {
		t.Skip("verified original template required")
	}
	root := t.TempDir()
	const id = "11111111-1111-4111-8111-111111111111"
	archive, out := filepath.Join(root, "imports", id), filepath.Join(root, "out")
	if err := os.MkdirAll(archive, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	raw := `{"id":"` + id + `","name":"Old name","about":"Old description","templateName":"Plain","articles":[]}`
	site := store.Site{ID: id, Name: "New name", About: "New description", Imported: true, Created: time.Now()}
	if err := StageImportedPublic(archive, out, []byte(raw), site, nil, template); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil || !strings.Contains(string(page), "New name") || !strings.Contains(string(page), "New description") {
		t.Fatalf("old site information: %v", err)
	}
	metadata, err := os.ReadFile(filepath.Join(out, "planet.json"))
	if err != nil || !strings.Contains(string(metadata), `"name": "New name"`) {
		t.Fatalf("old public metadata: %s %v", metadata, err)
	}
}

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
	raw := `{"id":"` + id + `","name":"Site","about":"","slug":"blog","ipns":"k51test","created":10,"updated":20,"templateName":"Plain","filebaseEnabled":false,"filebaseAPIToken":"SECRET_TOKEN","articles":[{"id":"` + post + `","link":"/` + post + `/","title":"Old","content":"Old body","created":10,"starType":0,"articleNumber":7,"attachments":["picture.png"]}]}`
	write("planet.json", raw)
	write("index.html", "STALE SITE")
	write(filepath.Join(post, "index.html"), "STALE POST")
	write(filepath.Join(post, "picture.png"), "image bytes")
	write(filepath.Join(post, "credentials.json"), "SECRET_EXTRA")
	write(filepath.Join(post, "article.json"), `{"id":"`+post+`","title":"Old","content":"Old body","articleReference":"BLOG-7","attachments":["picture.png"]}`)
	site := store.Site{ID: id, Name: "Site", IPNS: "k51test", Imported: true, Created: time.Now()}
	articles := []store.Article{{ID: post, SiteID: id, Title: "New title", Content: "New body", Created: time.Now(), Modified: time.Now(), Attachments: []string{"picture.png"}}}
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
		{"unsupported integration", strings.Replace(raw, `"filebaseEnabled":false`, `"filebaseEnabled":false,"cloudflarePagesEnabled":true`, 1)},
		{"unsafe attachment", strings.Replace(raw, `"picture.png"`, `"../secret"`, 1)},
		{"unsafe slug", strings.Replace(raw, `"starType":0`, `"starType":0,"slug":"../escape"`, 1)},
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

func TestImportedPinningConfigurationDoesNotEnterPublicSite(t *testing.T) {
	template := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if template == "" {
		t.Skip("original template required")
	}
	const id = "11111111-1111-4111-8111-111111111111"
	dir := t.TempDir()
	archive := filepath.Join(dir, "imports", id)
	if err := os.MkdirAll(archive, 0700); err != nil {
		t.Fatal(err)
	}
	const secret = "https://pinnable.xyz/pin/SECRET-ENDPOINT"
	raw := []byte(`{"id":"` + id + `","name":"Site","about":"","templateName":"Plain","pinnableEnabled":true,"pinnableAPIEndpoint":"` + secret + `","pinnablePinCID":"bafyold","filebaseEnabled":true,"filebasePinName":"site","filebaseAPIToken":"FILEBASE-PRIVATE","articles":[]}`)
	site := store.Site{ID: id, Name: "Site", Imported: true, Created: time.Now()}
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	if err := StageImportedPublic(archive, out, raw, site, nil, template); err != nil {
		t.Fatal(err)
	}
	if err := filepath.WalkDir(out, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		if strings.Contains(string(b), "SECRET-ENDPOINT") || strings.Contains(string(b), "FILEBASE-PRIVATE") {
			t.Fatalf("credential leaked into %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestImportedPlausibleTrackingUsesPublicSettingsWithoutAPIKey(t *testing.T) {
	template := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if template == "" {
		t.Skip("original template required")
	}
	const id = "11111111-1111-4111-8111-111111111111"
	for _, tc := range []struct {
		name, server, domain string
		valid                bool
	}{
		{"valid", "plausible.io", "example.com", true}, {"unsafe server", "https://evil.test/path", "example.com", false}, {"unsafe domain", "plausible.io", `example.com\" onload=\"evil`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "imports", id)
			if err := os.MkdirAll(archive, 0700); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, "out")
			if err := os.Mkdir(out, 0700); err != nil {
				t.Fatal(err)
			}
			meta := map[string]any{"id": id, "name": "Site", "about": "", "templateName": "Plain", "articles": []any{}, "plausibleEnabled": true, "plausibleDomain": tc.domain, "plausibleAPIServer": tc.server, "plausibleAPIKey": "PRIVATE-TOKEN"}
			raw, _ := json.Marshal(meta)
			site := store.Site{ID: id, Name: "Site", Imported: true, Created: time.Now()}
			err := StageImportedPublic(archive, out, raw, site, nil, template)
			if !tc.valid {
				if err == nil {
					t.Fatal("unsafe analytics accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			page, err := os.ReadFile(filepath.Join(out, "index.html"))
			if err != nil || !strings.Contains(string(page), `data-domain="example.com"`) || !strings.Contains(string(page), "https://plausible.io/js/plausible.local.js") {
				t.Fatalf("tracking missing: %v", err)
			}
			if err := filepath.WalkDir(out, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				b, e := os.ReadFile(path)
				if e != nil {
					return e
				}
				if strings.Contains(string(b), "PRIVATE-TOKEN") {
					t.Fatalf("API key leaked into %s", path)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestImportedPlausibleTrackingCanBeDisabled(t *testing.T) {
	template := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if template == "" {
		t.Skip("original template required")
	}
	dir := t.TempDir()
	archive := filepath.Join(dir, "archive")
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(archive, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	const id = "11111111-1111-4111-8111-111111111111"
	raw := []byte(`{"id":"` + id + `","name":"Site","about":"","templateName":"Plain","plausibleEnabled":true,"plausibleDomain":"old.example","plausibleAPIServer":"stats.example","articles":[]}`)
	site := store.Site{ID: id, Name: "Site", Imported: true, PlausibleOverride: &store.PlausibleTracking{Enabled: false}}
	if err := StageImportedPublic(archive, out, raw, site, nil, template); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page), "plausible.local.js") {
		t.Fatal("tracking still enabled")
	}
}

func TestImportedPlausibleCustomServerRendersSafely(t *testing.T) {
	template := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if template == "" {
		t.Skip("original template required")
	}
	dir := t.TempDir()
	archive := filepath.Join(dir, "archive")
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(archive, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	id := "11111111-1111-4111-8111-111111111111"
	raw := []byte(`{"id":"` + id + `","name":"Site","about":"","templateName":"Plain","plausibleEnabled":true,"plausibleDomain":"example.com","plausibleAPIServer":"stats.example.com:8443","plausibleAPIKey":"PRIVATE-TOKEN","articles":[]}`)
	site := store.Site{ID: id, Name: "Site", Imported: true, Created: time.Now()}
	if err := StageImportedPublic(archive, out, raw, site, nil, template); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "https://stats.example.com:8443/js/plausible.local.js") || strings.Contains(string(page), "PRIVATE-TOKEN") {
		t.Fatal("incorrect tracking or leaked token")
	}
}

func TestRebuiltBackupKeepsPreviousPlausibleCredentialPrivate(t *testing.T) {
	template := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if template == "" {
		t.Skip("original template required")
	}
	dir := t.TempDir()
	archive := filepath.Join(dir, "archive")
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(archive, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	id := "11111111-1111-4111-8111-111111111111"
	original := []byte(`{"id":"` + id + `","name":"Site","about":"","ipns":"k51example","created":10,"templateName":"Plain","plausibleEnabled":true,"plausibleDomain":"old.example","plausibleAPIServer":"old.example","plausibleAPIKey":"PRIVATE-OLD-KEY","articles":[]}`)
	site := store.Site{ID: id, Name: "Site", IPNS: "k51example", Imported: true, Created: time.Now(), PlausibleOverride: &store.PlausibleTracking{Enabled: true, Domain: "new.example", Server: "https://new.example"}}
	raw, err := backup.Merge(original, site, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "PRIVATE-OLD-KEY") {
		t.Fatal("original backup secret not preserved")
	}
	if err = StageImportedPublic(archive, out, raw, site, nil, template); err != nil {
		t.Fatal(err)
	}
	if err = filepath.WalkDir(out, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		b, e := os.ReadFile(path)
		if e == nil && strings.Contains(string(b), "PRIVATE-OLD-KEY") {
			t.Fatalf("secret leaked: %s", path)
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
}

func TestImportedTemplateUsesCustomServerForModernScript(t *testing.T) {
	template := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if template == "" {
		t.Skip("original template required")
	}
	dir := t.TempDir()
	archive := filepath.Join(dir, "archive")
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(archive, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	id := "11111111-1111-4111-8111-111111111111"
	raw := []byte(`{"id":"` + id + `","name":"Site","about":"","templateName":"Plain","articles":[]}`)
	site := store.Site{ID: id, Name: "Site", Imported: true, Created: time.Now(), PlausibleOverride: &store.PlausibleTracking{Enabled: true, Domain: "example.com", ScriptID: "pa-Test123", Server: "https://stats.example.com:8443"}}
	if err := StageImportedPublic(archive, out, raw, site, nil, template); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "https://stats.example.com:8443/js/pa-Test123.js") || strings.Contains(string(page), "plausible.local.js") {
		t.Fatal("incorrect modern script")
	}
}

func TestImportedTemplateUsesSiteSpecificPlausibleScript(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("original templates required")
	}
	for _, name := range []string{"plain", "gamedb", "grid", "platinum", "sepia"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "archive")
			out := filepath.Join(dir, "out")
			if err := os.Mkdir(archive, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(out, 0700); err != nil {
				t.Fatal(err)
			}
			id := "11111111-1111-4111-8111-111111111111"
			post := "22222222-2222-4222-8222-222222222222"
			raw := []byte(`{"id":"` + id + `","name":"Site","about":"","templateName":"Plain","plausibleEnabled":true,"plausibleDomain":"old.example","plausibleAPIKey":"PRIVATE-SECRET","articles":[]}`)
			site := store.Site{ID: id, Name: "Site", Imported: true, Created: time.Now(), PlausibleOverride: &store.PlausibleTracking{Enabled: true, Domain: "example.com", ScriptID: "pa-Test123"}}
			article := store.Article{ID: post, SiteID: id, Title: "Post", Created: time.Now()}
			if err := StageImportedPublic(archive, out, raw, site, []store.Article{article}, filepath.Join(root, name)); err != nil {
				t.Fatal(err)
			}
			paths := []string{"index.html", "page1.html", post + "/index.html"}
			if _, err := os.Stat(filepath.Join(out, "archive.html")); err == nil {
				paths = append(paths, "archive.html")
			}
			for _, path := range paths {
				b, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(path)))
				if err != nil {
					t.Fatal(err)
				}
				page := string(b)
				if strings.Count(page, "https://plausible.io/js/pa-Test123.js") != 1 || !strings.Contains(page, "plausible.init()") || strings.Contains(page, "plausible.local.js") || strings.Contains(page, "PRIVATE-SECRET") {
					t.Fatalf("incorrect %s tracking", path)
				}
			}
		})
	}
}

func TestImportedSiteSupportsVerifiedTemplates(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("verified templates required")
	}
	const id = "11111111-1111-4111-8111-111111111111"
	const post = "22222222-2222-4222-8222-222222222222"
	for _, tc := range []struct{ dir, name string }{{"plain", "Plain"}, {"gamedb", "8-bit"}, {"grid", "Grid"}, {"platinum", "Platinum"}, {"sepia", "Sepia"}} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			archive := filepath.Join(tmp, "imports", id)
			os.MkdirAll(filepath.Join(archive, post), 0700)
			raw := `{"id":"` + id + `","name":"Site","about":"","templateName":"` + tc.name + `","articles":[{"id":"` + post + `","link":"/` + post + `/","title":"Post","content":"Body","created":10,"starType":0}]}`
			site := store.Site{ID: id, Name: "Site", Imported: true, Created: time.Now()}
			article := store.Article{ID: post, SiteID: id, Title: "Post", Content: "Body", Created: time.Now(), Modified: time.Now()}
			out := filepath.Join(tmp, "out")
			os.Mkdir(out, 0700)
			if err := StageImportedPublic(archive, out, []byte(raw), site, []store.Article{article}, filepath.Join(root, tc.dir)); err != nil {
				t.Fatal(err)
			}
			if b, err := os.ReadFile(filepath.Join(out, post, "index.html")); err != nil || !strings.Contains(string(b), "Body") {
				t.Fatalf("page: %s %v", b, err)
			}
		})
	}
}

func TestImportedMediaRebuildPreservesAudioVideoAndThumbnail(t *testing.T) {
	template := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if template == "" {
		t.Skip("verified template required")
	}
	root := t.TempDir()
	const id = "11111111-1111-4111-8111-111111111111"
	const post = "22222222-2222-4222-8222-222222222222"
	archive := filepath.Join(root, "imports", id)
	os.MkdirAll(filepath.Join(archive, post), 0700)
	raw := `{"id":"` + id + `","name":"Site","about":"","templateName":"Plain","articles":[{"id":"` + post + `","link":"/` + post + `/","title":"Media","content":"Body","created":10,"starType":0,"audioFilename":"song.mp3","videoFilename":"clip.mp4","attachments":["song.mp3","clip.mp4"]}]}`
	os.WriteFile(filepath.Join(archive, post, "song.mp3"), []byte("audio"), 0600)
	os.WriteFile(filepath.Join(archive, post, "clip.mp4"), []byte("video"), 0600)
	os.WriteFile(filepath.Join(archive, post, "_videoThumbnail.png"), []byte("thumbnail"), 0600)
	os.WriteFile(filepath.Join(archive, post, "article.json"), []byte(`{"id":"`+post+`","hasAudio":true,"audioFilename":"song.mp3","audioDuration":12,"audioByteLength":5,"hasVideo":true,"videoFilename":"clip.mp4","attachments":["song.mp3","clip.mp4"]}`), 0600)
	site := store.Site{ID: id, Name: "Site", Imported: true, Created: time.Now()}
	a := store.Article{ID: post, SiteID: id, Title: "Media", Content: "Body", Created: time.Now(), Modified: time.Now(), AudioFilename: "song.mp3", VideoFilename: "clip.mp4", Attachments: []string{"song.mp3", "clip.mp4"}}
	out := filepath.Join(root, "out")
	os.Mkdir(out, 0700)
	if err := StageImportedPublic(archive, out, []byte(raw), site, []store.Article{a}, template); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"song.mp3", "clip.mp4", "_videoThumbnail.png"} {
		if _, err := os.Stat(filepath.Join(out, post, name)); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(filepath.Join(out, post, "article.json"))
	if err != nil || !strings.Contains(string(b), `"audioDuration": 12`) || !strings.Contains(string(b), `"hasVideo": true`) {
		t.Fatalf("metadata=%s %v", b, err)
	}
	page, err := os.ReadFile(filepath.Join(out, post, "index.html"))
	if err != nil || !strings.Contains(string(page), "song.mp3") || !strings.Contains(string(page), "clip.mp4") {
		t.Fatalf("page=%s %v", page, err)
	}
}
