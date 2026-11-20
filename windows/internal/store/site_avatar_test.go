package store

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"

	"windows/internal/feeds"
)

func TestSiteAvatarPersistsAndRejectsInvalidImages(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	avatar := base64.StdEncoding.EncodeToString(encoded.Bytes())
	updated, err := s.SetSiteAvatar(site.ID, avatar)
	if err != nil || updated.AvatarPNG != avatar {
		t.Fatalf("avatar not saved: %+v %v", updated, err)
	}
	if _, err = s.SetSiteAvatar(site.ID, base64.StdEncoding.EncodeToString([]byte("not an image"))); err == nil {
		t.Fatal("invalid image accepted")
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	sites, err := reopened.ListSites()
	if err != nil || len(sites) != 1 || sites[0].AvatarPNG != avatar {
		t.Fatalf("avatar not persisted: %+v %v", sites, err)
	}
	updated, err = reopened.SetSiteAvatar(site.ID, "")
	if err != nil || updated.AvatarPNG != "" {
		t.Fatalf("avatar not cleared: %+v %v", updated, err)
	}
}

func TestSubscriptionIconPersistsAndSurvivesEmptyRefresh(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	img1 := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img1.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	var buf1 bytes.Buffer
	if err = png.Encode(&buf1, img1); err != nil {
		t.Fatal(err)
	}
	icon1 := base64.StdEncoding.EncodeToString(buf1.Bytes())

	img2 := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img2.Set(0, 0, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	var buf2 bytes.Buffer
	if err = png.Encode(&buf2, img2); err != nil {
		t.Fatal(err)
	}
	icon2 := base64.StdEncoding.EncodeToString(buf2.Bytes())

	sub, err := s.SaveSubscription("https://example.test/feed", feeds.Feed{Title: "Blog", IconPNG: icon1})
	if err != nil || sub.Feed.IconPNG != icon1 {
		t.Fatalf("icon not saved: %+v %v", sub, err)
	}
	// Refresh without icon (e.g. transient favicon failure) preserves previous icon.
	sub, err = s.SaveSubscription("https://example.test/feed", feeds.Feed{Title: "Blog"})
	if err != nil || sub.Feed.IconPNG != icon1 {
		t.Fatalf("icon not preserved: %+v %v", sub, err)
	}
	// Refresh with invalid icon string ignores invalid data and preserves previous icon.
	sub, err = s.SaveSubscription("https://example.test/feed", feeds.Feed{Title: "Blog", IconPNG: "not-valid-png"})
	if err != nil || sub.Feed.IconPNG != icon1 {
		t.Fatalf("invalid icon overwrote valid icon: %+v %v", sub, err)
	}
	// Refresh with new valid icon updates it.
	sub, err = s.SaveSubscription("https://example.test/feed", feeds.Feed{Title: "Blog", IconPNG: icon2})
	if err != nil || sub.Feed.IconPNG != icon2 {
		t.Fatalf("icon not updated: %+v %v", sub, err)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	subs, err := reopened.ListSubscriptions()
	if err != nil || len(subs) != 1 || subs[0].Feed.IconPNG != icon2 {
		t.Fatalf("icon not persisted across reopen: %+v %v", subs, err)
	}
}
