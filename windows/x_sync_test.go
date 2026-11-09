package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"windows/internal/services"
	"windows/internal/store"
)

type fakeXPoster struct {
	calls  int
	text   string
	err    error
	before func()
}

func (p *fakeXPoster) CreateXPost(_ services.XCredentials, text string) (string, error) {
	p.calls++
	p.text = text
	if p.before != nil {
		p.before()
	}
	if p.err != nil {
		return "", p.err
	}
	return "123456789", nil
}

func xTestLibrary(t *testing.T) (*Library, store.Site, string, *fakeXPoster) {
	t.Helper()
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	p := &fakeXPoster{}
	l := &Library{xPoster: p}
	site, err := l.CreateSite("X site", "")
	if err != nil {
		t.Fatal(err)
	}
	return l, site, root, p
}

func configureTestX(t *testing.T, l *Library, siteID string, auto bool) {
	t.Helper()
	if err := l.ConfigureXSync(siteID, "api-key", "api-secret", "access-token", "access-secret", auto); err != nil {
		t.Fatal(err)
	}
}

func publishXSnapshot(t *testing.T, l *Library, id string) (store.Site, []store.Article) {
	t.Helper()
	s, err := l.open()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetIdentity(id, "test-ipns"); err != nil {
		t.Fatal(err)
	}
	site, articles, _, err := s.PublicationSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetPublishedRevision(id, site.IPNS, "test-cid", site.ContentRevision); err != nil {
		t.Fatal(err)
	}
	site, articles, _, err = s.PublicationSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	return site, articles
}

func TestXConfigurationPreservesCredentialsAndNeverReturnsThem(t *testing.T) {
	l, site, root, _ := xTestLibrary(t)
	configureTestX(t, l, site.ID, true)
	before, err := services.LoadConfig(root, site.ID, "x")
	if err != nil {
		t.Fatal(err)
	}
	if err = l.ConfigureXSync(site.ID, "", "", "", "", false); err != nil {
		t.Fatal(err)
	}
	after, err := services.LoadConfig(root, site.ID, "x")
	if err != nil {
		t.Fatal(err)
	}
	if after.XCredentials == nil || *after.XCredentials != *before.XCredentials || after.AutoSync {
		t.Fatal("credentials not preserved")
	}
	if err = l.ConfigureXSync(site.ID, "partial", "", "", "", true); err == nil {
		t.Fatal("partial credentials accepted")
	}
	info, err := l.XSyncInfo(site.ID)
	if err != nil || !info.Configured || info.AutoSync {
		t.Fatalf("info: %#v %v", info, err)
	}
	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"api-key", "api-secret", "access-token", "access-secret"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("secret returned to frontend")
		}
	}
	if err = l.DisableXSync(site.ID); err != nil {
		t.Fatal(err)
	}
	info, err = l.XSyncInfo(site.ID)
	if err != nil || info.Configured {
		t.Fatalf("disable: %#v %v", info, err)
	}
}

func TestXManualSyncRequiresPublishedRevisionAndDoesNotRepeat(t *testing.T) {
	l, site, root, p := xTestLibrary(t)
	a, err := l.SaveArticle(site.ID, "", "Title", "body")
	if err != nil {
		t.Fatal(err)
	}
	configureTestX(t, l, site.ID, false)
	if _, err = l.SyncXArticle(site.ID, a.ID, false); err == nil || p.calls != 0 {
		t.Fatal("unpublished article sent")
	}
	publishXSnapshot(t, l, site.ID)
	p.before = func() {
		r, e := services.LoadXPost(root, site.ID, a.ID)
		if e != nil || r.Status != "pending" {
			t.Fatalf("missing durable intent: %#v %v", r, e)
		}
	}
	post, err := l.SyncXArticle(site.ID, a.ID, false)
	if err != nil || post.Status != "sent" || post.URL != "https://x.com/i/web/status/123456789" || p.calls != 1 {
		t.Fatalf("post: %#v %v calls=%d", post, err, p.calls)
	}
	if !strings.Contains(p.text, "https://ipfs.io/ipns/test-ipns/"+a.ID+"/") {
		t.Fatal(p.text)
	}
	if _, err = l.SyncXArticle(site.ID, a.ID, true); err != nil || p.calls != 1 {
		t.Fatalf("duplicate: %v %d", err, p.calls)
	}
	if err = l.DisableXSync(site.ID); err != nil {
		t.Fatal(err)
	}
	configureTestX(t, l, site.ID, true)
	if _, err = l.SyncXArticle(site.ID, a.ID, false); err != nil || p.calls != 1 {
		t.Fatal("disable/re-enable lost history")
	}
}

func TestXAutomaticSyncOnlyNewArticlesAndSkipsPages(t *testing.T) {
	l, site, root, p := xTestLibrary(t)
	old, err := l.SaveArticle(site.ID, "", "Old", "body")
	if err != nil {
		t.Fatal(err)
	}
	configureTestX(t, l, site.ID, true)
	snapshot, articles := publishXSnapshot(t, l, site.ID)
	if warning := l.syncPublishedX(root, snapshot, articles); warning != "" || p.calls != 0 {
		t.Fatalf("backfilled history: %s %d", warning, p.calls)
	}
	fresh, err := l.SaveArticle(site.ID, "", "Fresh", "body")
	if err != nil {
		t.Fatal(err)
	}
	page, err := l.SaveArticle(site.ID, "", "Page", "body")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.UpdateArticleSettings(site.ID, page.ID, "page", 1, "", false, 0); err != nil {
		t.Fatal(err)
	}
	snapshot, articles = publishXSnapshot(t, l, site.ID)
	// Explicit timestamps avoid depending on clock resolution in the test fixture.
	cfg, err := services.LoadConfig(root, site.ID, "x")
	if err != nil {
		t.Fatal(err)
	}
	for i := range articles {
		if articles[i].ID == fresh.ID {
			articles[i].Created = cfg.XEnabledAt.Add(time.Second)
		}
		if articles[i].ID == old.ID {
			articles[i].Created = cfg.XEnabledAt.Add(-time.Second)
		}
	}
	if warning := l.syncPublishedX(root, snapshot, articles); warning != "" || p.calls != 1 {
		t.Fatalf("auto: %s %d", warning, p.calls)
	}
	if warning := l.syncPublishedX(root, snapshot, articles); warning != "" || p.calls != 1 {
		t.Fatalf("repeated: %s %d", warning, p.calls)
	}
}

