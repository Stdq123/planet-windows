package attachments

import (
	"os"
	"path/filepath"
	"testing"
	"windows/internal/store"
)

func TestAddAndCopyAttachment(t *testing.T) {
	root := t.TempDir()
	library, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := library.CreateSite("Test", "")
	if err != nil {
		t.Fatal(err)
	}
	article, err := library.SaveArticle(site.ID, "", "Title", "Text")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "photo.png")
	if err = os.WriteFile(source, []byte("image content"), 0600); err != nil {
		t.Fatal(err)
	}
	updated, err := Add(root, library, site.ID, article.ID, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Attachments) != 1 || updated.Attachments[0] != "photo.png" {
		t.Fatalf("attachments=%v", updated.Attachments)
	}
	reopened, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	posts, err := reopened.ListArticles(site.ID)
	if err != nil || len(posts) != 1 || len(posts[0].Attachments) != 1 {
		t.Fatalf("posts=%v err=%v", posts, err)
	}
	out := t.TempDir()
	if err = os.Mkdir(filepath.Join(out, article.ID), 0700); err != nil {
		t.Fatal(err)
	}
	if err = CopyToSite(root, out, posts); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, article.ID, "photo.png"))
	if err != nil || string(b) != "image content" {
		t.Fatalf("asset=%q err=%v", b, err)
	}
	if _, err = Add(root, library, site.ID, article.ID, source); err == nil {
		t.Fatal("duplicate attachment accepted")
	}
	reserved := filepath.Join(t.TempDir(), "index.html")
	os.WriteFile(reserved, []byte("bad"), 0600)
	if _, err = Add(root, library, site.ID, article.ID, reserved); err == nil {
		t.Fatal("reserved filename accepted")
	}
	if _, err = Add(root, library, "wrong", article.ID, source); err == nil {
		t.Fatal("cross-site attachment accepted")
	}
	updated, err = Remove(root, library, site.ID, article.ID, "photo.png")
	if err != nil || len(updated.Attachments) != 0 {
		t.Fatalf("remove=%v err=%v", updated.Attachments, err)
	}
	if _, err = os.Stat(filepath.Join(root, "attachments", site.ID, article.ID, "photo.png")); !os.IsNotExist(err) {
		t.Fatalf("attachment remains: %v", err)
	}
}
