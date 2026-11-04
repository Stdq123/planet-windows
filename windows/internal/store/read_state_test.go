package store

import (
	"testing"

	"windows/internal/feeds"
)

func TestReadStatusSurvivesRefreshAndReopen(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	address := "https://example.test/feed"
	_, err = s.SaveSubscription(address, feeds.Feed{Items: []feeds.Item{{ID: "first", Title: "Old"}, {ID: "second", Title: "Old two"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetSubscriptionRead(address, "first", true); err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveSubscription(address, feeds.Feed{Items: []feeds.Item{{ID: "first", Title: "Updated"}, {ID: "new", Title: "New", Read: true}}})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	subs, err := reopened.ListSubscriptions()
	if err != nil {
		t.Fatal(err)
	}
	items := map[string]feeds.Item{}
	for _, item := range subs[0].Feed.Items {
		items[item.ID] = item
	}
	if !items["first"].Read || items["first"].Title != "Updated" {
		t.Fatalf("read state lost: %+v", items["first"])
	}
	if items["new"].Read || items["second"].Read {
		t.Fatalf("new or old unread item became read: %+v", items)
	}
	if err = reopened.SetSubscriptionRead(address, "first", false); err != nil {
		t.Fatal(err)
	}
	subs, err = reopened.ListSubscriptions()
	if err != nil || subs[0].Feed.Items[0].Read {
		t.Fatalf("could not mark unread: %+v %v", subs, err)
	}
	if err = reopened.SetSubscriptionRead(address, "missing", true); err == nil {
		t.Fatal("unknown item was accepted")
	}
}

func TestMarkSubscriptionReadOnlyChangesThatSubscription(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, b := "https://a.test/feed", "https://b.test/feed"
	for _, address := range []string{a, b} {
		if _, err = s.SaveSubscription(address, feeds.Feed{Items: []feeds.Item{{ID: "one"}, {ID: "two"}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.MarkSubscriptionRead(a); err != nil {
		t.Fatal(err)
	}
	subs, err := s.ListSubscriptions()
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range subs {
		for _, item := range sub.Feed.Items {
			if item.Read != (sub.URL == a) {
				t.Fatalf("wrong read state: %+v %+v", sub, item)
			}
		}
	}
	if err = s.MarkSubscriptionRead("missing"); err == nil {
		t.Fatal("unknown subscription accepted")
	}
}
