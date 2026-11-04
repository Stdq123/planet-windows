package ipfs

import (
	"encoding/json"
	"errors"
	"time"
)

func (c *Client) requireOwnedDaemonLocked() error {
	if c.process == nil || c.ownedAPI == "" {
		return errors.New("IPFS 节点未运行")
	}
	select {
	case <-c.exited:
		c.process = nil
		c.ownedAPI = ""
		c.ownedGateway = ""
		return errors.New("IPFS 节点已停止")
	default:
		return nil
	}
}

// RepoSize reports repository usage without starting a daemon.
func (c *Client) RepoSize() (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.requireOwnedDaemonLocked(); err != nil {
		return 0, err
	}
	out, err := c.runTimeout(15*time.Second, "--enc=json", "repo", "stat", "--size-only")
	if err != nil {
		return 0, err
	}
	var result struct {
		RepoSize int64 `json:"RepoSize"`
	}
	if err = json.Unmarshal([]byte(out), &result); err != nil {
		return 0, err
	}
	return result.RepoSize, nil
}

// GarbageCollect removes unpinned blocks from the owned daemon's repository.
func (c *Client) GarbageCollect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.requireOwnedDaemonLocked(); err != nil {
		return err
	}
	_, err := c.runTimeout(5*time.Minute, "repo", "gc", "--silent")
	return err
}
