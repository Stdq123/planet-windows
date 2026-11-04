package attachments

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"windows/internal/store"
)

func TestDraftAttachmentsStayPrivateUntilPublication(t *testing.T) {
	root := t.TempDir()
	library, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := library.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := library.SaveDraft(site.ID, "", "", "Draft", "Private")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(source, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	draft, err = AddDraft(root, library, site.ID, draft.ID, source)
	if err != nil || len(draft.Attachments) != 1 {
		t.Fatalf("draft=%+v err=%v", draft, err)
	}
	_, articles, _, err := library.PublicationSnapshot(site.ID)
	if err != nil || len(articles) != 0 {
		t.Fatalf("draft leaked: %+v %v", articles, err)
	}
	library, err = store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	article, err := PublishDraft(root, library, site.ID, draft.ID, "Published", "Text")
	if err != nil {
		t.Fatal(err)
	}
	if len(article.Attachments) != 1 || article.Attachments[0] != "private.txt" {
		t.Fatalf("article=%+v", article)
	}
	f, err := OpenRegistered(root, site, article, "private.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil || string(b) != "secret" {
		t.Fatalf("attachment=%q err=%v", b, err)
	}
	if drafts, err := library.ListDrafts(site.ID); err != nil || len(drafts) != 0 {
		t.Fatalf("draft remains: %+v %v", drafts, err)
	}
}

func TestStaleDraftAttachmentPublicationDoesNotChangeArticle(t *testing.T) {
	root := t.TempDir()
	library, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := library.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	article, err := library.SaveArticle(site.ID, "", "Original", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := library.SaveDraft(site.ID, "", article.ID, "Draft", "")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(source, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = AddDraft(root, library, site.ID, draft.ID, source); err != nil {
		t.Fatal(err)
	}
	if _, err = library.SaveArticle(site.ID, article.ID, "Changed", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = PublishDraft(root, library, site.ID, draft.ID, "Draft", ""); err == nil {
		t.Fatal("stale draft published")
	}
	posts, err := library.ListArticles(site.ID)
	if err != nil || len(posts) != 1 || len(posts[0].Attachments) != 0 {
		t.Fatalf("article changed: %+v %v", posts, err)
	}
	drafts, err := library.ListDrafts(site.ID)
	if err != nil || len(drafts) != 1 || len(drafts[0].Attachments) != 1 {
		t.Fatalf("draft lost: %+v %v", drafts, err)
	}
}

func TestDraftAttachmentRemovalAndExistingArticlePublication(t *testing.T) {
	root := t.TempDir()
	lib, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	article, err := lib.SaveArticle(site.ID, "", "Original", "")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "old.txt")
	if err := os.WriteFile(source, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(root, lib, site.ID, article.ID, source); err != nil {
		t.Fatal(err)
	}
	draft, err := lib.SaveDraft(site.ID, "", article.ID, "Draft", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddDraft(root, lib, site.ID, draft.ID, source); err == nil {
		t.Fatal("duplicate existing filename accepted")
	}
	newfile := filepath.Join(t.TempDir(), "new.txt")
	if err := os.WriteFile(newfile, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := AddDraft(root, lib, site.ID, draft.ID, newfile); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveDraft(root, lib, site.ID, draft.ID, "old.txt"); err == nil {
		t.Fatal("removed public article attachment from draft")
	}
	if _, err := RemoveDraft(root, lib, site.ID, draft.ID, "new.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddDraft(root, lib, site.ID, draft.ID, newfile); err != nil {
		t.Fatal(err)
	}
	article, err = PublishDraft(root, lib, site.ID, draft.ID, "Draft", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(article.Attachments) != 2 || article.Attachments[0] != "old.txt" || article.Attachments[1] != "new.txt" {
		t.Fatalf("attachments=%v", article.Attachments)
	}
	if _, err := os.Stat(filepath.Join(root, "draft-attachments", site.ID, draft.ID, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("private copy remains: %v", err)
	}
}

func TestDraftPublicationRollsBackCopiedFilesWhenAnotherFileIsMissing(t *testing.T) {
	root := t.TempDir()
	lib, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	article, err := lib.SaveArticle(site.ID, "", "Original", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := lib.SaveDraft(site.ID, "", article.ID, "Draft", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first.txt", "second.txt"} {
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := AddDraft(root, lib, site.ID, draft.ID, path); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(root, "draft-attachments", site.ID, draft.ID, "second.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishDraft(root, lib, site.ID, draft.ID, "Draft", ""); err == nil {
		t.Fatal("publication succeeded with missing file")
	}
	if _, err := os.Stat(filepath.Join(root, "attachments", site.ID, article.ID, "first.txt")); !os.IsNotExist(err) {
		t.Fatalf("staged file not rolled back: %v", err)
	}
	posts, err := lib.ListArticles(site.ID)
	if err != nil || len(posts) != 1 || posts[0].Title != "Original" || len(posts[0].Attachments) != 0 {
		t.Fatalf("article changed: %+v %v", posts, err)
	}
	drafts, err := lib.ListDrafts(site.ID)
	if err != nil || len(drafts) != 1 || len(drafts[0].Attachments) != 2 {
		t.Fatalf("draft lost: %+v %v", drafts, err)
	}
}

func TestDraftImagePreviewOnlyReadsRegisteredImages(t *testing.T) {
	root := t.TempDir()
	lib, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := lib.SaveDraft(site.ID, "", "", "Draft", "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "photo.png")
	image := []byte("\x89PNG\r\n\x1a\nprivate image")
	if err := os.WriteFile(path, image, 0600); err != nil {
		t.Fatal(err)
	}
	draft, err = AddDraft(root, lib, site.ID, draft.ID, path)
	if err != nil {
		t.Fatal(err)
	}
	data, mime, err := ReadDraftPreviewImage(root, draft, "photo.png")
	if err != nil || string(data) != string(image) || mime != "image/png" {
		t.Fatalf("preview %q %q %v", data, mime, err)
	}
	if _, _, err := ReadDraftPreviewImage(root, draft, "../photo.png"); err == nil {
		t.Fatal("path traversal accepted")
	}
	if _, _, err := ReadDraftPreviewImage(root, draft, "unregistered.png"); err == nil {
		t.Fatal("unregistered image accepted")
	}
}

func TestDraftCleanupIsRetriedAfterRestart(t *testing.T) {
	root := t.TempDir()
	lib, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := lib.SaveDraft(site.ID, "", "", "Draft", "")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(file, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = AddDraft(root, lib, site.ID, draft.ID, file); err != nil {
		t.Fatal(err)
	}
	if err = lib.DeleteDraft(site.ID, draft.ID); err != nil {
		t.Fatal(err)
	}
	lib, err = store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.PendingDraftCleanup()) != 1 {
		t.Fatal("cleanup was not persisted")
	}
	CleanupDraftFiles(root, lib)
	if len(lib.PendingDraftCleanup()) != 0 {
		t.Fatal("cleanup remains pending")
	}
	if _, err := os.Stat(filepath.Join(root, "draft-attachments", site.ID, draft.ID, "private.txt")); !os.IsNotExist(err) {
		t.Fatalf("private file remains: %v", err)
	}
}

func TestDraftAttachmentCanReuseIdenticalFilesFromInterruptedOperations(t *testing.T) {
	root := t.TempDir()
	lib, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	article, err := lib.SaveArticle(site.ID, "", "Original", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := lib.SaveDraft(site.ID, "", article.ID, "Draft", "")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(source, []byte("identical"), 0600); err != nil {
		t.Fatal(err)
	}
	privateDir := filepath.Join(root, "draft-attachments", site.ID, draft.ID)
	if err := os.MkdirAll(privateDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(privateDir, "photo.png"), []byte("identical"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := AddDraft(root, lib, site.ID, draft.ID, source); err != nil {
		t.Fatalf("could not resume add: %v", err)
	}
	publicDir := filepath.Join(root, "attachments", site.ID, article.ID)
	if err := os.MkdirAll(publicDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(publicDir, "photo.png"), []byte("different"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishDraft(root, lib, site.ID, draft.ID, "Draft", ""); err == nil {
		t.Fatal("different file reused")
	}
	if err := os.WriteFile(filepath.Join(publicDir, "photo.png"), []byte("identical"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishDraft(root, lib, site.ID, draft.ID, "Draft", ""); err != nil {
		t.Fatalf("could not resume publish: %v", err)
	}
}

func TestCleanupRetainsUnremovableEntriesAndCleansAbandonedStaging(t *testing.T) {
	root := t.TempDir()
	lib, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := lib.SaveDraft(site.ID, "", "", "Draft", "")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(source, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := AddDraft(root, lib, site.ID, draft.ID, source); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "draft-attachments", site.ID, draft.ID)
	file := filepath.Join(dir, "private.txt")
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(dir, ".draft-stage-interrupted")
	if err := os.WriteFile(stage, []byte("partial secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := lib.DeleteDraft(site.ID, draft.ID); err != nil {
		t.Fatal(err)
	}
	CleanupDraftFiles(root, lib)
	if len(lib.PendingDraftCleanup()) != 1 {
		t.Fatal("failed cleanup not retained")
	}
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Fatalf("abandoned staging remains: %v", err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	CleanupDraftFiles(root, lib)
	if len(lib.PendingDraftCleanup()) != 0 {
		t.Fatal("cleanup not retried")
	}
}

func TestInterruptedDraftAddIsTrackedAndCleanedAfterRestart(t *testing.T) {
	root := t.TempDir()
	lib, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := lib.SaveDraft(site.ID, "", "", "Draft", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := lib.BeginDraftAttachment(site.ID, draft.ID, "private.txt"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "draft-attachments", site.ID, draft.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "private.txt")
	if err := os.WriteFile(path, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	lib, err = store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	CleanupDraftFiles(root, lib)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("orphan remains: %v", err)
	}
	if len(lib.PendingDraftStaging()) != 0 {
		t.Fatal("staging entry remains")
	}
}

func TestDraftExistingAttachmentRemovalIsDeferredAndCanBeReadded(t *testing.T) {
	root := t.TempDir()
	lib, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	article, err := lib.SaveArticle(site.ID, "", "Original", "")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(source, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(root, lib, site.ID, article.ID, source); err != nil {
		t.Fatal(err)
	}
	draft, err := lib.SaveDraft(site.ID, "", article.ID, "Draft", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetDraftExistingAttachmentRemoved(site.ID, draft.ID, "image.png", true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "attachments", site.ID, article.ID, "image.png")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file removed before publication: %v", err)
	}
	if _, err := PublishDraft(root, lib, site.ID, draft.ID, "Draft", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file remains after publication: %v", err)
	}
	if len(lib.PendingArticleCleanup()) != 0 {
		t.Fatal("cleanup still pending")
	}
	if err := os.WriteFile(source, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(root, lib, site.ID, article.ID, source); err != nil {
		t.Fatalf("cannot readd: %v", err)
	}
}

func TestImportedDraftRemovalKeepsArchivedOriginal(t *testing.T) {
	root := t.TempDir()
	lib, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site := store.Site{ID: "site", Name: "Site", IPNS: "test", Imported: true}
	article := store.Article{ID: "post", SiteID: site.ID, Attachments: []string{"old.png"}}
	if err := lib.Import(site, []store.Article{article}, []byte(`{"articles":[]}`)); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "imports", site.ID, article.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "old.png")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	draft, err := lib.SaveDraft(site.ID, "", article.ID, "Draft", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetDraftExistingAttachmentRemoved(site.ID, draft.ID, "old.png", true); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishDraft(root, lib, site.ID, draft.ID, "Draft", ""); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "original" {
		t.Fatalf("original archive changed: %q %v", b, err)
	}
	posts, err := lib.ListArticles(site.ID)
	if err != nil || len(posts) != 1 || len(posts[0].Attachments) != 0 {
		t.Fatalf("attachments: %+v %v", posts, err)
	}
}

func TestStaleDraftRemovalDoesNotRemoveCurrentArticleAttachment(t *testing.T) {
	root := t.TempDir()
	lib, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	article, err := lib.SaveArticle(site.ID, "", "Original", "")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(source, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(root, lib, site.ID, article.ID, source); err != nil {
		t.Fatal(err)
	}
	draft, err := lib.SaveDraft(site.ID, "", article.ID, "Draft", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetDraftExistingAttachmentRemoved(site.ID, draft.ID, "file.txt", true); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SaveArticle(site.ID, article.ID, "Newer", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishDraft(root, lib, site.ID, draft.ID, "Draft", ""); err == nil {
		t.Fatal("stale draft was published")
	}
	if _, err := os.Stat(filepath.Join(root, "attachments", site.ID, article.ID, "file.txt")); err != nil {
		t.Fatalf("current file removed: %v", err)
	}
}

func TestDraftPublishRetriesPendingArticleCleanupBeforeReusingName(t *testing.T) {
	root := t.TempDir()
	lib, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	article, err := lib.SaveArticle(site.ID, "", "Original", "")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(source, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(root, lib, site.ID, article.ID, source); err != nil {
		t.Fatal(err)
	}
	draft, err := lib.SaveDraft(site.ID, "", article.ID, "First", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetDraftExistingAttachmentRemoved(site.ID, draft.ID, "file.txt", true); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.PublishDraft(site.ID, draft.ID, "First", ""); err != nil {
		t.Fatal(err)
	}
	if len(lib.PendingArticleCleanup()) != 1 {
		t.Fatal("expected pending cleanup")
	}
	draft, err = lib.SaveDraft(site.ID, "", article.ID, "Second", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := AddDraft(root, lib, site.ID, draft.ID, source); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishDraft(root, lib, site.ID, draft.ID, "Second", ""); err != nil {
		t.Fatalf("could not retry cleanup: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(root, "attachments", site.ID, article.ID, "file.txt")); err != nil || string(b) != "new" {
		t.Fatalf("wrong file: %q %v", b, err)
	}
}

func TestArticleAttachmentCleanupSurvivesRestart(t *testing.T) {
	root := t.TempDir()
	lib, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	article, err := lib.SaveArticle(site.ID, "", "Original", "")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(source, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(root, lib, site.ID, article.ID, source); err != nil {
		t.Fatal(err)
	}
	draft, err := lib.SaveDraft(site.ID, "", article.ID, "Draft", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetDraftExistingAttachmentRemoved(site.ID, draft.ID, "file.txt", true); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.PublishDraft(site.ID, draft.ID, "Draft", ""); err != nil {
		t.Fatal(err)
	}
	lib, err = store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.PendingArticleCleanup()) != 1 {
		t.Fatal("cleanup was not persisted")
	}
	CleanupArticleFiles(root, lib)
	if len(lib.PendingArticleCleanup()) != 0 {
		t.Fatal("cleanup not completed")
	}
	if _, err := os.Stat(filepath.Join(root, "attachments", site.ID, article.ID, "file.txt")); !os.IsNotExist(err) {
		t.Fatalf("file remains: %v", err)
	}
}

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

func TestImportedAttachmentsCanBeAddedAndRemoved(t *testing.T) {
	root := t.TempDir()
	lib, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site := store.Site{ID: "site", Name: "Site", IPNS: "test", Imported: true}
	post := store.Article{ID: "post", SiteID: site.ID, Attachments: []string{"old.png"}}
	if err = lib.Import(site, []store.Article{post}, []byte(`{"articles":[]}`)); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "imports", site.ID, post.ID)
	if err = os.MkdirAll(archive, 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(archive, "old.png"), []byte("old"), 0600)
	source := filepath.Join(t.TempDir(), "new.png")
	os.WriteFile(source, []byte("new"), 0600)
	updated, err := Add(root, lib, site.ID, post.ID, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Attachments) != 2 {
		t.Fatalf("%+v", updated)
	}
	updated, err = Remove(root, lib, site.ID, post.ID, "old.png")
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Attachments) != 1 || updated.Attachments[0] != "new.png" {
		t.Fatalf("%+v", updated)
	}
	if b, err := os.ReadFile(filepath.Join(archive, "old.png")); err != nil || string(b) != "old" {
		t.Fatalf("archive modified: %q %v", b, err)
	}
}

func TestImportedCurrentFilesUseEditedListAndPreserveOriginal(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "imports", "site", "post")
	os.MkdirAll(archive, 0700)
	os.WriteFile(filepath.Join(archive, "old.png"), []byte("old"), 0600)
	os.WriteFile(filepath.Join(archive, "keep.png"), []byte("keep"), 0600)
	owned := filepath.Join(root, "attachments", "site", "post")
	os.MkdirAll(owned, 0700)
	os.WriteFile(filepath.Join(owned, "new.png"), []byte("new"), 0600)
	out := t.TempDir()
	os.Mkdir(filepath.Join(out, "post"), 0700)
	post := store.Article{ID: "post", SiteID: "site", Attachments: []string{"keep.png", "new.png"}}
	if err := CopyImportedCurrent(root, filepath.Join(root, "imports", "site"), out, post); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"keep.png": "keep", "new.png": "new"} {
		b, err := os.ReadFile(filepath.Join(out, "post", name))
		if err != nil || string(b) != want {
			t.Fatalf("%s: %q %v", name, b, err)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "post", "old.png")); !os.IsNotExist(err) {
		t.Fatalf("removed file copied: %v", err)
	}
}

func TestPreviewOnlyEmbedsRegisteredRelativeImages(t *testing.T) {
	root := t.TempDir()
	library, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := library.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	post, err := library.SaveArticle(site.ID, "", "Post", "")
	if err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(t.TempDir(), "图 片.png")
	png := []byte("\x89PNG\r\n\x1a\nimage-content")
	if err = os.WriteFile(image, png, 0600); err != nil {
		t.Fatal(err)
	}
	post, err = Add(root, library, site.ID, post.ID, image)
	if err != nil {
		t.Fatal(err)
	}
	if data, mime, err := ReadPreviewImage(root, site, post, "图 片.png"); err != nil || mime != "image/png" || string(data) != string(png) {
		t.Fatalf("image=%q mime=%q err=%v", data, mime, err)
	}
	if _, _, err = ReadPreviewImage(root, site, post, "../secret.png"); err == nil {
		t.Fatal("traversal accepted")
	}
	if _, _, err = ReadPreviewImage(root, site, post, "unregistered.png"); err == nil {
		t.Fatal("unregistered file accepted")
	}
}

func TestImportedPreviewUsesArchiveAndOwnedReplacement(t *testing.T) {
	root := t.TempDir()
	site := store.Site{ID: "site", Imported: true}
	post := store.Article{ID: "post", SiteID: site.ID, Attachments: []string{"photo.png"}}
	dir := filepath.Join(root, "imports", site.ID, post.ID)
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "photo.png"), []byte("\x89PNG\r\n\x1a\nold"), 0600)
	data, _, err := ReadPreviewImage(root, site, post, "photo.png")
	if err != nil || !strings.HasSuffix(string(data), "old") {
		t.Fatalf("archive=%q err=%v", data, err)
	}
	owned := filepath.Join(root, "attachments", site.ID, post.ID)
	os.MkdirAll(owned, 0700)
	os.WriteFile(filepath.Join(owned, "photo.png"), []byte("\x89PNG\r\n\x1a\nnew"), 0600)
	data, _, err = ReadPreviewImage(root, site, post, "photo.png")
	if err != nil || !strings.HasSuffix(string(data), "new") {
		t.Fatalf("replacement=%q err=%v", data, err)
	}
}

func TestPreviewRejectsNonImagesAndOversizedFiles(t *testing.T) {
	root := t.TempDir()
	lib, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	post, err := lib.SaveArticle(site.ID, "", "Post", "")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "fake.png")
	os.WriteFile(file, []byte("<svg onload='bad()'></svg>"), 0600)
	post, err = Add(root, lib, site.ID, post.ID, file)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = ReadPreviewImage(root, site, post, "fake.png"); err == nil {
		t.Fatal("non-image accepted")
	}
	path := filepath.Join(root, "attachments", site.ID, post.ID, "fake.png")
	if err = os.Truncate(path, 8<<20+1); err != nil {
		t.Fatal(err)
	}
	if _, _, err = ReadPreviewImage(root, site, post, "fake.png"); err == nil {
		t.Fatal("oversized file accepted")
	}
}

func TestMediaAttachmentSelectionAndRemoval(t *testing.T) {
	root := t.TempDir()
	lib, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	post, err := lib.SaveArticle(site.ID, "", "Post", "")
	if err != nil {
		t.Fatal(err)
	}
	audio := filepath.Join(t.TempDir(), "song.wav")
	os.WriteFile(audio, []byte("test"), 0600)
	post, err = Add(root, lib, site.ID, post.ID, audio)
	if err != nil {
		t.Fatal(err)
	}
	if post.AudioFilename != "song.wav" || post.AudioByteLength == nil || *post.AudioByteLength != 4 {
		t.Fatalf("audio metadata: %+v", post)
	}
	video := filepath.Join(t.TempDir(), "clip.mp4")
	os.WriteFile(video, []byte("video"), 0600)
	post, err = Add(root, lib, site.ID, post.ID, video)
	if err != nil {
		t.Fatal(err)
	}
	if post.VideoFilename != "clip.mp4" {
		t.Fatalf("video metadata: %+v", post)
	}
	post, err = Remove(root, lib, site.ID, post.ID, "song.wav")
	if err != nil {
		t.Fatal(err)
	}
	if post.AudioFilename != "" || post.AudioDuration != nil || post.AudioByteLength != nil {
		t.Fatalf("stale audio metadata: %+v", post)
	}
}

func TestOpenRegisteredAudioRejectsWrongArticleAndSymlinks(t *testing.T) {
	root := t.TempDir()
	lib, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := lib.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	post, err := lib.SaveArticle(site.ID, "", "Post", "")
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "sound.mp3")
	os.WriteFile(src, []byte("sample"), 0600)
	post, err = Add(root, lib, site.ID, post.ID, src)
	if err != nil {
		t.Fatal(err)
	}
	f, err := OpenRegistered(root, site, post, "sound.mp3")
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(f)
	f.Close()
	if err != nil || string(b) != "sample" {
		t.Fatalf("read %q %v", b, err)
	}
	if f, err = OpenRegistered(root, site, post, "../secret"); err == nil {
		f.Close()
		t.Fatal("invalid path accepted")
	}
	if f, err = OpenRegistered(root, store.Site{ID: "wrong"}, post, "sound.mp3"); err == nil {
		f.Close()
		t.Fatal("wrong site accepted")
	}
}

func TestEnrichMediaRestoresSizeForOlderArticles(t *testing.T) {
	root := t.TempDir()
	lib, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, _ := lib.CreateSite("Site", "")
	post, _ := lib.SaveArticle(site.ID, "", "Post", "")
	input := filepath.Join(t.TempDir(), "song.mp3")
	os.WriteFile(input, []byte("123456"), 0600)
	post, err = Add(root, lib, site.ID, post.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	post.AudioByteLength = nil
	enriched, err := EnrichMedia(root, site, []store.Article{post})
	if err != nil {
		t.Fatal(err)
	}
	if enriched[0].AudioByteLength == nil || *enriched[0].AudioByteLength != 6 {
		t.Fatalf("enriched=%+v", enriched[0])
	}
}
