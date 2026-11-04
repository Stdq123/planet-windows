package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"windows/internal/feeds"
)

func TestPlausibleTrackingPersistsAndMarksContentChanged(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.SetPlausibleTracking(site.ID, true, "example.com")
	if err != nil || updated.ContentRevision <= site.ContentRevision {
		t.Fatalf("save: %+v %v", updated, err)
	}
	if _, err = s.SetPlausibleTracking(site.ID, true, `evil.com\"><script>`); err == nil {
		t.Fatal("accepted unsafe domain")
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, _, _, err := reopened.PublicationSnapshot(site.ID)
	if err != nil || got.PlausibleOverride == nil || got.PlausibleOverride.Domain != "example.com" {
		t.Fatalf("persist: %+v %v", got, err)
	}
}

func TestPlausibleScriptIDMustBeSafeAndPersists(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetPlausibleTrackingScript(site.ID, true, "example.com", "pa-ABC_123"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"https://evil/x.js", `pa-x\"><script>`, "pa-../evil"} {
		if _, err = s.SetPlausibleTrackingScript(site.ID, true, "example.com", bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, _, _, err := reopened.PublicationSnapshot(site.ID)
	if err != nil || got.PlausibleOverride == nil || got.PlausibleOverride.ScriptID != "pa-ABC_123" {
		t.Fatalf("lost ID: %+v %v", got, err)
	}
}

func TestPlausibleCustomServerPersistsWithoutAcceptingUnsafeURLs(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetPlausibleTrackingServer(site.ID, true, "example.com", "pa-Test123", "https://stats.example.com:8443"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetPlausibleTrackingServer(site.ID, true, "example.com", "pa-Test123", "https://user:password@evil.example/path"); err == nil {
		t.Fatal("accepted unsafe server")
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, _, _, err := reopened.PublicationSnapshot(site.ID)
	if err != nil || got.PlausibleOverride == nil || got.PlausibleOverride.Server != "https://stats.example.com:8443" {
		t.Fatalf("lost server: %+v %v", got, err)
	}
	rendered := ApplyPlausibleOverride(got)
	if rendered.PlausibleAPIServer != "stats.example.com:8443" {
		t.Fatalf("wrong server: %q", rendered.PlausibleAPIServer)
	}
}

func TestRemovedSiteCannotPublishAndCanBeRestoredWithItsData(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	article, err := s.SaveArticle(site.ID, "", "Saved", "Article")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveDraft(site.ID, "", article.ID, "Draft", "Private"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetIdentity(site.ID, "k51identity"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveSite(site.ID); err != nil {
		t.Fatal(err)
	}
	if sites, err := s.ListSites(); err != nil || len(sites) != 0 {
		t.Fatalf("removed site visible: %+v %v", sites, err)
	}
	if _, _, _, err = s.PublicationSnapshot(site.ID); err == nil {
		t.Fatal("removed site can still be published")
	}
	if _, err = s.SaveArticle(site.ID, article.ID, "Changed", "body"); err == nil {
		t.Fatal("removed site can still be edited")
	}
	if _, err = s.AddAttachment(site.ID, article.ID, "file.txt"); err == nil {
		t.Fatal("removed site can still be modified")
	}
	if _, err = s.UpdateSite(site.ID, "Changed", ""); err == nil {
		t.Fatal("removed site settings can still be modified")
	}
	if err = s.SetTemplate(site.ID, "template-test", "Plain"); err == nil {
		t.Fatal("removed site template can still be modified")
	}
	if err = s.SetPublished(site.ID, "k51identity", "bafycid"); err == nil {
		t.Fatal("removed site publication can still be renewed")
	}
	if _, err = s.SetPublishedRevision(site.ID, "k51identity", "bafycid", 0); err == nil {
		t.Fatal("removed site publication can still be recorded")
	}
	if err = s.SetIdentity(site.ID, "k51identity"); err == nil {
		t.Fatal("removed site identity can still be modified")
	}
	if err = s.Import(Site{ID: site.ID, Name: "Duplicate", IPNS: "other"}, nil, json.RawMessage(`{}`)); err == nil {
		t.Fatal("removed identity was reused")
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := s.RemovedSites()
	if err != nil || len(removed) != 1 {
		t.Fatal("removed site not retained")
	}
	if _, err := s.RestoreSite(site.ID); err != nil {
		t.Fatal(err)
	}
	current, articles, _, err := s.PublicationSnapshot(site.ID)
	if err != nil || len(articles) != 1 || articles[0].Content != "Article" || current.IPNS != "k51identity" || current.PublishPending || current.ContentRevision <= current.PublishedRevision {
		t.Fatalf("restored data: %+v %+v %v", current, articles, err)
	}
	drafts, err := s.ListDrafts(site.ID)
	if err != nil || len(drafts) != 1 || drafts[0].Content != "Private" {
		t.Fatalf("draft lost: %+v %v", drafts, err)
	}
}

func TestPermanentDeletionIsDurableAndCannotRestoreOnceStarted(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	article, err := s.SaveArticle(site.ID, "", "Post", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveDraft(site.ID, "", article.ID, "Draft", "secret"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.BeginSiteDeletion(site.ID); err == nil {
		t.Fatal("active site deletion accepted")
	}
	if err = s.RemoveSite(site.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.BeginSiteDeletion(site.ID); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := s.RemovedSites()
	if err != nil || len(removed) != 1 || !removed[0].DeletionPending {
		t.Fatalf("pending deletion lost: %+v %v", removed, err)
	}
	if _, err = s.RestoreSite(site.ID); err == nil {
		t.Fatal("restored partially deleted site")
	}
	if _, err = s.BeginSiteDeletion(site.ID); err != nil {
		t.Fatal("retry not allowed", err)
	}
	if err = s.FinalizeSiteDeletion(site.ID); err != nil {
		t.Fatal(err)
	}
	removed, err = s.RemovedSites()
	if err != nil || len(removed) != 0 {
		t.Fatalf("site retained: %+v %v", removed, err)
	}
	if len(s.data.Articles) != 0 || len(s.data.Drafts) != 0 {
		t.Fatal("site metadata retained")
	}
}

func TestFailedDeletionCommitPreservesOtherSitesAndRetryState(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	one, err := s.CreateSite("Delete", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateSite("Keep", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveArticle(other.ID, "", "Keep post", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.RemoveSite(one.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.BeginSiteDeletion(one.ID); err != nil {
		t.Fatal(err)
	}
	s.path = filepath.Join(dir, "blocked")
	if err = os.Mkdir(s.path, 0700); err != nil {
		t.Fatal(err)
	}
	if err = s.FinalizeSiteDeletion(one.ID); err == nil {
		t.Fatal("expected failed write")
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := s.RemovedSites()
	if err != nil || len(removed) != 1 || !removed[0].DeletionPending {
		t.Fatalf("pending state lost: %+v %v", removed, err)
	}
	articles, err := s.ListArticles(other.ID)
	if err != nil || len(articles) != 1 {
		t.Fatalf("other site damaged: %+v %v", articles, err)
	}
}

func TestFailedSiteRemovalDoesNotLoseVisibleSite(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	s.path = filepath.Join(dir, "blocked")
	if err := os.Mkdir(s.path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveSite(site.ID); err == nil {
		t.Fatal("expected failed write")
	}
	sites, err := s.ListSites()
	if err != nil || len(sites) != 1 || sites[0].ID != site.ID {
		t.Fatalf("site changed after failed write: %+v %v", sites, err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	sites, err = reopened.ListSites()
	if err != nil || len(sites) != 1 {
		t.Fatalf("site lost on disk: %+v %v", sites, err)
	}
}

func TestDraftSurvivesRestartAndStaysOutOfPublicationUntilPublished(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := s.SaveDraft(site.ID, "", "", "Draft", "Private text")
	if err != nil || draft.ID == "" {
		t.Fatalf("draft: %+v %v", draft, err)
	}
	snapshot, articles, _, err := s.PublicationSnapshot(site.ID)
	if err != nil || len(articles) != 0 || snapshot.PublishPending {
		t.Fatalf("draft became public: %+v %+v %v", snapshot, articles, err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := s.ListDrafts(site.ID)
	if err != nil || len(listed) != 1 {
		t.Fatal("draft was not persisted")
	}
	if _, err := s.PublishDraft(site.ID, draft.ID, "Published", "Public text"); err != nil {
		t.Fatal(err)
	}
	snapshot, articles, _, err = s.PublicationSnapshot(site.ID)
	if err != nil || len(articles) != 1 || articles[0].Title != "Published" || !snapshot.PublishPending {
		t.Fatalf("draft publication: %+v %+v %v", snapshot, articles, err)
	}
	listed, err = s.ListDrafts(site.ID)
	if err != nil || len(listed) != 0 {
		t.Fatal("published draft was retained")
	}
}

func TestDraftCannotOverwriteArticleChangedSinceDraftWasCreated(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	article, err := s.SaveArticle(site.ID, "", "Original", "First")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := s.SaveDraft(site.ID, "", article.ID, "Draft", "Draft text")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveArticle(site.ID, article.ID, "Newer", "Changed elsewhere"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PublishDraft(site.ID, draft.ID, "Draft", "Draft text"); err == nil {
		t.Fatal("stale draft overwrote a newer article")
	}
	articles, err := s.ListArticles(site.ID)
	if err != nil || len(articles) != 1 || articles[0].Title != "Newer" {
		t.Fatalf("newer article lost: %+v %v", articles, err)
	}
	drafts, err := s.ListDrafts(site.ID)
	if err != nil || len(drafts) != 1 {
		t.Fatalf("draft lost: %+v %v", drafts, err)
	}
}

func TestFailedDraftPublicationKeepsSavedDraft(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := s.SaveDraft(site.ID, "", "", "Draft", "Private")
	if err != nil {
		t.Fatal(err)
	}
	s.path = filepath.Join(dir, "blocked")
	if err := os.Mkdir(s.path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PublishDraft(site.ID, draft.ID, "Title", "Content"); err == nil {
		t.Fatal("expected failed write")
	}
	_, articles, _, err := s.PublicationSnapshot(site.ID)
	if err != nil || len(articles) != 0 {
		t.Fatal("failed publication changed memory")
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	drafts, err := reopened.ListDrafts(site.ID)
	if err != nil || len(drafts) != 1 || drafts[0].Content != "Private" {
		t.Fatalf("draft lost after failure: %+v %v", drafts, err)
	}
}

func TestFailedDraftWriteRollsBackPreparedFiles(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := s.SaveDraft(site.ID, "", "", "Draft", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeDraftAttachment(site.ID, draft.ID, "photo.png", true); err != nil {
		t.Fatal(err)
	}
	s.path = filepath.Join(dir, "blocked")
	if err := os.Mkdir(s.path, 0700); err != nil {
		t.Fatal(err)
	}
	rolledBack := false
	_, err = s.PublishDraftPrepared(site.ID, draft.ID, "Draft", "", func(article Article, names []string) (Article, func(), error) {
		if len(names) != 1 || names[0] != "photo.png" {
			t.Fatalf("names=%v", names)
		}
		return article, func() { rolledBack = true }, nil
	})
	if err == nil || !rolledBack {
		t.Fatalf("write error=%v rollback=%v", err, rolledBack)
	}
	drafts, err := s.ListDrafts(site.ID)
	if err != nil || len(drafts) != 1 || len(drafts[0].Attachments) != 1 {
		t.Fatalf("draft lost: %+v %v", drafts, err)
	}
}

func TestDraftStagesExistingAttachmentRemovalUntilPublish(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	article, err := s.SaveArticle(site.ID, "", "Original", "")
	if err != nil {
		t.Fatal(err)
	}
	article, err = s.AddAttachment(site.ID, article.ID, "song.mp3")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := s.SaveDraft(site.ID, "", article.ID, "Draft", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err = s.SetDraftExistingAttachmentRemoved(site.ID, draft.ID, "song.mp3", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.RemovedAttachments) != 1 {
		t.Fatalf("draft=%+v", draft)
	}
	_, articles, _, err := s.PublicationSnapshot(site.ID)
	if err != nil || len(articles[0].Attachments) != 1 {
		t.Fatalf("existing article changed: %+v %v", articles, err)
	}
	if _, err = s.SetDraftExistingAttachmentRemoved(site.ID, draft.ID, "song.mp3", false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetDraftExistingAttachmentRemoved(site.ID, draft.ID, "song.mp3", true); err != nil {
		t.Fatal(err)
	}
	article, err = s.PublishDraft(site.ID, draft.ID, "Draft", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(article.Attachments) != 0 || article.AudioFilename != "" {
		t.Fatalf("published attachment retained: %+v", article)
	}
	if len(s.PendingArticleCleanup()) != 1 {
		t.Fatal("attachment cleanup not tracked")
	}
}

func TestTemplateSettingsPersistAndMarkSiteChanged(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetTemplate(site.ID, "template-test", "Sepia"); err != nil {
		t.Fatal(err)
	}
	before, _, _, err := s.PublicationSnapshot(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	settings := map[string]string{"backgroundColor": "#123456", "showRef": "true"}
	updated, err := s.SetTemplateSettings(site.ID, settings)
	if err != nil {
		t.Fatal(err)
	}
	settings["backgroundColor"] = "#FFFFFF"
	if updated.TemplateSettings["backgroundColor"] != "#123456" || updated.ContentRevision <= before.ContentRevision {
		t.Fatalf("settings=%+v", updated)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	reopened, _, _, err := s.PublicationSnapshot(site.ID)
	if err != nil || reopened.TemplateSettings["showRef"] != "true" {
		t.Fatalf("settings lost: %+v %v", reopened, err)
	}
	if err := s.SetTemplate(site.ID, "template-other", "Plain"); err != nil {
		t.Fatal(err)
	}
	changed, _, _, err := s.PublicationSnapshot(site.ID)
	if err != nil || changed.TemplateSettings != nil {
		t.Fatalf("settings carried to other template: %+v %v", changed, err)
	}
}

func TestUpdateSitePersistsSettingsAndMarksPublishedVersionOutdated(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Before", "Old")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPublished(site.ID, "k51identity", "bafyprevious"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateSite(site.ID, "After", "New"); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	sites, err := s.ListSites()
	if err != nil || len(sites) != 1 {
		t.Fatalf("sites: %+v %v", sites, err)
	}
	got := sites[0]
	if got.Name != "After" || got.About != "New" || got.IPNS != "k51identity" || got.ContentRevision <= got.PublishedRevision || got.PublishPending {
		t.Fatalf("updated site: %+v", got)
	}
}

func TestImportAssignsStableNumbersToMissingAndDuplicateArticles(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	const siteID = "11111111-1111-4111-8111-111111111111"
	seven := 7
	site := Site{ID: siteID, Name: "Site", IPNS: "name", Imported: true}
	articles := []Article{
		{ID: "44444444-4444-4444-8444-444444444444", SiteID: siteID, Created: time.Unix(3, 0)},
		{ID: "33333333-3333-4333-8333-333333333333", SiteID: siteID, Created: time.Unix(2, 0), ArticleNumber: &seven},
		{ID: "22222222-2222-4222-8222-222222222222", SiteID: siteID, Created: time.Unix(1, 0), ArticleNumber: &seven},
	}
	if err := s.Import(site, articles, json.RawMessage(`{"slug":"journal","nextArticleNumber":10,"articles":[]}`)); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.ListArticles(siteID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"22222222-2222-4222-8222-222222222222": 7, "33333333-3333-4333-8333-333333333333": 10, "44444444-4444-4444-8444-444444444444": 11}
	for _, a := range got {
		if a.ArticleNumber == nil || *a.ArticleNumber != want[a.ID] || a.ArticleReference != fmt.Sprintf("JOURNAL-%d", want[a.ID]) {
			t.Fatalf("number mismatch: %+v", a)
		}
	}
	added, err := s.SaveArticle(siteID, "", "new", "body")
	if err != nil {
		t.Fatal(err)
	}
	if added.ArticleNumber == nil || *added.ArticleNumber != 12 {
		t.Fatalf("next number: %+v", added)
	}
}

func TestExistingImportedLibraryUpgradesMissingNumbersOnce(t *testing.T) {
	dir := t.TempDir()
	const siteID = "11111111-1111-4111-8111-111111111111"
	old := `{"version":3,"sites":[{"id":"` + siteID + `","name":"Site","ipns":"name","imported":true}],"articles":[{"id":"22222222-2222-4222-8222-222222222222","siteId":"` + siteID + `","title":"Post","created":"2020-01-01T00:00:00Z","modified":"2020-01-01T00:00:00Z"}],"backupMetadata":{"` + siteID + `":{"slug":"journal","nextArticleNumber":5,"articles":[]}}}`
	path := filepath.Join(dir, "library.json")
	if err := os.WriteFile(path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	posts, err := s.ListArticles(siteID)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 || posts[0].ArticleNumber == nil || *posts[0].ArticleNumber != 5 || posts[0].ArticleReference != "JOURNAL-5" {
		t.Fatalf("upgrade: %+v", posts)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("reopening renumbered an unchanged library")
	}
}

func TestNumberUpgradeMarksPreviouslyPublishedSnapshotOutdated(t *testing.T) {
	dir := t.TempDir()
	const siteID = "11111111-1111-4111-8111-111111111111"
	old := `{"version":3,"sites":[{"id":"` + siteID + `","name":"Site","ipns":"name","imported":true,"lastPublishedCID":"old-cid","contentRevision":2,"publishedRevision":2}],"articles":[{"id":"22222222-2222-4222-8222-222222222222","siteId":"` + siteID + `","created":"2020-01-01T00:00:00Z","modified":"2020-01-01T00:00:00Z"}],"backupMetadata":{"` + siteID + `":{"articles":[]}}}`
	if err := os.WriteFile(filepath.Join(dir, "library.json"), []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	sites, err := s.ListSites()
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0].ContentRevision != 3 || sites[0].PublishedRevision != 2 || sites[0].PublishPending {
		t.Fatalf("incorrect publication state: %+v", sites)
	}
}

func TestImportedArticleNumbersContinueAfterDeletionAndReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site := Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Site", IPNS: "name", Imported: true}
	seven := 7
	old := Article{ID: "22222222-2222-4222-8222-222222222222", SiteID: site.ID, ArticleNumber: &seven}
	raw := json.RawMessage(`{"nextArticleNumber":10,"slug":"journal","articles":[]}`)
	if err := s.Import(site, []Article{old}, raw); err != nil {
		t.Fatal(err)
	}
	first, err := s.SaveArticle(site.ID, "", "first", "body")
	if err != nil {
		t.Fatal(err)
	}
	if first.ArticleNumber == nil || *first.ArticleNumber != 10 || first.ArticleReference != "JOURNAL-10" {
		t.Fatalf("first article: %+v", first)
	}
	if err := s.DeleteArticle(site.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.SaveArticle(site.ID, "", "second", "body")
	if err != nil {
		t.Fatal(err)
	}
	if second.ArticleNumber == nil || *second.ArticleNumber != 11 || second.ArticleReference != "JOURNAL-11" {
		t.Fatalf("second article: %+v", second)
	}
}

func TestImportedDeletedArticleNumberIsNotReusedWhenCounterMissing(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	site := Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Site", IPNS: "name", Imported: true}
	seven := 7
	old := Article{ID: "22222222-2222-4222-8222-222222222222", SiteID: site.ID, ArticleNumber: &seven}
	if err := s.Import(site, []Article{old}, json.RawMessage(`{"articles":[{"articleNumber":7}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteArticle(site.ID, old.ID); err != nil {
		t.Fatal(err)
	}
	added, err := s.SaveArticle(site.ID, "", "new", "body")
	if err != nil {
		t.Fatal(err)
	}
	if added.ArticleNumber == nil || *added.ArticleNumber != 8 {
		t.Fatalf("number reused: %+v", added)
	}
}

func TestSitesAndArticlesSurviveReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("我的博客", "个人网站")
	if err != nil {
		t.Fatal(err)
	}
	post, err := s.SaveArticle(site.ID, "", "第一篇", "Hello **Planet**")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveArticle(site.ID, post.ID, "第一篇更新", "正文")
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	sites, err := reopened.ListSites()
	if err != nil || len(sites) != 1 || sites[0].Name != "我的博客" {
		t.Fatalf("sites = %#v, err = %v", sites, err)
	}
	posts, err := reopened.ListArticles(site.ID)
	if err != nil || len(posts) != 1 || posts[0].Title != "第一篇更新" || posts[0].Content != "正文" {
		t.Fatalf("posts = %#v, err = %v", posts, err)
	}
}

func TestUnknownSiteCannotBeWritten(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveArticle("../outside", "", "title", "content"); err == nil {
		t.Fatal("expected unknown site error")
	}
}

func TestEmptySiteNameIsRejected(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSite("  ", ""); err == nil {
		t.Fatal("expected name validation error")
	}
}

func TestFailedWriteDoesNotChangeInMemoryLibrary(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSite("existing", ""); err != nil {
		t.Fatal(err)
	}
	// A directory cannot be replaced by the temporary library file.
	s.path = filepath.Join(dir, "blocked")
	if err := os.Mkdir(s.path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSite("not saved", ""); err == nil {
		t.Fatal("expected failed write")
	}
	sites, err := s.ListSites()
	if err != nil || len(sites) != 1 || sites[0].Name != "existing" {
		t.Fatalf("sites = %#v, err = %v", sites, err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	sites, err = reopened.ListSites()
	if err != nil || len(sites) != 1 || sites[0].Name != "existing" {
		t.Fatalf("persisted sites = %#v, err = %v", sites, err)
	}
}

func TestInvalidLibraryDoesNotGetOverwritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.json")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("expected invalid library error")
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "broken" {
		t.Fatalf("library = %q, err = %v", b, err)
	}
}

func TestSubscriptionsSurviveReopenAndUpdate(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveSubscription("https://example.com/feed", feeds.Feed{Title: "Example", Items: []feeds.Item{{ID: "1", Title: "Old"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveSubscription("https://example.com/feed", feeds.Feed{Title: "Example", Items: []feeds.Item{{ID: "1", Title: "Updated"}}})
	if err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.ListSubscriptions()
	if err != nil || len(got) != 1 || len(got[0].Feed.Items) != 1 || got[0].Feed.Items[0].Title != "Updated" {
		t.Fatalf("subscriptions = %#v, err = %v", got, err)
	}
}

func TestRefreshPreservesOlderFeedItems(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveSubscription("https://example.com/feed", feeds.Feed{Items: []feeds.Item{{ID: "a", Title: "A"}, {ID: "b", Title: "B"}}})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.SaveSubscription("https://example.com/feed", feeds.Feed{Items: []feeds.Item{{ID: "b", Title: "B updated"}, {ID: "c", Title: "C"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Feed.Items) != 3 || updated.Feed.Items[0].Title != "B updated" || updated.Feed.Items[1].ID != "c" || updated.Feed.Items[2].ID != "a" {
		t.Fatalf("items = %#v", updated.Feed.Items)
	}
}

func TestDeleteArticleIsScopedToSite(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateSite("A", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateSite("B", "")
	if err != nil {
		t.Fatal(err)
	}
	post, err := s.SaveArticle(a.ID, "", "Article", "text")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteArticle(b.ID, post.ID); err == nil {
		t.Fatal("expected cross-site deletion to fail")
	}
	if err := s.DeleteArticle(a.ID, post.ID); err != nil {
		t.Fatal(err)
	}
	posts, err := s.ListArticles(a.ID)
	if err != nil || len(posts) != 0 {
		t.Fatalf("posts = %#v, err = %v", posts, err)
	}
}

func TestPublishedIdentitySurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Test", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetPublished(site.ID, "k51abc", "bafyabc"); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	sites, err := s.ListSites()
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0].IPNS != "k51abc" || sites[0].LastPublishedCID != "bafyabc" {
		t.Fatalf("sites: %#v", sites)
	}
}

func TestIdentityCannotBeReplaced(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Test", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetIdentity(site.ID, "original"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetIdentity(site.ID, "different"); err == nil {
		t.Fatal("expected identity mismatch")
	}
	sites, err := s.ListSites()
	if err != nil || sites[0].IPNS != "original" {
		t.Fatalf("sites=%#v err=%v", sites, err)
	}
}

func TestImportIsAtomicAndRetainsRawMetadata(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site := Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Imported", IPNS: "k51"}
	article := Article{ID: "22222222-2222-4222-8222-222222222222", SiteID: site.ID, Title: "Post"}
	raw := []byte(`{"unknown":123}`)
	if err := s.Import(site, []Article{article}, raw); err != nil {
		t.Fatal(err)
	}
	if err := s.Import(site, []Article{article}, raw); err == nil {
		t.Fatal("duplicate import accepted")
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.BackupMetadata(site.ID)
	var metadata map[string]int
	if err != nil || json.Unmarshal(got, &metadata) != nil || metadata["unknown"] != 123 {
		t.Fatalf("metadata=%s err=%v", got, err)
	}
	posts, err := s.ListArticles(site.ID)
	if err != nil || len(posts) != 1 {
		t.Fatalf("posts=%#v err=%v", posts, err)
	}
}

func TestTemplateSelectionSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Test", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetTemplate(site.ID, "TEMPLATE-1", "Plain"); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	sites, err := s.ListSites()
	if err != nil || len(sites) != 1 || sites[0].TemplateID != "TEMPLATE-1" || sites[0].TemplateName != "Plain" {
		t.Fatalf("sites=%#v err=%v", sites, err)
	}
}

func TestSavedPublicationIntentSurvivesRestartAndNewerEdits(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	post, err := s.SaveArticle(site.ID, "", "First", "one")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, _, err := s.PublicationSnapshot(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.PublishPending || snapshot.ContentRevision == 0 {
		t.Fatalf("no durable publication intent: %#v", snapshot)
	}
	if _, err = s.SaveArticle(site.ID, post.ID, "Second", "two"); err != nil {
		t.Fatal(err)
	}
	pending, err := s.SetPublishedRevision(site.ID, "k51abc", "old-cid", snapshot.ContentRevision)
	if err != nil || !pending {
		t.Fatalf("new edit lost: pending=%v err=%v", pending, err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, articles, _, err := s.PublicationSnapshot(site.ID)
	if err != nil || !snapshot.PublishPending || articles[0].Content != "two" {
		t.Fatalf("restart=%#v %#v %v", snapshot, articles, err)
	}
	pending, err = s.SetPublishedRevision(site.ID, "k51abc", "new-cid", snapshot.ContentRevision)
	if err != nil || pending {
		t.Fatalf("latest not cleared: %v %v", pending, err)
	}
	if err = s.DeleteArticle(site.ID, post.ID); err != nil {
		t.Fatal(err)
	}
	snapshot, _, _, err = s.PublicationSnapshot(site.ID)
	if err != nil || snapshot.ContentRevision <= snapshot.PublishedRevision {
		t.Fatalf("deletion appears published: %#v %v", snapshot, err)
	}
	pending, err = s.SetPublishedRevision(site.ID, "k51abc", "older-cid", snapshot.PublishedRevision)
	if err != nil || !pending {
		t.Fatalf("change during publication was not queued: %v %v", pending, err)
	}
}

func TestLegacyImportedAttachmentsSurviveUpgradeAndRemoval(t *testing.T) {
	root := t.TempDir()
	data := `{"version":1,"sites":[{"id":"site","name":"Site","ipns":"test","imported":true}],"articles":[{"id":"post","siteId":"site","title":"Post","content":"Text","created":"2020-01-01T00:00:00Z","modified":"2020-01-01T00:00:00Z"}],"backupMetadata":{"site":{"articles":[{"id":"post","attachments":["old.png"],"articleNumber":7}]}}}`
	if err := os.WriteFile(filepath.Join(root, "library.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	lib, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	posts, err := lib.ListArticles("site")
	if err != nil || len(posts) != 1 || len(posts[0].Attachments) != 1 || posts[0].ArticleNumber == nil {
		t.Fatalf("posts=%+v err=%v", posts, err)
	}
	if _, err = lib.RemoveAttachment("site", "post", "old.png"); err != nil {
		t.Fatal(err)
	}
	lib, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	posts, err = lib.ListArticles("site")
	if err != nil || len(posts[0].Attachments) != 0 {
		t.Fatalf("removed file returned: %+v %v", posts, err)
	}
}

func TestVersionTwoImportedMediaMetadataIsUpgraded(t *testing.T) {
	root := t.TempDir()
	data := `{"version":2,"sites":[{"id":"site","name":"Site","ipns":"test","imported":true}],"articles":[{"id":"post","siteId":"site","title":"Post","content":"","created":"2020-01-01T00:00:00Z","modified":"2020-01-01T00:00:00Z","attachments":["sound.m4a","video.mp4"]}],"backupMetadata":{"site":{"articles":[{"id":"post","audioFilename":"sound.m4a","videoFilename":"video.mp4"}]}}}`
	os.WriteFile(filepath.Join(root, "library.json"), []byte(data), 0600)
	lib, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	posts, err := lib.ListArticles("site")
	if err != nil || posts[0].AudioFilename != "sound.m4a" || posts[0].VideoFilename != "video.mp4" {
		t.Fatalf("upgraded=%+v err=%v", posts, err)
	}
}

func TestSubscriptionStarSurvivesRefreshAndRestart(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	url := "https://example.test/feed"
	_, err = s.SaveSubscription(url, feeds.Feed{Title: "Feed", Items: []feeds.Item{{ID: "item-1", Title: "Old", Link: "https://example.test/one"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetSubscriptionStarred(url, "item-1", true); err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveSubscription(url, feeds.Feed{Title: "Feed", Items: []feeds.Item{{ID: "item-1", Title: "New", Link: "https://example.test/one"}}})
	if err != nil {
		t.Fatal(err)
	}
	s, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	subs, err := s.ListSubscriptions()
	if err != nil || !subs[0].Feed.Items[0].Starred || subs[0].Feed.Items[0].Title != "New" {
		t.Fatalf("subscriptions=%+v err=%v", subs, err)
	}
	if err = s.SetSubscriptionStarred(url, "item-1", false); err != nil {
		t.Fatal(err)
	}
	subs, _ = s.ListSubscriptions()
	if subs[0].Feed.Items[0].Starred {
		t.Fatal("star not removed")
	}
}

func TestPlanetSubscriptionTracksPinAndRefreshFailuresWithoutLosingCache(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	url := "planet://k51abcdefghijk"
	feed := feeds.Feed{Title: "Site", Items: []feeds.Item{{ID: "a", Title: "Post"}}}
	if _, err = s.SavePlanetSubscription(url, feed, "bafyone", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.SetSubscriptionPinned(url, "bafyone"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetSubscriptionError(url, "offline"); err != nil {
		t.Fatal(err)
	}
	subs, _ := s.ListSubscriptions()
	if subs[0].CID != "bafyone" || subs[0].PinnedCID != "bafyone" || subs[0].LastError != "offline" || len(subs[0].Feed.Items) != 1 {
		t.Fatalf("subscription=%+v", subs[0])
	}
	if _, err = s.SavePlanetSubscription(url, feed, "bafytwo", "bafytwo"); err != nil {
		t.Fatal(err)
	}
	subs, _ = s.ListSubscriptions()
	if subs[0].CID != "bafytwo" || subs[0].PinnedCID != "bafytwo" || subs[0].LastError != "" {
		t.Fatalf("subscription=%+v", subs[0])
	}
	if err = s.SetSubscriptionPinned(url, "bafyone"); err == nil {
		t.Fatal("stale pin accepted")
	}
}

func TestOfflineFeedSnapshotSurvivesRefreshAndKeepsStars(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	address := "https://example.test/feed"
	feed := feeds.Feed{Title: "Feed", Items: []feeds.Item{{ID: "one", Title: "Old"}}}
	if _, err = s.SaveSubscription(address, feed); err != nil {
		t.Fatal(err)
	}
	if err = s.SetSubscriptionStarred(address, "one", true); err != nil {
		t.Fatal(err)
	}
	feed.Items[0].OfflineMedia = map[string]feeds.OfflineAsset{"https://example.test/a.png": {Snapshot: "snapshot-123", File: strings.Repeat("a", 64) + ".media", Size: 7, ContentType: "image/png"}}
	if _, err = s.SaveOfflineSubscription(address, feed, "snapshot-123"); err != nil {
		t.Fatal(err)
	}
	subs, _ := s.ListSubscriptions()
	if !subs[0].OfflinePinned || subs[0].OfflineSnapshot != "snapshot-123" || !subs[0].Feed.Items[0].Starred || len(subs[0].Feed.Items[0].OfflineMedia) != 1 {
		t.Fatalf("sub=%+v", subs[0])
	}
}
