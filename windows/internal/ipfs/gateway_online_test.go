package ipfs

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This publishes only synthetic content with a temporary key and repository.
// The gateway runs on a separate machine, unlike the local reader test.
func TestEthSucksPublicGateway(t *testing.T) {
	if os.Getenv("PLANET_TEST_ETH_GATEWAY") != "1" {
		t.Skip("public gateway integration is opt-in")
	}
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Fatal("PLANET_TEST_KUBO is required")
	}
	node := New(exe, filepath.Join(t.TempDir(), "publisher"))
	if err := node.Init(); err != nil {
		t.Fatal(err)
	}
	if err := node.Start(false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = node.Close() })
	if _, err := node.EnsureKey("gateway-test"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	// A fresh CID avoids a previous gateway cache masking a retrieval failure.
	content := fmt.Sprintf("Planet Windows eth.sucks synthetic gateway test %d", time.Now().UnixNano())
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cid, err := node.Add(dir)
	if err != nil {
		t.Fatal(err)
	}
	name, err := node.PublishOnline("gateway-test", cid)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("IPNS=%s CID=%s", name, cid)
	client := &http.Client{Timeout: 30 * time.Second}
	deadline := time.Now().Add(90 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		resp, err := client.Get("https://" + name + ".eth.sucks/")
		if err != nil {
			last = err.Error()
		} else {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			if readErr == nil && resp.StatusCode == http.StatusOK && strings.Contains(string(body), content) {
				return
			}
			last = resp.Status + ": " + string(body)
		}
		time.Sleep(5 * time.Second)
	}
	t.Fatalf("gateway did not return synthetic content: %s", last)
}