func TestXUncertainDeliverySurvivesRestartAndNeedsExplicitRetry(t *testing.T) {
	l, site, root, p := xTestLibrary(t)
	configureTestX(t, l, site.ID, true)
	a, err := l.SaveArticle(site.ID, "", "Title", "body")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, articles := publishXSnapshot(t, l, site.ID)
	p.err = errors.New("network unavailable")
	if warning := l.syncPublishedX(root, snapshot, articles); warning == "" || p.calls != 1 {
		t.Fatalf("missing warning: %s", warning)
	}
	r, err := services.LoadXPost(root, site.ID, a.ID)
	if err != nil || r.Status != "pending" {
		t.Fatalf("record: %#v %v", r, err)
	}
	restarted := &Library{xPoster: p}
	if _, err = restarted.SyncXArticle(site.ID, a.ID, false); err == nil || p.calls != 1 {
		t.Fatal("uncertain request resent")
	}
	p.err = nil
	if _, err = restarted.SyncXArticle(site.ID, a.ID, true); err != nil || p.calls != 2 {
		t.Fatalf("explicit retry: %v %d", err, p.calls)
	}
}

func TestXManualSyncBlocksUnsavedPublicationAndUsesSlug(t *testing.T) {
	l, site, _, p := xTestLibrary(t)
	a, err := l.SaveArticle(site.ID, "", "Title", "body")
	if err != nil {
		t.Fatal(err)
	}
	configureTestX(t, l, site.ID, false)
	publishXSnapshot(t, l, site.ID)
	if _, err = l.UpdateArticleSettings(site.ID, a.ID, "my-post", 0, "", false, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = l.SyncXArticle(site.ID, a.ID, false); err == nil || p.calls != 0 {
		t.Fatal("unpublished edits sent")
	}
	publishXSnapshot(t, l, site.ID)
	if _, err = l.SyncXArticle(site.ID, a.ID, false); err != nil || !strings.HasSuffix(p.text, "/my-post/") {
		t.Fatalf("slug: %s %v", p.text, err)
	}
}

func TestXRecordWriteFailurePreventsSending(t *testing.T) {
	l, site, root, p := xTestLibrary(t)
	a, err := l.SaveArticle(site.ID, "", "Title", "body")
	if err != nil {
		t.Fatal(err)
	}
	configureTestX(t, l, site.ID, false)
	publishXSnapshot(t, l, site.ID)
	// A non-directory at the state directory must fail closed before the POST.
	if err = os.WriteFile(filepath.Join(root, "integrations", "x-post"), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = l.SyncXArticle(site.ID, a.ID, false); err == nil || p.calls != 0 {
		t.Fatalf("sent without durable record: %v %d", err, p.calls)
	}
}

func TestXSentRecordWriteFailureKeepsPendingAndDoesNotResend(t *testing.T) {
	l, site, root, p := xTestLibrary(t)
	a, err := l.SaveArticle(site.ID, "", "Title", "body")
	if err != nil {
		t.Fatal(err)
	}
	configureTestX(t, l, site.ID, false)
	publishXSnapshot(t, l, site.ID)
	// An invalid success ID emulates a success that cannot be committed locally;
	// the HTTP client independently rejects malformed IDs before returning them.
	l.xPoster = invalidXPoster{}
	if _, err = l.SyncXArticle(site.ID, a.ID, false); err == nil {
		t.Fatal("invalid success recorded")
	}
	record, err := services.LoadXPost(root, site.ID, a.ID)
	if err != nil || record.Status != "pending" {
		t.Fatalf("lost pending state: %#v %v", record, err)
	}
	l.xPoster = p
	if _, err = l.SyncXArticle(site.ID, a.ID, false); err == nil || p.calls != 0 {
		t.Fatal("resent uncertain post")
	}
}

type invalidXPoster struct{}

func (invalidXPoster) CreateXPost(services.XCredentials, string) (string, error) {
	return "invalid", nil
}

func TestXPermanentSiteDeletionRemovesCredentialsAndRecords(t *testing.T) {
	l, site, root, _ := xTestLibrary(t)
	a, err := l.SaveArticle(site.ID, "", "Title", "body")
	if err != nil {
		t.Fatal(err)
	}
	configureTestX(t, l, site.ID, false)
	if err = services.SaveXPost(root, site.ID, a.ID, services.XPostRecord{Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	if err = l.RemoveSite(site.ID); err != nil {
		t.Fatal(err)
	}
	if err = l.PermanentlyDeleteSite(site.ID); err != nil {
		t.Fatal(err)
	}
	cfg, err := services.LoadConfig(root, site.ID, "x")
	if err != nil || cfg.XCredentials != nil {
		t.Fatalf("credentials survived: %v", err)
	}
	record, err := services.LoadXPost(root, site.ID, a.ID)
	if err != nil || record.Status != "" {
		t.Fatalf("records survived: %#v %v", record, err)
	}
}
