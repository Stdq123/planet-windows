package ipfs

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPostRPCReadsOwnedLoopbackServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.RequestURI() {
		case "/api/v0/version":
			_, _ = io.WriteString(w, `{"Version":"0.38.2"}`)
		case "/api/v0/swarm/peers":
			_, _ = io.WriteString(w, `{"Peers":[{"Peer":"p1"},{"Peer":"p2"},{"Peer":"p1"}]}`)
		case "/api/v0/repo/stat?size-only=true":
			_, _ = io.WriteString(w, `{"RepoSize":4096}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	port := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
	c := &Client{ownedAPI: "/ip4/127.0.0.1/tcp/" + port}

	var version struct{ Version string }
	if err := c.postRPC(3*time.Second, "/api/v0/version", 64*1024, &version); err != nil || version.Version != "0.38.2" {
		t.Fatalf("version RPC: %+v %v", version, err)
	}
	var peers struct{ Peers []struct{ Peer string } }
	if err := c.postRPC(3*time.Second, "/api/v0/swarm/peers", 4<<20, &peers); err != nil || len(peers.Peers) != 3 {
		t.Fatalf("peers RPC: %+v %v", peers, err)
	}
	var stat struct{ RepoSize int64 }
	if err := c.postRPC(3*time.Second, "/api/v0/repo/stat?size-only=true", 64*1024, &stat); err != nil || stat.RepoSize != 4096 {
		t.Fatalf("repo stat RPC: %+v %v", stat, err)
	}

	c.ownedAPI = "/ip4/192.168.1.2/tcp/5001"
	if err := c.postRPC(3*time.Second, "/api/v0/version", 64*1024, &version); err == nil {
		t.Fatal("non-loopback API accepted")
	}
}

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
