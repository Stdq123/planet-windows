package ipfs

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusDoesNotStartNodeAndReadsOwnedDaemon(t *testing.T) {
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	c := New(exe, filepath.Join(t.TempDir(), "repo"))
	status, err := c.Status()
	if err != nil || status.Running {
		t.Fatalf("unstarted node: %+v %v", status, err)
	}
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	status, err = c.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Running || status.Online || status.Version == "" || status.Peers != 0 {
		t.Fatalf("offline daemon: %+v", status)
	}
	if !strings.HasPrefix(status.Gateway, "http://127.0.0.1:") {
		t.Fatalf("missing local gateway: %+v", status)
	}
	cid, err := c.Add(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	response, err := client.Get(status.Gateway + "/ipfs/" + cid + "/?format=dag-json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("gateway returned %s", response.Status)
	}
}

func TestStatusReadsOnlineDaemon(t *testing.T) {
	if os.Getenv("PLANET_TEST_ONLINE") != "1" {
		t.Skip("online Kubo required")
	}
	exe := os.Getenv("PLANET_TEST_KUBO")
	if exe == "" {
		t.Skip("Kubo required")
	}
	c := New(exe, filepath.Join(t.TempDir(), "repo"))
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	status, err := c.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Running || !status.Online || status.Version == "" || status.Peers < 0 || status.RateIn < 0 || status.RateOut < 0 {
		t.Fatalf("online daemon: %+v", status)
	}
}
