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
	read := func(target any, args ...string) error {
		out, err := c.runTimeout(3*time.Second, append([]string{"--enc=json"}, args...)...)
		if err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(out), target); err != nil {
			return fmt.Errorf("无法读取 Kubo 状态: %w", err)
		}
		return nil
	}
	var version struct{ Version string }
	if err := read(&version, "version"); err != nil {
		return status, err
	}
	status.Version = version.Version
	if !status.Online {
		return status, nil
	}
	var peers struct{ Peers []struct{ Peer string } }
	if err := read(&peers, "swarm", "peers"); err != nil {
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
	if err := c.readBandwidth(&bandwidth); err != nil {
		return status, err
	}
	status.TotalIn, status.TotalOut = bandwidth.TotalIn, bandwidth.TotalOut
	status.RateIn, status.RateOut = bandwidth.RateIn, bandwidth.RateOut
	return status, nil
}

// The CLI formats stats bw as human-readable text even with --enc=json; the
// owned loopback RPC returns numeric values suitable for the status panel.
func (c *Client) readBandwidth(target any) error {
	if !loopbackAPI.MatchString(c.ownedAPI) {
		return errors.New("Kubo API 地址无效")
	}
	port := strings.TrimPrefix(c.ownedAPI, "/ip4/127.0.0.1/tcp/")
	client := http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	response, err := client.Post("http://127.0.0.1:"+port+"/api/v0/stats/bw", "application/json", nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("读取 Kubo 流量失败: HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(target)
}
