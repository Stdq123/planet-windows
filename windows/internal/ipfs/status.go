package ipfs

import (
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
	Disabled bool    `json:"disabled"`
	Running  bool    `json:"running"`
	Online   bool    `json:"online"`
	Version  string  `json:"version"`
	Peers    int     `json:"peers"`
	TotalIn  int64   `json:"totalIn"`
	TotalOut int64   `json:"totalOut"`
	RateIn   float64 `json:"rateIn"`
	RateOut  float64 `json:"rateOut"`
	Gateway  string  `json:"gateway"`
}

// Status never starts a daemon solely for monitoring.
func (c *Client) Status() (KuboStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.process == nil {
		return KuboStatus{}, nil
	}
	select {
	case <-c.exited:
		c.process = nil
		c.ownedAPI = ""
		return KuboStatus{}, nil
	default:
	}
	status := KuboStatus{Running: true, Online: !c.offline, Gateway: c.ownedGateway}
	var version struct{ Version string }
	if err := c.postRPC(3*time.Second, "/api/v0/version", 64*1024, &version); err != nil {
		return status, err
	}
	status.Version = version.Version
	if !status.Online {
		return status, nil
	}
	var peers struct{ Peers []struct{ Peer string } }
	if err := c.postRPC(3*time.Second, "/api/v0/swarm/peers", 4<<20, &peers); err != nil {
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
	if err := c.postRPC(3*time.Second, "/api/v0/stats/bw", 64*1024, &bandwidth); err != nil {
		return status, err
	}
	status.TotalIn, status.TotalOut = bandwidth.TotalIn, bandwidth.TotalOut
	status.RateIn, status.RateOut = bandwidth.RateIn, bandwidth.RateOut
	return status, nil
}

// postRPC queries the owned loopback RPC directly so periodic status polling
// does not spawn repeated ipfs.exe child processes on Windows.
func (c *Client) postRPC(timeout time.Duration, endpoint string, maxBytes int64, target any) error {
	if !loopbackAPI.MatchString(c.ownedAPI) {
		return errors.New("Kubo API 地址无效")
	}
	port := strings.TrimPrefix(c.ownedAPI, "/ip4/127.0.0.1/tcp/")
	client := http.Client{
		Timeout:       timeout,
		Transport:     &http.Transport{Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	response, err := client.Post("http://127.0.0.1:"+port+endpoint, "application/json", nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Kubo RPC %s 失败: HTTP %d", endpoint, response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxBytes)).Decode(target); err != nil {
		return fmt.Errorf("无法读取 Kubo 状态: %w", err)
	}
	return nil
}
