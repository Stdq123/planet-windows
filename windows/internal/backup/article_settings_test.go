package backup

import (
	"encoding/json"
	"testing"
	"time"
	"windows/internal/store"
)

func TestNativeBackupIncludesArticleSettings(t *testing.T) {
	const siteID = "11111111-1111-4111-8111-111111111111"
	const articleID = "22222222-2222-4222-8222-222222222222"
	site := store.Site{ID: siteID, Name: "Site", Created: time.Now()}
	a := store.Article{ID: articleID, SiteID: siteID, Title: "About", Created: time.Now(), Modified: time.Now(), Slug: "about", ArticleType: 1, ExternalLink: "https://example.com/", IsIncludedInNavigation: true, NavigationWeight: 3}
	b, err := Merge(nil, site, []store.Article{a})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Articles []struct {
			Slug         string `json:"slug"`
			Link         string `json:"link"`
			ArticleType  int    `json:"articleType"`
			ExternalLink string `json:"externalLink"`
			Navigation   bool   `json:"isIncludedInNavigation"`
			Weight       int    `json:"navigationWeight"`
		} `json:"articles"`
	}
	if err = json.Unmarshal(b, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Articles) != 1 || result.Articles[0].Slug != "about" || result.Articles[0].Link != "/about/" || result.Articles[0].ArticleType != 1 || result.Articles[0].ExternalLink != "https://example.com/" || !result.Articles[0].Navigation || result.Articles[0].Weight != 3 {
		t.Fatalf("missing settings: %s", b)
	}
}

func TestNativeBackupIncludesSiteIdentity(t *testing.T) {
	site := store.Site{ID: "11111111-1111-4111-8111-111111111111", Name: "Site", Slug: "my-site", AuthorName: "Ada", Created: time.Now()}
	b, err := Merge(nil, site, nil)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Slug   string `json:"slug"`
		Author string `json:"authorName"`
	}
	if err = json.Unmarshal(b, &data); err != nil || data.Slug != "my-site" || data.Author != "Ada" {
		t.Fatalf("identity missing: %s %v", b, err)
	}
}

func TestDefaultNativeBackupRemainsCompatibleWithExistingRebuildChecks(t *testing.T) {
	const siteID = "11111111-1111-4111-8111-111111111111"
	site := store.Site{ID: siteID, Name: "Site", Created: time.Now()}
	article := store.Article{ID: "22222222-2222-4222-8222-222222222222", SiteID: siteID, Title: "Post", Content: "Body", Created: time.Now(), Modified: time.Now(), NavigationWeight: 1}
	b, err := Merge(nil, site, []store.Article{article})
	if err != nil {
		t.Fatal(err)
	}
	if err = CheckSimplePlain(b, site, []store.Article{article}); err != nil {
		t.Fatalf("default native backup no longer rebuilds: %v; %s", err, b)
	}
}
