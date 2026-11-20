package store

import (
	"encoding/json"
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

func TestHybridSearchAndRelatedArticles(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Home", "")
	if err != nil {
		t.Fatal(err)
	}
	a1, err := s.SaveArticle(site.ID, "", "去中心化存储与 IPFS 协议指南", "IPFS 与 IPNS 让个人博客可以在点对点网络中分发，配合 Kubo 节点实现去中心化存储。")
	if err != nil {
		t.Fatal(err)
	}
	a2, err := s.SaveArticle(site.ID, "", "Kubo 节点与点对点网络优化", "配置 Kubo 路由与 IPNS 记录签名，提升去中心化存储在 DHT 中的可达性。")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveArticle(site.ID, "", "周末烘焙食谱", "使用高筋面粉、黄油与酵母制作法式面包。")
	if err != nil {
		t.Fatal(err)
	}
	results, err := s.SearchArticles("去中心化存储 Kubo")
	if err != nil || len(results) < 2 {
		t.Fatalf("expected at least 2 hybrid search hits, got %+v (%v)", results, err)
	}
	if results[0].RelevanceScore <= 0 || results[0].BM25Score <= 0 {
		t.Fatalf("expected non-zero RRF and BM25 scores: %+v", results[0])
	}
	related, err := s.RelatedArticles(site.ID, a1.ID, "", "", 5)
	if err != nil || len(related) == 0 {
		t.Fatalf("expected related articles for a1, got %+v (%v)", related, err)
	}
	if related[0].ArticleID != a2.ID {
		t.Fatalf("expected top related article to be a2 (%s), got %+v", a2.ID, related[0])
	}
}

func TestImportedSiteSettingsAndArticleSettingsAreEditable(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	siteID := "11111111-1111-4111-8111-111111111111"
	postID := "22222222-2222-4222-8222-222222222222"
	if err := s.Import(Site{ID: siteID, Name: "Imported", IPNS: "k51test", Imported: true}, []Article{{ID: postID, SiteID: siteID, Title: "Imported Post", Content: "Hello"}}, json.RawMessage(`{"articles":[]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateSiteDetails(siteID, "Imported Updated", "About", true, "imported-slug", "Author"); err != nil {
		t.Fatalf("UpdateSiteDetails on imported site failed: %v", err)
	}
	if _, err := s.UpdateSiteProfile(siteID, SiteProfile{Domain: "example.eth", GithubUsername: "planet"}); err != nil {
		t.Fatalf("UpdateSiteProfile on imported site failed: %v", err)
	}
	if _, err := s.UpdateArticleSettings(siteID, postID, "custom-path", 0, "", true, 2); err != nil {
		t.Fatalf("UpdateArticleSettings on imported site failed: %v", err)
	}
	if _, err := s.SetArticlePinned(siteID, postID, true); err != nil {
		t.Fatalf("SetArticlePinned on imported site failed: %v", err)
	}
}
