package store

import (
	"testing"

	"windows/internal/feeds"
)

func TestSearchFindsActiveSiteAndSubscriptionArticles(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Home", "")
	if err != nil {
		t.Fatal(err)
	}
	post, err := s.SaveArticle(site.ID, "", "First", "Hello 世界")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveSubscription("https://example.test/feed", feeds.Feed{Title: "Feed", Items: []feeds.Item{{ID: "x", Title: "HELLO reader", Content: "body"}}})
	if err != nil {
		t.Fatal(err)
	}
	results, err := s.SearchArticles("hello")
	if err != nil || len(results) != 2 {
		t.Fatalf("search: %+v %v", results, err)
	}
	found := map[string]bool{}
	for _, result := range results {
		if result.SiteID == site.ID && result.ArticleID == post.ID {
			found["site"] = true
		}
		if result.SubscriptionURL == "https://example.test/feed" && result.ItemKey == "x" {
			found["feed"] = true
		}
	}
	if !found["site"] || !found["feed"] {
		t.Fatalf("missing source identity: %+v", results)
	}
	if err = s.RemoveSite(site.ID); err != nil {
		t.Fatal(err)
	}
	results, err = s.SearchArticles("世界")
	if err != nil || len(results) != 0 {
		t.Fatalf("removed site leaked into search: %+v %v", results, err)
	}
	results, err = s.SearchArticles("   ")
	if err != nil || len(results) != 0 {
		t.Fatalf("empty search: %+v %v", results, err)
	}
}
