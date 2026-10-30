package publisher

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"windows/internal/ipfs"
	"windows/internal/store"
)

func TestPublicSiteFromIndependentNode(t *testing.T) {
	if os.Getenv("PLANET_TEST_ONLINE") != "1" {
		t.Skip("public network integration is opt-in")
	}
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Fatal("Kubo required")
	}
	root := t.TempDir()
	pub := ipfs.New(exe, filepath.Join(root, "publisher"))
	if err := pub.Init(); err != nil {
		t.Fatal(err)
	}
	if err := pub.Start(false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pub.Close() })
	readRepo := filepath.Join(root, "reader")
	reader := ipfs.New(exe, readRepo)
	if err := reader.Init(); err != nil {
		t.Fatal(err)
	}
	if err := reader.Start(false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	const siteID = "11111111-1111-4111-8111-111111111111"
	const postID = "22222222-2222-4222-8222-222222222222"
	key, err := pub.EnsureKey(siteID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	site := store.Site{ID: siteID, Name: "Planet Windows network test", IPNS: key, Created: now}
	post := store.Article{ID: postID, SiteID: siteID, Title: "Synthetic test", Content: "Network test only", Created: now, Modified: now}
	result, err := Publish(pub, root, site, []store.Article{post})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("IPNS=%s CID=%s", result.IPNS, result.CID)
	command := func(args ...string) string {
		cmd := exec.Command(exe, args...)
		cmd.Env = append(os.Environ(), "IPFS_PATH="+readRepo)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %s (%v)", args[0], out, err)
		}
		return strings.TrimSpace(string(out))
	}
	if got := command("name", "resolve", "--nocache", "/ipns/"+result.IPNS); got != "/ipfs/"+result.CID {
		t.Fatalf("resolved=%q", got)
	}
	if page := command("cat", "/ipfs/"+result.CID+"/"+postID+"/index.html"); !strings.Contains(page, "Network test only") {
		t.Fatalf("missing article: %s", page)
	}
	if metadata := command("cat", "/ipfs/"+result.CID+"/planet.json"); !strings.Contains(metadata, result.IPNS) {
		t.Fatalf("missing IPNS: %s", metadata)
	}
}
