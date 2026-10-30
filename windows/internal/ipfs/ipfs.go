// Package ipfs provides a Kubo command boundary using a dedicated repository.
package ipfs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var safeName = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

type Client struct {
	mu         sync.Mutex
	executable string
	repo       string
	process    *exec.Cmd
	exited     chan error
}

func New(executable, repo string) *Client { return &Client{executable: executable, repo: repo} }

func (c *Client) Alive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.process == nil {
		return false
	}
	select {
	case <-c.exited:
		c.process = nil
		return false
	default:
		return true
	}
}

func (c *Client) run(args ...string) (string, error) {
	return c.runTimeout(3*time.Minute, args...)
}

func (c *Client) runTimeout(timeout time.Duration, args ...string) (string, error) {
	return c.runContext(context.Background(), timeout, args...)
}

func (c *Client) runContext(parent context.Context, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.executable, args...)
	c.configure(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Kubo: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *Client) configure(cmd *exec.Cmd) {
	for _, env := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(env), "IPFS_PATH=") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env, "IPFS_PATH="+c.repo)
	hideWindow(cmd)
}

func (c *Client) Init() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := os.Stat(filepath.Join(c.repo, "config")); errors.Is(err, os.ErrNotExist) {
		if _, err := c.run("init", "--empty-repo"); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if _, err := c.run("config", "Addresses.API", "/ip4/127.0.0.1/tcp/0"); err != nil {
		return err
	}
	if _, err := c.run("config", "Addresses.Gateway", "/ip4/127.0.0.1/tcp/0"); err != nil {
		return err
	}
	_, err := c.run("config", "--json", "Addresses.Swarm", `["/ip4/0.0.0.0/tcp/0"]`)
	return err
}

// Start owns the daemon process it creates. Offline mode is intended for tests.
func (c *Client) Start(offline bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.process != nil {
		return errors.New("Kubo is already running")
	}
	args := []string{"daemon"}
	if offline {
		args = append(args, "--offline")
	}
	cmd := exec.Command(c.executable, args...)
	c.configure(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	ready := make(chan struct{}, 1)
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "Daemon is ready") {
				select {
				case ready <- struct{}{}:
				default:
				}
			}
		}
	}()
	go func() { exited <- cmd.Wait() }()
	select {
	case <-ready:
		c.process, c.exited = cmd, exited
		return nil
	case err := <-exited:
		return fmt.Errorf("Kubo daemon exited before startup: %v", err)
	case <-time.After(45 * time.Second):
		_ = cmd.Process.Kill()
		<-exited
		return errors.New("Kubo daemon startup timed out")
	}
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.process == nil {
		return nil
	}
	select {
	case <-c.exited:
		c.process = nil
		return nil
	default:
	}
	_, err := c.runTimeout(5*time.Second, "shutdown")
	select {
	case <-c.exited:
	case <-time.After(10 * time.Second):
		_ = c.process.Process.Kill()
		<-c.exited
		if err == nil {
			err = errors.New("Kubo did not shut down cleanly")
		}
	}
	c.process = nil
	return err
}

func (c *Client) EnsureKey(name string) (string, error) {
	if !safeName.MatchString(name) || name == "self" {
		return "", errors.New("invalid key name")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key, err := c.lookupKey(context.Background(), name)
	if err != nil || key != "" {
		return key, err
	}
	return c.run("key", "gen", "--type=ed25519", name)
}

func (c *Client) LookupKey(name string) (string, error) {
	return c.LookupKeyContext(context.Background(), name)
}

func (c *Client) LookupKeyContext(ctx context.Context, name string) (string, error) {
	if !safeName.MatchString(name) || name == "self" {
		return "", errors.New("invalid key name")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lookupKey(ctx, name)
}

// ExportKey writes a cleartext private key to a new path chosen by the caller.
func (c *Client) ExportKey(name, path string) error {
	if !safeName.MatchString(name) || name == "self" {
		return errors.New("invalid key name")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := os.Lstat(path); err == nil {
		return errors.New("key export destination exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err := c.run("key", "export", name, "-o", path)
	return err
}

func (c *Client) ImportKey(name, path string) (string, error) {
	if !safeName.MatchString(name) || name == "self" {
		return "", errors.New("invalid key name")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.run("key", "import", name, path)
}

func (c *Client) RemoveKey(name string) error {
	if !safeName.MatchString(name) || name == "self" {
		return errors.New("invalid key name")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.run("key", "rm", name)
	return err
}

func (c *Client) KeyMatches(name, identity string) (bool, error) {
	if !safeName.MatchString(name) || name == "self" || !safeName.MatchString(identity) {
		return false, errors.New("invalid key name or identity")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, base := range []string{"base36", "b58mh", "base32"} {
		list, err := c.run("key", "list", "-l", "--ipns-base="+base)
		if err != nil {
			return false, err
		}
		for _, line := range strings.Split(list, "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[1] == name && fields[0] == identity {
				return true, nil
			}
		}
	}
	return false, nil
}

func (c *Client) lookupKey(ctx context.Context, name string) (string, error) {
	list, err := c.runContext(ctx, 15*time.Second, "key", "list", "-l")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(list, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			return fields[0], nil
		}
	}
	return "", nil
}

func (c *Client) Add(dir string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.run("add", "-Qr", dir)
}

// PublishOffline creates a local IPNS record; it does not publish to the network.
func (c *Client) PublishOffline(key, cid string) (string, error) {
	if !safeName.MatchString(key) || !safeName.MatchString(cid) {
		return "", errors.New("invalid key or CID")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.run("--offline", "name", "publish", "--allow-offline", "--quieter", "--key="+key, "/ipfs/"+cid)
}

// PublishOnline asks the running daemon to publish an IPNS record to the network.
func (c *Client) PublishOnline(key, cid string) (string, error) {
	return c.PublishOnlineContext(context.Background(), key, cid)
}

func (c *Client) PublishOnlineContext(ctx context.Context, key, cid string) (string, error) {
	if !safeName.MatchString(key) || !safeName.MatchString(cid) {
		return "", errors.New("invalid key or CID")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.process == nil {
		return "", errors.New("Kubo daemon is not running")
	}
	select {
	case <-c.exited:
		c.process = nil
		return "", errors.New("Kubo daemon is not running")
	default:
	}
	return c.runContext(ctx, 3*time.Minute, "name", "publish", "--quieter", "--key="+key, "--lifetime=7200h", "/ipfs/"+cid)
}

func (c *Client) ResolveOffline(name string) (string, error) {
	if !safeName.MatchString(name) {
		return "", errors.New("invalid IPNS name")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.run("--offline", "name", "resolve", "/ipns/"+name)
}
