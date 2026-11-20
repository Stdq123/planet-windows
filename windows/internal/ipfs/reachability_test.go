package ipfs

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestInspectReachabilityClassifiesDirectRelayAndLocal(t *testing.T) {
	t.Run("local and virtual TUN addresses only", func(t *testing.T) {
		got, relays := inspectReachability([]string{
			"/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWLocal",
			"/ip4/192.168.1.25/udp/4001/quic-v1/p2p/12D3KooWLocal",
			"/ip4/198.18.0.1/tcp/4001/p2p/12D3KooWLocal",
			"/ip4/100.64.0.12/tcp/4001/p2p/12D3KooWLocal",
			"/ip6/::1/tcp/4001/p2p/12D3KooWLocal",
			"/ip6/fe80::1/tcp/4001/p2p/12D3KooWLocal",
		})
		if got != ReachabilityLocal || relays != 0 {
			t.Fatalf("got (%q, %d), want (%q, 0)", got, relays, ReachabilityLocal)
		}
	})

	t.Run("relay addresses deduplicated by relay peer", func(t *testing.T) {
		got, relays := inspectReachability([]string{
			"/ip4/127.0.0.1/tcp/53287/p2p/12D3KooWLocal",
			"/ip4/198.18.0.1/tcp/53287/p2p/12D3KooWLocal",
			"/ip4/167.99.245.70/tcp/4001/p2p/12D3KooWRelayOne/p2p-circuit/p2p/12D3KooWLocal",
			"/ip4/167.99.245.70/udp/4001/quic-v1/p2p/12D3KooWRelayOne/p2p-circuit/p2p/12D3KooWLocal",
			"/ip4/192.3.138.190/tcp/4001/p2p/12D3KooWRelayTwo/p2p-circuit/p2p/12D3KooWLocal",
		})
		if got != ReachabilityRelay || relays != 2 {
			t.Fatalf("got (%q, %d), want (%q, 2)", got, relays, ReachabilityRelay)
		}
	})

	t.Run("direct public IPv4 or IPv6 address", func(t *testing.T) {
		got, relays := inspectReachability([]string{
			"/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWLocal",
			"/ip4/1.1.1.1/tcp/4001/p2p/12D3KooWLocal",
			"/ip4/167.99.245.70/tcp/4001/p2p/12D3KooWRelayOne/p2p-circuit/p2p/12D3KooWLocal",
		})
		if got != ReachabilityDirect || relays != 1 {
			t.Fatalf("got (%q, %d), want (%q, 1)", got, relays, ReachabilityDirect)
		}
	})
}

func TestProvideContextCallsRecursiveRoutingProvide(t *testing.T) {
	var calledArg, calledRecursive string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v0/routing/provide" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		calledArg = r.URL.Query().Get("arg")
		calledRecursive = r.URL.Query().Get("recursive")
		_, _ = io.WriteString(w, `{"ID":"peer","Type":0}`+"\n")
	}))
	defer server.Close()

	port := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
	c := &Client{}
	c.setDaemonState(&exec.Cmd{}, make(chan error, 1), false, "/ip4/127.0.0.1/tcp/"+port, "http://127.0.0.1:8080")

	// Must not block on c.mu while another command is active.
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ProvideContext(context.Background(), "QmTestCid123"); err != nil {
		t.Fatalf("ProvideContext failed: %v", err)
	}
	if calledArg != "QmTestCid123" || calledRecursive != "true" {
		t.Fatalf("unexpected query: arg=%q recursive=%q", calledArg, calledRecursive)
	}
	if err := c.ProvideContext(context.Background(), "../bad"); err == nil {
		t.Fatal("invalid CID accepted")
	}

	offline := &Client{}
	offline.setDaemonState(&exec.Cmd{}, make(chan error, 1), true, "/ip4/127.0.0.1/tcp/"+port, "http://127.0.0.1:8080")
	if err := offline.ProvideContext(context.Background(), "QmTestCid123"); err == nil {
		t.Fatal("offline daemon accepted ProvideContext")
	}
}

func TestWaitForPublicReachabilityTransitionsFromLocalToRelay(t *testing.T) {
	var idCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v0/version":
			_, _ = io.WriteString(w, `{"Version":"0.38.2"}`)
		case "/api/v0/swarm/peers":
			_, _ = io.WriteString(w, `{"Peers":[{"Peer":"p1"},{"Peer":"p2"}]}`)
		case "/api/v0/stats/bw":
			_, _ = io.WriteString(w, `{"TotalIn":100,"TotalOut":50,"RateIn":10,"RateOut":5}`)
		case "/api/v0/id":
			if idCalls.Add(1) == 1 {
				_, _ = io.WriteString(w, `{"Addresses":["/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWLocal","/ip4/198.18.0.1/tcp/4001/p2p/12D3KooWLocal"]}`)
			} else {
				_, _ = io.WriteString(w, `{"Addresses":["/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWLocal","/ip4/167.99.245.70/tcp/4001/p2p/12D3KooWRelayOne/p2p-circuit/p2p/12D3KooWLocal","/ip4/192.3.138.190/tcp/4001/p2p/12D3KooWRelayTwo/p2p-circuit/p2p/12D3KooWLocal"]}`)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	port := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
	c := &Client{}
	c.setDaemonState(&exec.Cmd{}, make(chan error, 1), false, "/ip4/127.0.0.1/tcp/"+port, "http://127.0.0.1:8080")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	status, ready := c.WaitForPublicReachability(ctx, 15*time.Millisecond)
	if !ready {
		t.Fatalf("expected relay reachability, got %+v", status)
	}
	if status.Reachability != ReachabilityRelay || status.RelayPeers != 2 {
		t.Fatalf("unexpected reachability: %+v", status)
	}
	if idCalls.Load() < 2 {
		t.Fatalf("expected at least 2 polls, got %d", idCalls.Load())
	}
}
