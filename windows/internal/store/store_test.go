package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"windows/internal/feeds"
)

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
