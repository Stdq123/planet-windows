package store

import "testing"

func TestSitePrivacySettingPersistsAndChangesRevision(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	site, err = s.UpdateSiteSettings(site.ID, "Site", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !site.DoNotIndex || site.ContentRevision != 1 {
		t.Fatalf("unexpected update: %+v", site)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	sites, err := reopened.ListSites()
	if err != nil || !sites[0].DoNotIndex {
		t.Fatalf("setting lost: %+v %v", sites, err)
	}
	site, err = reopened.UpdateSiteSettings(site.ID, "Site", "", false)
	if err != nil || site.DoNotIndex || site.ContentRevision != 2 {
		t.Fatalf("could not disable: %+v %v", site, err)
	}
}

func TestNativeSiteIdentitySettingsAndNewArticleReference(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.SaveArticle(site.ID, "", "First", "")
	if err != nil {
		t.Fatal(err)
	}
	site, err = s.UpdateSiteDetails(site.ID, "Renamed", "About", true, "my-site", "Author")
	if err != nil {
		t.Fatal(err)
	}
	if site.Slug != "my-site" || site.AuthorName != "Author" || !site.DoNotIndex {
		t.Fatalf("settings: %+v", site)
	}
	second, err := s.SaveArticle(site.ID, "", "Second", "")
	if err != nil {
		t.Fatal(err)
	}
	if second.ArticleReference != "MY-SITE-2" || first.ArticleReference == second.ArticleReference {
		t.Fatalf("incorrect article references: %s %s", first.ArticleReference, second.ArticleReference)
	}
	if _, err = s.UpdateSiteDetails(site.ID, "Site", "", false, "bad/slug", ""); err == nil {
		t.Fatal("unsafe site slug accepted")
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	sites, err := reopened.ListSites()
	if err != nil || sites[0].Slug != "my-site" || sites[0].AuthorName != "Author" {
		t.Fatalf("settings not persisted: %+v %v", sites, err)
	}
}
