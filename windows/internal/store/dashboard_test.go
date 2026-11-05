package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"windows/internal/feeds"
)

func TestDashboardCountsOnlyActiveContentAndReturnsMetadata(t *testing.T) {
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.UTC)
	s := &Store{data: data{
		Sites:         []Site{{ID: "a", Name: "A", LastPublished: &now, LastPublishedCID: "cid", ContentRevision: 2, PublishedRevision: 1}, {ID: "b", Name: "B"}, {ID: "removed", Removed: true}, {ID: "deleting", DeletionPending: true}},
		Articles:      []Article{{ID: "1", SiteID: "a", Title: "First", Content: "PRIVATE_BODY", Modified: now}, {ID: "2", SiteID: "b", Modified: now}, {ID: "3", SiteID: "removed"}, {ID: "4", SiteID: "missing"}, {ID: "5", SiteID: "deleting"}},
		Drafts:        []Draft{{ID: "d", SiteID: "b", Content: "PRIVATE_DRAFT", Modified: now}, {ID: "old", SiteID: "removed"}},
		Subscriptions: []Subscription{{Feed: feeds.Feed{Items: []feeds.Item{{Read: false}, {Read: true}}}}},
	}}
	got := s.Dashboard()
	if got.SiteCount != 2 || got.ArticleCount != 2 || got.DraftCount != 1 || got.UnreadCount != 1 {
		t.Fatalf("wrong counts: %+v", got)
	}
	if len(got.Sites) != 2 || !got.Sites[0].PublishPending || !got.Sites[0].HasPublished || got.Sites[1].DraftCount != 1 {
		t.Fatalf("wrong site metadata: %+v", got.Sites)
	}
	encoded, err := json.Marshal(got)
	if err != nil || strings.Contains(string(encoded), "PRIVATE") || strings.Contains(string(encoded), "content\"") {
		t.Fatalf("snapshot exposes body or failed serialization: %s, %v", encoded, err)
	}
	*got.Sites[0].LastPublished = time.Time{}
	if s.data.Sites[0].LastPublished.IsZero() {
		t.Fatal("snapshot aliases stored publication time")
	}
}

func TestDashboardRecentListsAreBoundedSortedAndStable(t *testing.T) {
	s := &Store{data: data{Sites: []Site{{ID: "s", Name: "Site"}}}}
	for i := 9; i >= 0; i-- {
		id := fmt.Sprint(i)
		modified := time.Unix(int64(i/2), 0)
		s.data.Articles = append(s.data.Articles, Article{ID: id, SiteID: "s", Modified: modified})
		s.data.Drafts = append(s.data.Drafts, Draft{ID: id, SiteID: "s", Modified: modified})
	}
	got := s.Dashboard()
	if got.ArticleCount != 10 || got.DraftCount != 10 || len(got.RecentArticles) != 6 || len(got.RecentDrafts) != 6 {
		t.Fatalf("wrong totals or limits: %+v", got)
	}
	for i, want := range []string{"8", "9", "6", "7", "4", "5"} {
		if got.RecentArticles[i].ID != want || got.RecentDrafts[i].ID != want {
			t.Fatalf("entry %d: expected %s", i, want)
		}
	}
	empty := (&Store{}).Dashboard()
	if empty.Sites == nil || empty.RecentArticles == nil || empty.RecentDrafts == nil {
		t.Fatal("empty lists must serialize as arrays")
	}
}
