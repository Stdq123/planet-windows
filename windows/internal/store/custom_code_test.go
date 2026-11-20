package store

import (
	"strings"
	"testing"
)

func TestSetArticlePinnedOrdersPinnedArticlesFirstAndPersists(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.SaveArticle(site.ID, "", "First", "Body 1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.SaveArticle(site.ID, "", "Second", "Body 2")
	if err != nil {
		t.Fatal(err)
	}
	pinnedFirst, err := s.SetArticlePinned(site.ID, first.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if pinnedFirst.Pinned == nil {
		t.Fatal("expected article to be pinned")
	}
	list, err := s.ListArticles(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != first.ID || list[1].ID != second.ID {
		t.Fatalf("unexpected article order: %+v", list)
	}
	unpinned, err := s.SetArticlePinned(site.ID, first.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if unpinned.Pinned != nil {
		t.Fatal("expected article to be unpinned")
	}
}

func TestUpdateSiteCustomCodeValidatesAndPersists(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	pubFalse := false
	prewarmFalse := false
	updated, err := s.UpdateSiteCustomCode(site.ID, SiteCustomCode{
		HeadEnabled:      true,
		Head:             "<meta name=\"custom\" content=\"1\">",
		BodyStartEnabled: true,
		BodyStart:        "<div id=\"banner\">Top</div>",
		BodyEndEnabled:   true,
		BodyEnd:          "<footer>Bottom</footer>",
		PublishAsIPNS:    &pubFalse,
		PrewarmNewPost:   &prewarmFalse,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.CustomCodeHeadEnabled || updated.CustomCodeHead == "" || updated.ShouldPublishIPNS() || updated.ShouldPrewarmNewPost() {
		t.Fatalf("unexpected updated site: %+v", updated)
	}
	if _, err := s.UpdateSiteCustomCode(site.ID, SiteCustomCode{Head: strings.Repeat("a", maxCustomCodeBytes+1)}); err == nil {
		t.Fatal("oversized custom code accepted")
	}
}
