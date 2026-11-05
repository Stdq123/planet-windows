package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/egoist/mygo"
)

func TestFollowHomepageRefreshPreservesReadingState(t *testing.T) {
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	revision := 1
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprintf(w, `<!doctype html><head><link rel="preload" href="/rss-%d.xml" as="fetch" type="application/rss+xml"></head>`, revision)
			return
		}
		if fail {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path != fmt.Sprintf("/rss-%d.xml", revision) {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `<rss version="2.0"><channel><title>Site</title><item><guid>one</guid><title>Revision %d</title></item></channel></rss>`, revision)
	}))
	defer server.Close()
	lib := &Library{}
	sub, err := lib.Follow(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if sub.URL != server.URL || len(sub.Feed.Items) != 1 {
		t.Fatalf("subscription=%+v", sub)
	}
	key := sub.Feed.Items[0].Key
	if err = lib.StarSubscription(sub.URL, key, true); err != nil {
		t.Fatal(err)
	}
	if err = lib.SetSubscriptionRead(sub.URL, key, true); err != nil {
		t.Fatal(err)
	}
	revision = 2
	reopened := &Library{}
	sub, err = reopened.RefreshSubscription(server.URL)
	if err != nil || len(sub.Feed.Items) != 1 || sub.Feed.Items[0].Title != "Revision 2" || !sub.Feed.Items[0].Starred || !sub.Feed.Items[0].Read {
		t.Fatalf("refreshed subscription=%+v err=%v", sub, err)
	}
	fail = true
	if _, err = reopened.RefreshSubscription(server.URL); err == nil {
		t.Fatal("expected a refresh failure")
	}
	subs, err := reopened.Subscriptions()
	if err != nil || len(subs) != 1 || subs[0].LastError == "" || subs[0].Feed.Items[0].Title != "Revision 2" || !subs[0].Feed.Items[0].Starred || !subs[0].Feed.Items[0].Read {
		t.Fatalf("cached subscriptions=%+v err=%v", subs, err)
	}
}

// Opt in to read the public site; all subscription state stays in a temporary profile.
func TestLiveFollowOlividaHomepage(t *testing.T) {
	if os.Getenv("PLANET_TEST_FEEDS_LIVE") != "1" {
		t.Skip("live feed test is opt-in")
	}
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	lib := &Library{}
	sub, err := lib.Follow("https://olivida.eth.limo")
	if err != nil {
		t.Fatal(err)
	}
	if sub.Feed.Title != "olivida.eth" || len(sub.Feed.Items) == 0 {
		t.Fatalf("title=%q items=%d", sub.Feed.Title, len(sub.Feed.Items))
	}
	saved, err := (&Library{}).Subscriptions()
	if err != nil || len(saved) != 1 || len(saved[0].Feed.Items) != len(sub.Feed.Items) {
		t.Fatalf("subscription was not persisted: %v", err)
	}
	t.Logf("Followed %s: title=%q items=%d first=%q", sub.URL, sub.Feed.Title, len(sub.Feed.Items), sub.Feed.Items[0].Title)
}
