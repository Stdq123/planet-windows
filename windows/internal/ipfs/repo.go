package ipfs

import (
	"errors"
	"time"
)

func (c *Client) requireOwnedDaemonLocked() error {
	if c.process == nil || c.ownedAPI == "" {
		return errors.New("IPFS 节点未运行")
	}
	select {
	case <-c.exited:
		c.clearDaemonState()
		return errors.New("IPFS 节点已停止")
	default:
		return nil
	}
}

// RepoSize reports repository usage without starting a daemon or blocking behind publish/resolve.
func (c *Client) RepoSize() (int64, error) {
	snap := c.snapshot()
	if !snap.running {
		return 0, errors.New("IPFS 节点未运行")
	}
	var result struct {
		RepoSize int64 `json:"RepoSize"`
	}
	if err := postRPCAPI(snap.api, 15*time.Second, "/api/v0/repo/stat?size-only=true", 64*1024, &result); err != nil {
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
