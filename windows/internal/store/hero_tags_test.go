package store

import (
	"encoding/json"
	"testing"
)

func TestManualHeroImageSurvivesSaveAndClears(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	article, err := s.SaveArticle(site.ID, "", "Hello", "Body ![first](a.png) more ![second](b.png)")
	if err != nil {
		t.Fatal(err)
	}
	// Adding attachments records them; the auto cover matches the macOS first-image rule.
	if _, err = s.AddAttachmentWithMetadata(site.ID, article.ID, "a.png", nil, nil); err != nil {
		t.Fatal(err)
	}
	article, err = s.AddAttachmentWithMetadata(site.ID, article.ID, "b.png", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if article.HeroImage != "a.png" {
		t.Fatalf("expected derived cover a.png, got %q", article.HeroImage)
	}
	// An explicit cover choice wins over derivation and survives content saves.
	article, err = s.SetHeroImage(site.ID, article.ID, "b.png")
	if err != nil || !article.HeroImageManual || article.HeroImage != "b.png" {
		t.Fatalf("manual cover not recorded: %+v %v", article, err)
	}
	article, err = s.SaveArticle(site.ID, article.ID, "Hello", "Body ![first](a.png) more ![second](b.png)")
	if err != nil {
		t.Fatal(err)
	}
	if !article.HeroImageManual || article.HeroImage != "b.png" {
		t.Fatalf("manual cover lost after save: %+v", article)
	}
	// Clearing the cover keeps it explicit so saving does not re-derive.
	article, err = s.SetHeroImage(site.ID, article.ID, "")
	if err != nil || !article.HeroImageManual || article.HeroImage != "" {
		t.Fatalf("cover not cleared: %+v %v", article, err)
	}
	article, err = s.SaveArticle(site.ID, article.ID, "Hello", "Body ![first](a.png) more ![second](b.png)")
	if err != nil {
		t.Fatal(err)
	}
	if !article.HeroImageManual || article.HeroImage != "" {
		t.Fatalf("cleared cover was re-derived: %+v", article)
	}
	// Removing the manually chosen cover attachment leaves the article explicitly
	// without a cover, matching the macOS writer.
	article, err = s.SetHeroImage(site.ID, article.ID, "b.png")
	if err != nil {
		t.Fatal(err)
	}
	article, err = s.RemoveAttachment(site.ID, article.ID, "b.png")
	if err != nil {
		t.Fatal(err)
	}
	if !article.HeroImageManual || article.HeroImage != "" {
		t.Fatalf("cover removal should stay explicitly cleared: %+v", article)
	}
	if _, err = s.SetHeroImage(site.ID, article.ID, "missing.png"); err == nil {
		t.Fatal("expected an error for an unregistered cover attachment")
	}
}

func TestDraftHeroImageFollowsTheDraft(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := s.SaveDraft(site.ID, "", "", "Draft", "Body")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ChangeDraftAttachment(site.ID, draft.ID, "cover.png", true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetDraftHeroImage(site.ID, draft.ID, "cover.png"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ChangeDraftAttachment(site.ID, draft.ID, "cover.png", false); err != nil {
		t.Fatal(err)
	}
	drafts, err := s.ListDrafts(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if drafts[0].HeroImage != "" || drafts[0].HeroImageManual {
		t.Fatalf("removing the cover attachment should clear the draft cover: %+v", drafts[0])
	}
}

func TestSiteAvailableTagsAggregatesLabelCounts(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		title string
		tags  map[string]string
	}{
		{"One", map[string]string{"go": "Go", "swift": "Swift"}},
		{"Two", map[string]string{"go-2": "Go"}},
	} {
		if _, err = s.SaveArticleWithTags(site.ID, "", entry.title, "Body", mustTagsJSON(t, entry.tags)); err != nil {
			t.Fatal(err)
		}
	}
	usage, err := s.SiteAvailableTags(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(usage) != 2 {
		t.Fatalf("expected two labels, got %+v", usage)
	}
	// Alphabetical order mirrors the macOS writer; the reserved key never appears.
	if usage[0].Label != "Go" || usage[0].Count != 2 || usage[1].Label != "Swift" || usage[1].Count != 1 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

func mustTagsJSON(t *testing.T, tags map[string]string) string {
	t.Helper()
	data, err := json.Marshal(tags)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
