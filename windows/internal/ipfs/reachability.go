package ipfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const (
	ReachabilityDirect = "direct"
	ReachabilityRelay  = "relay"
	ReachabilityLocal  = "local"
)

var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),   // RFC 6598 CGNAT
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),   // RFC 2544 / virtual TUN (e.g. Clash Fake-IP)
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("2001:db8::/32"),   // IPv6 documentation
}

func isPublicRoutableAddr(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

func extractRelayPeer(multiaddr string) string {
	idx := strings.Index(multiaddr, "/p2p-circuit")
	if idx < 0 {
		return ""
	}
	prefix := multiaddr[:idx]
	parts := strings.Split(prefix, "/")
	for i := len(parts) - 2; i >= 0; i-- {
		if parts[i] == "p2p" && parts[i+1] != "" {
			return parts[i+1]
		}
	}
	return ""
}

func extractDirectIP(multiaddr string) (netip.Addr, bool) {
	parts := strings.Split(multiaddr, "/")
	if len(parts) < 3 {
		return netip.Addr{}, false
	}
	if parts[1] != "ip4" && parts[1] != "ip6" {
		return netip.Addr{}, false
	}
	addr, err := netip.ParseAddr(parts[2])
	if err != nil {
		return netip.Addr{}, false
	}
	return addr, true
}

// inspectReachability classifies the node's advertised multiaddrs from /api/v0/id.
func inspectReachability(addresses []string) (string, int) {
	hasDirect := false
	hasRelay := false
	relayPeers := make(map[string]struct{})
	for _, addr := range addresses {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		if strings.Contains(addr, "/p2p-circuit") {
			hasRelay = true
			if peer := extractRelayPeer(addr); peer != "" {
				relayPeers[peer] = struct{}{}
			}
			continue
		}
		if ip, ok := extractDirectIP(addr); ok && isPublicRoutableAddr(ip) {
			hasDirect = true
		}
	}
	relays := len(relayPeers)
	if hasRelay && relays == 0 {
		relays = 1
	}
	if hasDirect {
		return ReachabilityDirect, relays
	}
	if hasRelay {
		return ReachabilityRelay, relays
	}
	return ReachabilityLocal, 0
}

// ProvideContext recursively announces a CID via Kubo's concurrent loopback RPC
// so public routing (delegated routers and DHT) receives the node's current
// direct or /p2p-circuit relay addresses without holding c.mu.
func (c *Client) ProvideContext(ctx context.Context, cid string) error {
	if !safeName.MatchString(cid) {
		return errors.New("CID 无效")
	}
	snap := c.snapshot()
	if !snap.running || !snap.online {
		return errors.New("Kubo 在线节点未运行")
	}
	if !loopbackAPI.MatchString(snap.api) {
		return errors.New("Kubo API 地址无效")
	}
	port := strings.TrimPrefix(snap.api, "/ip4/127.0.0.1/tcp/")
	endpoint := "http://127.0.0.1:" + port + "/api/v0/routing/provide?" + url.Values{
		"arg":       []string{cid},
		"recursive": []string{"true"},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	client := http.Client{
		Transport:     &http.Transport{Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Kubo RPC /api/v0/routing/provide 失败: HTTP %d", resp.StatusCode)
	}
	return nil
}

// WaitForPublicReachability polls Status until the online daemon advertises a
// public direct address or a /p2p-circuit relay address, stops running, or ctx ends.
func (c *Client) WaitForPublicReachability(ctx context.Context, pollInterval time.Duration) (KuboStatus, bool) {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	check := func() (KuboStatus, bool, bool) {
		status, err := c.Status()
		if err != nil {
			return status, false, false
		}
		if !status.Running || !status.Online {
			return status, false, true
		}
		if status.Reachability == ReachabilityDirect || status.Reachability == ReachabilityRelay {
			return status, true, true
		}
		return status, false, false
	}
	last, ready, done := check()
	if done {
		return last, ready
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return last, false
		case <-ticker.C:
			last, ready, done = check()
			if done {
				return last, ready
			}
		}
	}
}
