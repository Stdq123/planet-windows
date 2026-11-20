package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/egoist/mygo"
	"windows/internal/siteimage"
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

func TestFollowHomepageFetchesAndPersistsSiteIcon(t *testing.T) {
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	img.SetNRGBA(0, 0, color.NRGBA{R: 25, G: 130, B: 230, A: 255})
	var iconBuf bytes.Buffer
	if err := png.Encode(&iconBuf, img); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<!doctype html><head><link rel="apple-touch-icon" href="/avatar.png"><link rel="alternate" type="application/rss+xml" href="/rss.xml"></head>`)
		case "/rss.xml":
			fmt.Fprint(w, `<rss version="2.0"><channel><title>Icon Site</title><item><guid>1</guid><title>Hello</title></item></channel></rss>`)
		case "/avatar.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(iconBuf.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	lib := &Library{}
	sub, err := lib.Follow(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	decoded, _, err := siteimage.DecodePNG(sub.Feed.IconPNG)
	if err != nil || decoded.Bounds().Dx() != 32 {
		t.Fatalf("expected persisted 32x32 icon, err=%v", err)
	}
	saved, err := (&Library{}).Subscriptions()
	if err != nil || len(saved) != 1 || saved[0].Feed.IconPNG != sub.Feed.IconPNG {
		t.Fatalf("saved subscription icon mismatch: %v", err)
	}
}

func TestUpdateSubscriptionEditsTitleAndReplacesAddress(t *testing.T) {
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/old.xml":
			fmt.Fprint(w, `<rss version="2.0"><channel><title>Old Feed</title><item><guid>shared</guid><title>Shared Post</title></item></channel></rss>`)
		case "/new.xml":
			fmt.Fprint(w, `<rss version="2.0"><channel><title>New Feed</title><item><guid>shared</guid><title>Shared Post Updated</title></item><item><guid>second</guid><title>Second</title></item></channel></rss>`)
		case "/other.xml":
			fmt.Fprint(w, `<rss version="2.0"><channel><title>Other</title><item><guid>o1</guid><title>Other Post</title></item></channel></rss>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	lib := &Library{}
	sub, err := lib.Follow(server.URL + "/old.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Follow(server.URL + "/other.xml"); err != nil {
		t.Fatal(err)
	}
	if err := lib.StarSubscription(sub.URL, "shared", true); err != nil {
		t.Fatal(err)
	}
	if err := lib.SetSubscriptionRead(sub.URL, "shared", true); err != nil {
		t.Fatal(err)
	}

	// Rename only (same URL) persists and survives refresh.
	renamed, err := lib.UpdateSubscription(sub.URL, sub.URL, "我的自定义名称")
	if err != nil || renamed.CustomTitle != "我的自定义名称" {
		t.Fatalf("rename failed: sub=%+v err=%v", renamed, err)
	}
	refreshed, err := lib.RefreshSubscription(sub.URL)
	if err != nil || refreshed.CustomTitle != "我的自定义名称" {
		t.Fatalf("custom title lost on refresh: sub=%+v err=%v", refreshed, err)
	}

	// Colliding target URL is rejected without altering existing subscriptions.
	if _, err := lib.UpdateSubscription(sub.URL, server.URL+"/other.xml", "冲突"); err == nil {
		t.Fatal("expected duplicate subscription URL to be rejected")
	}

	// Failed fetch keeps the original subscription untouched.
	if _, err := lib.UpdateSubscription(sub.URL, server.URL+"/missing.xml", "失败"); err == nil {
		t.Fatal("expected unreachable new URL to fail")
	}

	// Replace URL preserves position, custom title, and matching item star/read state.
	replaced, err := lib.UpdateSubscription(sub.URL, server.URL+"/new.xml", "新名称")
	if err != nil {
		t.Fatal(err)
	}
	if replaced.URL != server.URL+"/new.xml" || replaced.CustomTitle != "新名称" || len(replaced.Feed.Items) != 2 {
		t.Fatalf("unexpected replaced subscription: %+v", replaced)
	}
	if !replaced.Feed.Items[0].Starred || !replaced.Feed.Items[0].Read {
		t.Fatalf("star/read state lost after URL replacement: %+v", replaced.Feed.Items[0])
	}

	reopened, err := (&Library{}).Subscriptions()
	if err != nil || len(reopened) != 2 || reopened[0].URL != server.URL+"/new.xml" || reopened[0].CustomTitle != "新名称" {
		t.Fatalf("reopened subscriptions mismatch: %+v err=%v", reopened, err)
	}
}
