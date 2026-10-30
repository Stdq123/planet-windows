package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"windows/internal/attachments"
	"windows/internal/ipfs"
	"windows/internal/migration"
	"windows/internal/store"
	"windows/internal/templateinstall"
)

func TestLibraryPublishesAndPersistsIdentity(t *testing.T) {
	if os.Getenv("PLANET_TEST_ONLINE") != "1" {
		t.Skip("public network integration is opt-in")
	}
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Fatal("Kubo required")
	}
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, root)
	mygo.App.SetPath(mygo.PathResources, filepath.Dir(exe))
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, ""); mygo.App.SetPath(mygo.PathResources, "") })
	library := &Library{}
	t.Cleanup(library.closeNode)
	site, err := library.CreateSite("Planet Windows integration test", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = library.SaveArticle(site.ID, "", "Synthetic post", "Public test content"); err != nil {
		t.Fatal(err)
	}
	result, err := library.PublishSite(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	sites, err := reopened.ListSites()
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0].IPNS != result.IPNS || sites[0].LastPublishedCID != result.CID || sites[0].LastPublished == nil {
		t.Fatalf("result=%#v sites=%#v", result, sites)
	}
	t.Logf("IPNS=%s CID=%s", result.IPNS, result.CID)
}

func TestLibraryPublishesImportedSiteWithAttachments(t *testing.T) {
	if os.Getenv("PLANET_TEST_ONLINE") != "1" {
		t.Skip("public network integration is opt-in")
	}
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Fatal("Kubo required")
	}
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(root, "app"))
	mygo.App.SetPath(mygo.PathResources, filepath.Dir(exe))
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, ""); mygo.App.SetPath(mygo.PathResources, "") })
	const id = "11111111-1111-4111-8111-111111111111"
	const post = "22222222-2222-4222-8222-222222222222"
	source := ipfs.New(exe, filepath.Join(root, "source"))
	if err := source.Init(); err != nil {
		t.Fatal(err)
	}
	name, err := source.EnsureKey(id)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "synthetic.planet")
	if err = os.MkdirAll(filepath.Join(input, post), 0700); err != nil {
		t.Fatal(err)
	}
	put := func(path, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(input, path), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	put("index.html", "Synthetic imported site")
	put(filepath.Join(post, "index.html"), "Imported article with attachment")
	put(filepath.Join(post, "photo.txt"), "synthetic attachment")
	public := fmt.Sprintf(`{"id":%q,"link":%q,"title":"Imported post","content":"Synthetic content","created":10,"attachments":["photo.txt"]}`, post, "/"+post+"/")
	put(filepath.Join(post, "article.json"), public)
	metadata := fmt.Sprintf(`{"id":%q,"name":"Imported","about":"","ipns":%q,"created":10,"updated":20,"templateName":"Plain","filebaseAPIToken":"SECRET_TEST_TOKEN","articles":[%s]}`, id, name, public)
	put("planet.json", metadata)
	if err = source.ExportKey(id, filepath.Join(input, "planet.key")); err != nil {
		t.Fatal(err)
	}
	library := &Library{}
	t.Cleanup(library.closeNode)
	s, err := library.open()
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = library.ensureNode(false)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := migration.New(filepath.Join(root, "app"), exe, s, library.node).Import(input)
	if err != nil {
		t.Fatal(err)
	}
	templateRoot := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if templateRoot == "" {
		t.Fatal("Plain template required")
	}
	posts, err := s.ListArticles(imported.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = templateinstall.Install(filepath.Join(root, "app"), templateRoot, imported, posts, s); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveArticle(imported.ID, post, "Updated imported post", "Updated synthetic content"); err != nil {
		t.Fatal(err)
	}
	result, err := library.PublishSite(imported.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.IPNS != name {
		t.Fatalf("identity changed: %s", result.IPNS)
	}
	readerRepo := filepath.Join(root, "reader")
	reader := ipfs.New(exe, readerRepo)
	if err = reader.Init(); err != nil {
		t.Fatal(err)
	}
	if err = reader.Start(false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	run := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, args...)
		cmd.Env = append(os.Environ(), "IPFS_PATH="+readerRepo)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s (%v)", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	if got := run("name", "resolve", "--nocache", "/ipns/"+name); got != "/ipfs/"+result.CID {
		t.Fatalf("resolved %s", got)
	}
	if got := run("cat", "/ipfs/"+result.CID+"/"+post+"/photo.txt"); got != "synthetic attachment" {
		t.Fatalf("attachment=%q", got)
	}
	if got := run("cat", "/ipfs/"+result.CID+"/"+post+"/index.html"); !strings.Contains(got, "Updated synthetic content") {
		t.Fatalf("stale article page: %s", got)
	}
	if got := run("cat", "/ipfs/"+result.CID+"/planet.json"); strings.Contains(got, "SECRET_TEST_TOKEN") || strings.Contains(got, "filebaseAPIToken") {
		t.Fatalf("private metadata published: %s", got)
	}
	t.Logf("Synthetic imported site: IPNS=%s CID=%s", result.IPNS, result.CID)
}

func TestLibraryPublishesWindowsAttachment(t *testing.T) {
	if os.Getenv("PLANET_TEST_ONLINE") != "1" {
		t.Skip("public network integration is opt-in")
	}
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Fatal("Kubo required")
	}
	root := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(root, "app"))
	mygo.App.SetPath(mygo.PathResources, filepath.Dir(exe))
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, ""); mygo.App.SetPath(mygo.PathResources, "") })
	library := &Library{}
	t.Cleanup(library.closeNode)
	site, err := library.CreateSite("Synthetic attachments", "")
	if err != nil {
		t.Fatal(err)
	}
	post, err := library.SaveArticle(site.ID, "", "Post", "![photo](photo.png)")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "photo.png")
	if err = os.WriteFile(file, []byte("synthetic image bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := library.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = attachments.Add(filepath.Join(root, "app"), s, site.ID, post.ID, file); err != nil {
		t.Fatal(err)
	}
	result, err := library.PublishSite(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "reader")
	reader := ipfs.New(exe, repo)
	if err = reader.Init(); err != nil {
		t.Fatal(err)
	}
	if err = reader.Start(false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "cat", "/ipfs/"+result.CID+"/"+post.ID+"/photo.png")
	cmd.Env = append(os.Environ(), "IPFS_PATH="+repo)
	out, err := cmd.CombinedOutput()
	if err != nil || string(out) != "synthetic image bytes" {
		t.Fatalf("attachment=%q err=%v", out, err)
	}
}
