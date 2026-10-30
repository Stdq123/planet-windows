package publisher

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"windows/internal/attachments"
	"windows/internal/store"
)

type fakeNode struct {
	added       bool
	published   bool
	failPublish bool
}

type attachmentNode struct {
	fakeNode
	expected string
}

func (n *attachmentNode) Add(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, n.expected, "photo.png"))
	if err != nil || string(b) != "image bytes" {
		return "", errors.New("attachment absent")
	}
	return n.fakeNode.Add(dir)
}
func (n *attachmentNode) PublishOnline(key, cid string) (string, error) { return "test-name", nil }
func TestPublishIncludesWindowsAttachment(t *testing.T) {
	root := t.TempDir()
	library, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := library.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	post, err := library.SaveArticle(site.ID, "", "Title", "Body")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "photo.png")
	if err = os.WriteFile(file, []byte("image bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	post, err = attachments.Add(root, library, site.ID, post.ID, file)
	if err != nil {
		t.Fatal(err)
	}
	node := &attachmentNode{expected: post.ID}
	_, err = PublishWithAssets(node, t.TempDir(), site, []store.Article{post}, "", root)
	if err != nil {
		t.Fatal(err)
	}
}

func (n *fakeNode) LookupKey(name string) (string, error) { return "test-name", nil }

func (n *fakeNode) EnsureKey(name string) (string, error) { return "test-name", nil }
func (n *fakeNode) Add(dir string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		return "", err
	}
	n.added = true
	return "test-cid", nil
}
func (n *fakeNode) PublishOnline(key, cid string) (string, error) {
	if n.failPublish {
		return "", errors.New("network unavailable")
	}
	if key != "SITE-1" || cid != "test-cid" {
		return "", errors.New("wrong key or CID")
	}
	n.published = true
	return "test-name", nil
}

func TestPublishBuildsSiteAndReportsNetworkResult(t *testing.T) {
	node := &fakeNode{}
	result, err := Publish(node, t.TempDir(), store.Site{ID: "SITE-1", Name: "Test"}, []store.Article{{ID: "POST-1", SiteID: "SITE-1", Title: "Hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if !node.added || !node.published || result.CID != "test-cid" || result.IPNS != "test-name" {
		t.Fatalf("result = %#v", result)
	}
}

func TestPublishDoesNotReportSuccessOnNetworkFailure(t *testing.T) {
	node := &fakeNode{failPublish: true}
	result, err := Publish(node, t.TempDir(), store.Site{ID: "SITE-1", Name: "Test"}, nil)
	if err == nil || result.IPNS != "" {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
}

func TestPublishRejectsWrongExistingIdentity(t *testing.T) {
	node := &fakeNode{}
	_, err := Publish(node, t.TempDir(), store.Site{ID: "SITE-1", Name: "Test", IPNS: "other-name"}, nil)
	if err == nil || node.added || node.published {
		t.Fatalf("err=%v, node=%#v", err, node)
	}
}

func TestPublishDirectoryChecksIdentityBeforeAdding(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("published page"), 0600); err != nil {
		t.Fatal(err)
	}
	node := &fakeNode{}
	_, err := PublishDirectory(node, dir, store.Site{ID: "SITE-1", IPNS: "other-name"})
	if err == nil || node.added || node.published {
		t.Fatalf("identity mismatch published: %v %#v", err, node)
	}
	result, err := PublishDirectory(node, dir, store.Site{ID: "SITE-1", IPNS: "test-name"})
	if err != nil || !node.added || !node.published || result.CID != "test-cid" {
		t.Fatalf("result=%#v err=%v node=%#v", result, err, node)
	}
}
