package ipfs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// KuboStatus describes only the daemon started by this application.
type KuboStatus struct {
	Disabled     bool    `json:"disabled"`
	Running      bool    `json:"running"`
	Online       bool    `json:"online"`
	Version      string  `json:"version"`
	Peers        int     `json:"peers"`
	TotalIn      int64   `json:"totalIn"`
	TotalOut     int64   `json:"totalOut"`
	RateIn       float64 `json:"rateIn"`
	RateOut      float64 `json:"rateOut"`
	Gateway      string  `json:"gateway"`
	Reachability string  `json:"reachability,omitempty"`
	RelayPeers   int     `json:"relayPeers,omitempty"`
}

// Status never starts a daemon solely for monitoring and does not block behind
// long-running publish/resolve commands because Kubo's loopback RPC is concurrent.
func (c *Client) Status() (KuboStatus, error) {
	snap := c.snapshot()
	if !snap.running {
		return KuboStatus{}, nil
	}
	status := KuboStatus{Running: true, Online: snap.online, Gateway: snap.gateway}
	var version struct{ Version string }
	if err := postRPCAPI(snap.api, 3*time.Second, "/api/v0/version", 64*1024, &version); err != nil {
		return status, err
	}
	status.Version = version.Version
	if !status.Online {
		return status, nil
	}
	var peers struct{ Peers []struct{ Peer string } }
	if err := postRPCAPI(snap.api, 3*time.Second, "/api/v0/swarm/peers", 4<<20, &peers); err != nil {
		return status, err
	}
	uniquePeers := make(map[string]struct{}, len(peers.Peers))
	for _, peer := range peers.Peers {
		if peer.Peer != "" {
			uniquePeers[peer.Peer] = struct{}{}
		}
	}
	status.Peers = len(uniquePeers)
	var bandwidth struct {
		TotalIn, TotalOut int64
		RateIn, RateOut   float64
	}
	if err := postRPCAPI(snap.api, 3*time.Second, "/api/v0/stats/bw", 64*1024, &bandwidth); err != nil {
		return status, err
	}
	status.TotalIn, status.TotalOut = bandwidth.TotalIn, bandwidth.TotalOut
	status.RateIn, status.RateOut = bandwidth.RateIn, bandwidth.RateOut
	var identity struct {
		Addresses []string `json:"Addresses"`
	}
	if err := postRPCAPI(snap.api, 3*time.Second, "/api/v0/id", 256*1024, &identity); err == nil {
		status.Reachability, status.RelayPeers = inspectReachability(identity.Addresses)
	}
	return status, nil
}

// postRPC queries the owned loopback RPC directly so periodic status polling
// does not spawn repeated ipfs.exe child processes on Windows.
func (c *Client) postRPC(timeout time.Duration, endpoint string, maxBytes int64, target any) error {
	return postRPCAPI(c.ownedAPI, timeout, endpoint, maxBytes, target)
}

func postRPCAPI(api string, timeout time.Duration, endpoint string, maxBytes int64, target any) error {
	return postRPCAPIContext(context.Background(), api, timeout, endpoint, maxBytes, target)
}

func postRPCAPIContext(parent context.Context, api string, timeout time.Duration, endpoint string, maxBytes int64, target any) error {
	if !loopbackAPI.MatchString(api) {
		return errors.New("Kubo API 地址无效")
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	port := strings.TrimPrefix(api, "/ip4/127.0.0.1/tcp/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:"+port+endpoint, nil)
	if err != nil {
		return err
	}
	client := http.Client{
		Transport:     &http.Transport{Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return formatRPCError(endpoint, response.StatusCode, response.Body)
	}
	if target == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxBytes))
		return nil
	}
	b, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return fmt.Errorf("无法读取 Kubo 响应: %w", err)
	}
	if int64(len(b)) > maxBytes {
		return errors.New("Kubo 响应超过大小限制")
	}
	if err := json.Unmarshal(b, target); err != nil {
		return fmt.Errorf("无法读取 Kubo 状态: %w", err)
	}
	return nil
}

func formatRPCError(endpoint string, statusCode int, body io.Reader) error {
	path := endpoint
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	b, _ := io.ReadAll(io.LimitReader(body, 64<<10))
	var rpcErr struct {
		Message string `json:"Message"`
	}
	if json.Unmarshal(b, &rpcErr) == nil && strings.TrimSpace(rpcErr.Message) != "" {
		return fmt.Errorf("Kubo RPC %s 失败 (HTTP %d): %s", path, statusCode, strings.TrimSpace(rpcErr.Message))
	}
	if text := strings.TrimSpace(string(b)); text != "" {
		return fmt.Errorf("Kubo RPC %s 失败 (HTTP %d): %s", path, statusCode, text)
	}
	return fmt.Errorf("Kubo RPC %s 失败: HTTP %d", path, statusCode)
}
