package store

import "testing"

func TestPodcastSettingsPersistAndValidate(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.UpdatePodcastSettings(site.ID, "zh-CN", true, []string{"Technology", "Education"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.PodcastLanguage != "zh-CN" || !updated.PodcastExplicit || len(updated.PodcastCategories) != 2 {
		t.Fatalf("wrong settings: %+v", updated)
	}
	if _, err = s.UpdatePodcastSettings(site.ID, "bad<>value", false, nil); err == nil {
		t.Fatal("invalid language accepted")
	}
	if _, err = s.UpdatePodcastSettings(site.ID, "en", false, []string{"Not a category"}); err == nil {
		t.Fatal("invalid category accepted")
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	sites, err := reopened.ListSites()
	if err != nil || sites[0].PodcastLanguage != "zh-CN" {
		t.Fatalf("settings lost: %+v %v", sites, err)
	}
}
