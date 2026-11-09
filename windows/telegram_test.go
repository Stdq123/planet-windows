package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/egoist/mygo"
	"windows/internal/backup"
	"windows/internal/services"
	"windows/internal/sitegen"
	"windows/internal/store"
)

const appTelegramToken = "123456789:" + "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghi"

type appTelegramAPI struct {
	sends, edits, resolves int
	texts                  []string
}

func (a *appTelegramAPI) ResolveChat(token, target string) (services.TelegramChat, error) {
	a.resolves++
	id := target
	if target == "@example" {
		id = "-100123456789"
	}
	return services.TelegramChat{ID: id, Title: "Example", Type: "channel"}, nil
}
func (a *appTelegramAPI) Send(token, target, text string) (int64, error) {
	a.sends++
	a.texts = append(a.texts, text)
	return int64(a.sends), nil
}
func (a *appTelegramAPI) Edit(token, target string, id int64, text string) error {
	a.edits++
	a.texts = append(a.texts, text)
	return nil
}
func telegramLibrary(t *testing.T) (*Library, *appTelegramAPI, string, store.Site) {
	t.Helper()
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	api := &appTelegramAPI{}
	lib := &Library{telegramAPI: api}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	return lib, api, root, site
}

func TestTelegramConfigurationIsPrivateAndSiteScoped(t *testing.T) {
	lib, api, root, site := telegramLibrary(t)
	other, err := lib.CreateSite("Other", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = lib.ConfigureTelegram(site.ID, "@example", appTelegramToken); err != nil {
		t.Fatal(err)
	}
	if api.sends != 0 || api.edits != 0 {
		t.Fatal("configuration sent a message")
	}
	info, err := lib.TelegramInfo(site.ID)
	if err != nil || !info.Configured || info.ChatID != "-100123456789" {
		t.Fatalf("info: %+v %v", info, err)
	}
	raw, _ := json.Marshal(info)
	if strings.Contains(string(raw), appTelegramToken) || strings.Contains(string(raw), "token") {
		t.Fatal("secret exposed to UI")
	}
	otherInfo, err := lib.TelegramInfo(other.ID)
	if err != nil || otherInfo.Configured {
		t.Fatalf("cross-site config: %+v %v", otherInfo, err)
	}
	data, err := os.ReadFile(filepath.Join(root, "integrations", "telegram", site.ID+".dat"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), appTelegramToken) {
		t.Fatal("plaintext token on disk")
	}
	if err = lib.ConfigureTelegram(site.ID, "-100999", ""); err == nil {
		t.Fatal("saved token reused for changed destination")
	}
	if err = lib.DisableTelegram(site.ID); err != nil {
		t.Fatal(err)
	}
	info, err = lib.TelegramInfo(site.ID)
	if err != nil || info.Configured {
		t.Fatalf("disable: %+v %v", info, err)
	}
}

func TestTelegramRetryRequiresPublishedSnapshotAndSkipsDrafts(t *testing.T) {
	lib, api, _, site := telegramLibrary(t)
	post, err := lib.SaveArticle(site.ID, "", "Published title", "Public **summary**.")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lib.SaveDraft(site.ID, "", "", "Private draft", "Do not send this"); err != nil {
		t.Fatal(err)
	}
	if err = lib.ConfigureTelegram(site.ID, "@example", appTelegramToken); err != nil {
		t.Fatal(err)
	}
	if _, err = lib.RetryTelegram(site.ID); err == nil || api.sends != 0 {
		t.Fatal("unpublished content sent")
	}
	s, _ := lib.open()
	snapshot, _, _, _ := s.PublicationSnapshot(site.ID)
	if _, err = s.SetPublishedRevision(site.ID, "k51-test", "bafy-test", snapshot.ContentRevision); err != nil {
		t.Fatal(err)
	}
	if n, err := lib.RetryTelegram(site.ID); err != nil || n != 1 {
		t.Fatalf("retry: %d %v", n, err)
	}
	if len(api.texts) != 1 || !strings.Contains(api.texts[0], "Public summary.") || !strings.Contains(api.texts[0], "/"+post.ID+"/") {
		t.Fatalf("message: %q", api.texts)
	}
	if n, err := lib.RetryTelegram(site.ID); err != nil || n != 0 {
		t.Fatalf("duplicate: %d %v", n, err)
	}
	if _, err = lib.SaveArticle(site.ID, post.ID, "Unpublished secret", "Private change"); err != nil {
		t.Fatal(err)
	}
	if _, err = lib.RetryTelegram(site.ID); err == nil || api.edits != 0 {
		t.Fatal("unpublished update sent")
	}
}

func TestTelegramPostsUsePlainSummaryAndCanonicalLinks(t *testing.T) {
	posts, err := telegramPosts(store.Site{ID: "site", Domain: "example.eth"}, []store.Article{
		{ID: "post", SiteID: "site", Slug: "hello", Title: "A <title>", Content: "Hello **world** &amp; [link](https://example.com).\n\n<script>secret</script>\n\nNext."},
		{ID: "page", SiteID: "site", ArticleType: 1, Title: "Page"},
	})
	if err != nil || len(posts) != 1 {
		t.Fatalf("posts: %+v %v", posts, err)
	}
	text := posts[0].Text
	for _, want := range []string{"A <title>", "Hello world & link.", "Next.", "https://example.eth.limo/hello/"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %q", want, text)
		}
	}
	if strings.Contains(text, "secret") || strings.Contains(text, "**") {
		t.Fatalf("markup leaked: %s", text)
	}
	posts, err = telegramPosts(store.Site{ID: "site", IPNS: "k51-test"}, []store.Article{{ID: "post", SiteID: "site", Title: strings.Repeat("😀", 5000), Content: strings.Repeat("😀", 5000)}})
	if err != nil || len(utf16.Encode([]rune(posts[0].Text))) > 4096 || !strings.HasSuffix(posts[0].Text, "/post/") {
		t.Fatalf("oversized message: %v", err)
	}
}

