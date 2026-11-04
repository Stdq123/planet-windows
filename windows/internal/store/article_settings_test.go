package store

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestArticleSettingsValidateAndPersist(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("site", "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.SaveArticle(site.ID, "", "First", "Content")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.SaveArticle(site.ID, "", "Second", "Content")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.UpdateArticleSettings(site.ID, first.ID, "hello-world", 1, "https://example.com/page", true, -2)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Slug != "hello-world" || updated.ArticleType != 1 || updated.ExternalLink != "https://example.com/page" || !updated.IsIncludedInNavigation || updated.NavigationWeight != -2 {
		t.Fatalf("wrong settings: %+v", updated)
	}
	for _, invalid := range []string{"../evil", "assets", "tags", "CON", second.ID, "HELLO-WORLD", "under_score"} {
		if _, err := s.UpdateArticleSettings(site.ID, second.ID, invalid, 0, "", false, 1); err == nil {
			t.Errorf("accepted conflicting or unsafe slug %q", invalid)
		}
	}
	for _, bad := range []string{"javascript:alert(1)", "https://user:pass@example.com", "//example.com", `https://example.com/?q=";alert(1)//`, "https://example.com/</script>", "https://example.com/a\\b", "https://example.com/\u2028evil"} {
		if _, err := s.UpdateArticleSettings(site.ID, second.ID, "", 0, bad, false, 1); err == nil {
			t.Errorf("accepted link %q", bad)
		}
	}
	if _, err := s.UpdateArticleSettings(site.ID, second.ID, "", 2, "", false, 1); err == nil {
		t.Error("accepted invalid article type")
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	articles, err := reopened.ListArticles(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range articles {
		if a.ID == first.ID && (a.Slug != "hello-world" || a.ArticleType != 1 || a.NavigationWeight != -2) {
			t.Fatalf("settings lost: %+v", a)
		}
	}
}

func TestZeroNavigationWeightIsExposedAndPersisted(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.SaveArticle(site.ID, "", "Title", "")
	if err != nil {
		t.Fatal(err)
	}
	a, err = s.UpdateArticleSettings(site.ID, a.ID, "", 0, "", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"navigationWeight":0`) {
		t.Fatalf("zero not present in API: %s", b)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	items, err := reopened.ListArticles(site.ID)
	if err != nil || len(items) != 1 || items[0].NavigationWeight != 0 {
		t.Fatalf("zero not persisted: %+v %v", items, err)
	}
}

func TestLegacyArticleDefaultsNavigationWeightToOne(t *testing.T) {
	var a Article
	if err := json.Unmarshal([]byte(`{"id":"a"}`), &a); err != nil {
		t.Fatal(err)
	}
	if a.NavigationWeight != 1 {
		t.Fatalf("legacy weight=%d", a.NavigationWeight)
	}
	if err := json.Unmarshal([]byte(`{"id":"a","navigationWeight":0}`), &a); err != nil || a.NavigationWeight != 0 {
		t.Fatalf("explicit zero lost: %+v %v", a, err)
	}
}
