package store

import (
	"testing"
)

func TestPublishedFoldersAndSiteAggregation(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	folderDir := t.TempDir()
	folder, err := s.AddPublishedFolder("Docs Folder", folderDir, true)
	if err != nil {
		t.Fatal(err)
	}
	if folder.Name != "Docs Folder" || folder.KeyName == "" || !folder.AutoPublish {
		t.Fatalf("unexpected folder: %+v", folder)
	}
	updated, err := s.RecordFolderPublication(folder.ID, "k51folder", "bafyfolder1")
	if err != nil {
		t.Fatal(err)
	}
	if updated.LastCID != "bafyfolder1" || len(updated.History) != 1 {
		t.Fatalf("unexpected recorded publication: %+v", updated)
	}
	if err := s.RemovePublishedFolder(folder.ID); err != nil {
		t.Fatal(err)
	}
	remaining, err := s.ListPublishedFolders()
	if err != nil || len(remaining) != 0 {
		t.Fatalf("expected 0 folders after remove, got %+v (%v)", remaining, err)
	}

	site, err := s.CreateSite("Agg Blog", "")
	if err != nil {
		t.Fatal(err)
	}
	aggSite, err := s.UpdateSiteAggregation(site.ID, []string{"https://example.com/feed.xml", "vitalik.eth", "vitalik.eth", ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(aggSite.Aggregation) != 2 || aggSite.Aggregation[1] != "vitalik.eth" {
		t.Fatalf("unexpected aggregation sources: %+v", aggSite.Aggregation)
	}
}
