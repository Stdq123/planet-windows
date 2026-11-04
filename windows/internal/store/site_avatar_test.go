package store

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
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
