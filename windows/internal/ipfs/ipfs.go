// Package ipfs provides a Kubo command boundary using a dedicated repository.
package ipfs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"windows/internal/sitecleanup"
)

var safeName = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
var loopbackAPI = regexp.MustCompile(`^/ip4/127\.0\.0\.1/tcp/[0-9]{1,5}$`)

type Client struct {
	mu         sync.Mutex
	executable string
	repo       string
	process    *exec.Cmd
	exited     chan error
	ownedAPI   string
	offline    bool
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
		c.ownedAPI = ""
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
	// Never let the Kubo CLI infer a daemon from a mutable repository api file.
	temporary, err := c.ensureCommandDaemon()
	if err != nil {
		return "", err
	}
	if temporary {
		defer func() { _ = c.closeLocked() }()
	}
	if len(args) > 0 && strings.HasPrefix(args[0], "--api=") && args[0] != "--api="+c.ownedAPI {
		return "", errors.New("Kubo API 与本程序启动的服务不一致")
	}
	if len(args) == 0 || !strings.HasPrefix(args[0], "--api=") {
		args = append([]string{"--api=" + c.ownedAPI}, args...)
	}
	cmd := exec.CommandContext(ctx, c.executable, args...)
	c.configure(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Kubo: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *Client) ensureCommandDaemon() (bool, error) {
	if present, err := sitecleanup.KuboRepoPresent(c.repo); err != nil || !present {
		if err != nil {
			return false, err
		}
		return false, errors.New("Kubo 仓库不存在")
	}
	if c.process != nil {
		if c.ownedAPI == "" {
			return false, errors.New("Kubo API 身份未经核验")
		}
		return false, nil
	}
	if err := c.validateKeyAPI(); err != nil {
		return false, err
	}
	if err := sitecleanup.CheckOfflineKuboRepo(c.repo); err != nil {
		return false, err
	}
	if err := c.startLocked(true); err != nil {
		return false, err
	}
	return true, nil
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
	if c.process != nil {
		return errors.New("Kubo 正在运行，不能修改仓库配置")
	}
	repoInfo, repoErr := os.Lstat(c.repo)
	if repoErr == nil {
		if !repoInfo.IsDir() || repoInfo.Mode()&os.ModeSymlink != 0 {
			return errors.New("Kubo 仓库目录无效")
		}
	} else if !errors.Is(repoErr, os.ErrNotExist) {
		return repoErr
	}
	if err := c.validateKeyAPI(); err != nil {
		return err
	}
	path := filepath.Join(c.repo, "config")
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, c.executable, "--offline", "init", "--empty-repo")
		c.configure(cmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("Kubo 初始化失败：%s: %w", strings.TrimSpace(string(out)), err)
		}
	} else if err != nil {
		return err
	}
	if present, err := sitecleanup.KuboRepoPresent(c.repo); err != nil || !present {
		if err != nil {
			return err
		}
		return errors.New("Kubo 仓库不存在")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return errors.New("Kubo 配置无效")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	actual, err := f.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		f.Close()
		return errors.New("Kubo 配置已改变")
	}
	b, err := io.ReadAll(io.LimitReader(f, (16<<20)+1))
	closeErr := f.Close()
	if err != nil || closeErr != nil || len(b) > 16<<20 {
		return errors.New("Kubo 配置读取失败")
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(b, &config) != nil || config == nil {
		return errors.New("Kubo 配置无效")
	}
	var addresses map[string]json.RawMessage
	if len(config["Addresses"]) > 0 && json.Unmarshal(config["Addresses"], &addresses) != nil {
		return errors.New("Kubo 地址配置无效")
	}
	if addresses == nil {
		addresses = map[string]json.RawMessage{}
	}
	addresses["API"] = json.RawMessage(`"/ip4/127.0.0.1/tcp/0"`)
	addresses["Gateway"] = json.RawMessage(`"/ip4/127.0.0.1/tcp/0"`)
	addresses["Swarm"] = json.RawMessage(`["/ip4/0.0.0.0/tcp/0"]`)
	config["Addresses"], err = json.Marshal(addresses)
	if err != nil {
		return err
	}
	var dns map[string]json.RawMessage
	if len(config["DNS"]) > 0 && json.Unmarshal(config["DNS"], &dns) != nil {
		return errors.New("Kubo DNS 配置无效")
	}
	if dns == nil {
		dns = map[string]json.RawMessage{}
	}
	var resolvers map[string]json.RawMessage
	if len(dns["Resolvers"]) > 0 && json.Unmarshal(dns["Resolvers"], &resolvers) != nil {
		return errors.New("Kubo DNS 解析配置无效")
	}
	if resolvers == nil {
		resolvers = map[string]json.RawMessage{}
	}
	if _, exists := resolvers["eth."]; !exists {
		resolvers["eth."] = json.RawMessage(`"https://dns.eth.limo/dns-query"`)
	}
	dns["Resolvers"], err = json.Marshal(resolvers)
	if err != nil {
		return err
	}
	config["DNS"], err = json.Marshal(dns)
	if err != nil {
		return err
	}
	b, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(c.repo, ".config-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if e := tmp.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}

// Start owns the daemon process it creates. Offline mode is intended for tests.
func (c *Client) Start(offline bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.startLocked(offline)
}

func (c *Client) startLocked(offline bool) error {
	if c.process != nil {
		return errors.New("Kubo is already running")
	}
	if err := c.validateKeyAPI(); err != nil {
		return err
	}
	if present, err := sitecleanup.KuboRepoPresent(c.repo); err != nil || !present {
		if err != nil {
			return err
		}
		return errors.New("Kubo 仓库不存在")
	}
	if err := sitecleanup.CheckOfflineKuboRepo(c.repo); err != nil {
		return err
	}
	c.ownedAPI = ""
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
	ready := make(chan string, 1)
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		announcedAPI := ""
		for scanner.Scan() {
			line := scanner.Text()
			if index := strings.Index(line, "RPC API server listening on "); index >= 0 {
				announcedAPI = strings.TrimSpace(line[index+len("RPC API server listening on "):])
			}
			if strings.Contains(line, "Daemon is ready") {
				select {
				case ready <- announcedAPI:
				default:
				}
			}
		}
	}()
	go func() { exited <- cmd.Wait() }()
	select {
	case announcedAPI := <-ready:
		c.process, c.exited = cmd, exited
		c.offline = offline
		if b, err := os.ReadFile(filepath.Join(c.repo, "api")); err == nil && len(b) <= 512 && loopbackAPI.MatchString(announcedAPI) && strings.TrimSpace(string(b)) == announcedAPI {
			c.ownedAPI = announcedAPI
		}
		if c.ownedAPI == "" {
			_ = cmd.Process.Kill()
			<-exited
			c.process = nil
			return errors.New("Kubo API 地址未通过核验")
		}
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
	return c.closeLocked()
}

func (c *Client) closeLocked() error {
	if c.process == nil {
		return nil
	}
	select {
	case <-c.exited:
		c.process = nil
		c.ownedAPI = ""
		return nil
	default:
	}
	if err := c.validateKeyAPI(); err != nil {
		// Never send shutdown to an API address we did not start. Terminate only
		// our own child process if the repository's API file was redirected.
		if c.process != nil {
			_ = c.process.Process.Kill()
			<-c.exited
		}
		c.process = nil
		c.ownedAPI = ""
		return fmt.Errorf("Kubo API 无法验证，已停止本程序的 Kubo 进程：%w", err)
	}
	if c.process == nil {
		c.ownedAPI = ""
		c.offline = false
		return nil
	}
	_, err := c.runTimeout(5*time.Second, c.ownedAPIArgs("shutdown")...)
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
	c.ownedAPI = ""
	c.offline = false
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
func (c *Client) ExportKey(name, path string) (result error) {
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
	if c.process != nil {
		offline := c.offline
		if err := c.closeLocked(); err != nil {
			return err
		}
		defer func() {
			if err := c.startLocked(offline); err != nil {
				result = errors.Join(result, fmt.Errorf("重启 Kubo 失败：%w", err))
			}
		}()
	}
	if err := c.validateKeyAPI(); err != nil {
		return err
	}
	present, err := sitecleanup.KuboRepoPresent(c.repo)
	if err != nil {
		return err
	}
	if !present {
		return errors.New("Kubo 仓库不存在")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.executable, "--offline", "key", "export", name, "-o", path)
	c.configure(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("导出密钥失败：%s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (c *Client) ImportKey(name, path string) (string, error) {
	if !safeName.MatchString(name) || name == "self" {
		return "", errors.New("invalid key name")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if present, err := sitecleanup.KuboRepoPresent(c.repo); err != nil || !present {
		if err != nil {
			return "", err
		}
		return "", errors.New("Kubo 仓库不存在")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return "", errors.New("导入密钥文件无效")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		return "", errors.New("导入密钥文件已改变")
	}
	key, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(key) > 1<<20 {
		return "", errors.New("导入密钥文件无效")
	}
	if c.process == nil {
		if err = c.validateKeyAPI(); err != nil {
			return "", err
		}
		if err = sitecleanup.CheckOfflineKuboRepo(c.repo); err != nil {
			return "", err
		}
		if err = c.startLocked(true); err != nil {
			return "", err
		}
		defer func() { _ = c.closeLocked() }()
	}
	if err = c.validateKeyAPI(); err != nil {
		return "", err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "key")
	if err != nil {
		return "", err
	}
	if _, err = part.Write(key); err != nil {
		return "", err
	}
	if err = writer.Close(); err != nil {
		return "", err
	}
	port := strings.TrimPrefix(c.ownedAPI, "/ip4/127.0.0.1/tcp/")
	address := "http://127.0.0.1:" + port + "/api/v0/key/import?" + url.Values{"arg": []string{name}}.Encode()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, &body)
	if err != nil {
		return "", errors.New("导入密钥请求无效")
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("导入密钥请求失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("Kubo 未接受导入密钥")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil || len(b) > 64<<10 {
		return "", errors.New("导入密钥响应无效")
	}
	var result struct {
		ID   string `json:"Id"`
		Name string `json:"Name"`
	}
	if json.Unmarshal(b, &result) != nil || result.Name != name || !safeName.MatchString(result.ID) {
		return "", errors.New("导入密钥响应无效")
	}
	return result.ID, nil
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
	return c.keyMatches(name, identity)
}

func (c *Client) keyMatches(name, identity string) (bool, error) {
	for _, base := range []string{"base36", "b58mh", "base32"} {
		list, err := c.run(c.ownedAPIArgs("key", "list", "-l", "--ipns-base="+base)...)
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

func (c *Client) ownedAPIArgs(args ...string) []string {
	if c.process != nil && c.ownedAPI != "" {
		return append([]string{"--api=" + c.ownedAPI}, args...)
	}
	return args
}

// ValidateKeyAPI prevents a repository's api file from routing destructive key
// commands to a daemon that this client did not start.
func (c *Client) ValidateKeyAPI() error { c.mu.Lock(); defer c.mu.Unlock(); return c.validateKeyAPI() }
func (c *Client) validateKeyAPI() error {
	path := filepath.Join(c.repo, "api")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if c.process != nil {
			select {
			case <-c.exited:
				c.process = nil
				c.ownedAPI = ""
			default:
				return errors.New("运行中的 Kubo API 地址缺失")
			}
		}
		return nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 512 {
		return errors.New("Kubo API 地址文件无效")
	}
	if c.process == nil || c.ownedAPI == "" {
		return errors.New("Kubo API 不属于本程序，不能删除密钥")
	}
	select {
	case <-c.exited:
		c.process = nil
		c.ownedAPI = ""
		return errors.New("Kubo 服务已停止，不能核对 API")
	default:
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		return errors.New("Kubo API 地址文件已改变")
	}
	b, err := io.ReadAll(io.LimitReader(f, 513))
	if err != nil || len(b) > 512 || strings.TrimSpace(string(b)) != c.ownedAPI {
		return errors.New("Kubo API 地址与本程序启动的服务不一致")
	}
	return nil
}

// RemoveKeyIfMatches makes retries safe and refuses to delete an unrelated key.
func (c *Client) RemoveKeyIfMatches(name, identity string) error {
	if !safeName.MatchString(name) || name == "self" || (identity != "" && !safeName.MatchString(identity)) {
		return errors.New("invalid key name or identity")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if present, err := sitecleanup.KuboRepoPresent(c.repo); err != nil || !present {
		if err != nil {
			return err
		}
		return errors.New("Kubo 仓库不存在")
	}
	if c.process == nil || c.ownedAPI == "" {
		return errors.New("必须使用本程序启动并核验的 Kubo 服务删除密钥")
	}
	if err := c.validateKeyAPI(); err != nil {
		return err
	}
	existing, err := c.lookupKey(context.Background(), name)
	if err != nil {
		return err
	}
	if existing == "" {
		return nil
	}
	if identity == "" {
		return errors.New("站点密钥身份未知，未删除")
	}
	match, err := c.keyMatches(name, identity)
	if err != nil {
		return err
	}
	if !match {
		return errors.New("站点密钥与原 IPNS 不匹配，未删除")
	}
	_, err = c.run(c.ownedAPIArgs("key", "rm", name)...)
	return err
}

func (c *Client) lookupKey(ctx context.Context, name string) (string, error) {
	list, err := c.runContext(ctx, 15*time.Second, c.ownedAPIArgs("key", "list", "-l")...)
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

// Resolve resolves an IPNS peer ID or DNSLink/ENS name to its root CID.
func (c *Client) Resolve(name string) (string, error) {
	if name == "" || len(name) > 255 || strings.ContainsAny(name, `/\: `) {
		return "", errors.New("IPNS 名称无效")
	}
	value := ""
	var err error
	target := "/ipns/" + name
	if strings.HasSuffix(strings.ToLower(name), ".eth") {
		value, err = resolveENS(context.Background(), ensRPC(), name)
		if err != nil {
			return "", err
		}
		if strings.HasPrefix(value, "/ipns/") {
			target = value
			value = ""
		}
	}
	if value == "" {
		c.mu.Lock()
		value, err = c.runTimeout(90*time.Second, "name", "resolve", "--nocache", target)
		c.mu.Unlock()
	}
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(value, "/ipfs/") {
		return "", errors.New("站点解析结果不是 IPFS 内容")
	}
	resolved := strings.TrimPrefix(value, "/ipfs/")
	parts := strings.SplitN(resolved, "/", 2)
	if !safeName.MatchString(parts[0]) || (len(parts) > 1 && !validContentPath(parts[0], parts[1])) {
		return "", errors.New("站点解析路径无效")
	}
	return resolved, nil
}

// Read fetches a bounded public file without using an HTTP gateway.
func (c *Client) Read(cid, path string) ([]byte, error) {
	if !validContentPath(cid, path) {
		return nil, errors.New("IPFS 内容路径无效")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	temporary, err := c.ensureCommandDaemon()
	if err != nil {
		return nil, err
	}
	if temporary {
		defer func() { _ = c.closeLocked() }()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.executable, "--api="+c.ownedAPI, "cat", "/ipfs/"+cid+"/"+path)
	c.configure(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	const limit = 16 << 20
	data, readErr := io.ReadAll(io.LimitReader(stdout, limit+1))
	if len(data) > limit {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if len(data) > limit {
		return nil, errors.New("站点元数据超过 16 MB")
	}
	if readErr != nil {
		return nil, readErr
	}
	if waitErr != nil {
		return nil, fmt.Errorf("Kubo: %s: %w", strings.TrimSpace(stderr.String()), waitErr)
	}
	return data, nil
}

// Pin stores a full immutable site in the local repository for offline use.
func (c *Client) Pin(cid string) error {
	if !safeName.MatchString(cid) {
		return errors.New("CID 无效")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.run("pin", "add", "--progress=false", "/ipfs/"+cid)
	return err
}

func validContentPath(cid, path string) bool {
	if !safeName.MatchString(cid) || path == "" || strings.Contains(path, `\`) {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, `<>:"|?*`) {
			return false
		}
	}
	return true
}

// CopyContent streams a public IPFS file into an already-open private temporary file.
func (c *Client) CopyContent(cid, path string, out *os.File) error {
	if !validContentPath(cid, path) || out == nil {
		return errors.New("IPFS 内容路径无效")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	temporary, err := c.ensureCommandDaemon()
	if err != nil {
		return err
	}
	if temporary {
		defer func() { _ = c.closeLocked() }()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.executable, "--api="+c.ownedAPI, "cat", "/ipfs/"+cid+"/"+path)
	c.configure(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		return err
	}
	const maxBytes int64 = 1 << 30
	n, copyErr := io.Copy(out, io.LimitReader(stdout, maxBytes+1))
	if n > maxBytes || copyErr != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if n > maxBytes {
		return errors.New("音频文件超过 1 GiB")
	}
	if copyErr != nil {
		return copyErr
	}
	if waitErr != nil {
		return fmt.Errorf("Kubo: %s: %w", strings.TrimSpace(stderr.String()), waitErr)
	}
	return nil
}