func TestTelegramPublicationUsesCapturedSnapshot(t *testing.T) {
	lib, api, root, site := telegramLibrary(t)
	post, err := lib.SaveArticle(site.ID, "", "Public title", "Public body")
	if err != nil {
		t.Fatal(err)
	}
	if err = lib.ConfigureTelegram(site.ID, "@example", appTelegramToken); err != nil {
		t.Fatal(err)
	}
	s, _ := lib.open()
	snapshot, articles, _, err := s.PublicationSnapshot(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.IPNS = "k51-test"
	if _, err = lib.SaveArticle(site.ID, post.ID, "Unpublished secret", "Private change"); err != nil {
		t.Fatal(err)
	}
	if warning := lib.syncPublishedTelegram(root, snapshot, articles, nil); warning != "" {
		t.Fatal(warning)
	}
	if api.sends != 1 || strings.Contains(api.texts[0], "secret") || !strings.Contains(api.texts[0], "Public title") {
		t.Fatalf("wrong snapshot: %q", api.texts)
	}
	dir := t.TempDir()
	if err = sitegen.Render(dir, snapshot, articles); err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		if strings.Contains(string(data), appTelegramToken) || strings.Contains(string(data), "-100123456789") {
			t.Errorf("private config in public file %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTelegramTargetChangesResetHistoryAndDeletionCleansConfig(t *testing.T) {
	lib, _, root, site := telegramLibrary(t)
	if err := lib.ConfigureTelegram(site.ID, "@example", appTelegramToken); err != nil {
		t.Fatal(err)
	}
	cfg, _ := services.LoadTelegramConfig(root, site.ID)
	cfg.Messages = map[string]services.TelegramMessage{"post": {MessageID: 42, Hash: strings.Repeat("a", 64)}}
	if err := services.SaveTelegramConfig(root, site.ID, cfg); err != nil {
		t.Fatal(err)
	}
	rotated := "123456789:" + strings.Repeat("b", 35)
	if err := lib.ConfigureTelegram(site.ID, cfg.ChatID, rotated); err != nil {
		t.Fatal(err)
	}
	info, _ := lib.TelegramInfo(site.ID)
	if info.SyncedCount != 1 {
		t.Fatal("token rotation erased history")
	}
	if err := lib.ConfigureTelegram(site.ID, "-100987654321", rotated); err != nil {
		t.Fatal(err)
	}
	info, _ = lib.TelegramInfo(site.ID)
	if info.SyncedCount != 0 || info.ChatID != "-100987654321" {
		t.Fatalf("old destination history retained: %+v", info)
	}
	if err := lib.RemoveSite(site.ID); err != nil {
		t.Fatal(err)
	}
	if err := lib.PermanentlyDeleteSite(site.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "integrations", "telegram", site.ID+".dat")); !os.IsNotExist(err) {
		t.Fatalf("config survived deletion: %v", err)
	}
}

func TestTelegramImportedArticleUsesPublishedSlug(t *testing.T) {
	raw := []byte(`{"id":"11111111-1111-4111-8111-111111111111","name":"Imported","ipns":"k51-test","created":0,"templateName":"Plain","articles":[{"id":"22222222-2222-4222-8222-222222222222","title":"Imported post","content":"Body","slug":"hello","created":0}]}`)
	parsed, err := backup.ParseMetadata(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Articles[0].Slug != "" {
		t.Fatal("fixture must exercise raw metadata overlay")
	}
	posts, err := telegramSnapshotPosts(parsed.Site, parsed.Articles, parsed.Raw)
	if err != nil || len(posts) != 1 || !strings.HasSuffix(posts[0].Text, "/hello/") {
		t.Fatalf("imported canonical link: %+v %v", posts, err)
	}
	if parsed.Articles[0].Slug != "" {
		t.Fatal("mutated source snapshot")
	}
}

func TestTelegramCorruptConfigurationCanBeDisabled(t *testing.T) {
	lib, _, root, site := telegramLibrary(t)
	if err := lib.ConfigureTelegram(site.ID, "@example", appTelegramToken); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "integrations", "telegram", site.ID+".dat")
	if err := os.WriteFile(path, []byte("invalid encrypted configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.TelegramInfo(site.ID); err == nil {
		t.Fatal("corrupt configuration accepted")
	}
	if err := lib.DisableTelegram(site.ID); err != nil {
		t.Fatal(err)
	}
	info, err := lib.TelegramInfo(site.ID)
	if err != nil || info.Configured {
		t.Fatalf("recovery failed: %+v %v", info, err)
	}
}
