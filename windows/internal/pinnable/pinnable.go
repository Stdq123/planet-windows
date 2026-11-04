// Package pinnable manages optional Pinnable requests and protected credentials.
package pinnable

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var safeSite = regexp.MustCompile(`^[A-Za-z0-9-]{1,100}$`)
var tokenPath = regexp.MustCompile(`^/pin/[A-Za-z0-9-]{1,128}$`)

// ParseEndpoint accepts only Pinnable HTTPS pin URLs, never credentials or redirects.
func ParseEndpoint(value string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
		return nil, errors.New("Pinnable 地址无效，请使用 HTTPS 固定地址")
	}
	host := strings.ToLower(u.Hostname())
	if host != "pinnable.xyz" && !strings.HasSuffix(host, ".pinnable.xyz") {
		return nil, errors.New("Pinnable 地址域名无效")
	}
	if !tokenPath.MatchString(u.Path) || u.RawPath != "" {
		return nil, errors.New("Pinnable 固定地址格式无效")
	}
	return u, nil
}

type Status struct {
	Status string `json:"status"`
	IPNS   string `json:"last_known_ipns"`
	CID    string `json:"last_known_cid"`
}
type Client struct{ http *http.Client }

func newClient(client *http.Client) *Client {
	copied := *client
	copied.Timeout = 10 * time.Second
	copied.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{http: &copied}
}
func NewClient() *Client { return newClient(&http.Client{}) }

// PinForSite checks the service's reported IPNS before sending a pin request.
// Automatic requests require a known identity; a manual request may accept an
// uninitialized service after the user explicitly confirms its target.
func (c *Client) PinForSite(endpoint, ipns string, requireKnown bool) error {
	u, err := ParseEndpoint(endpoint)
	if err != nil {
		return err
	}
	return c.pinForSite(u.String(), ipns, requireKnown)
}

func MatchesIPNS(reported, expected string) bool {
	reported = strings.TrimSpace(reported)
	reported = strings.TrimPrefix(reported, "/ipns/")
	reported = strings.TrimSuffix(reported, "/")
	return reported != "" && expected != "" && reported == expected
}

func (c *Client) pinForSite(endpoint, ipns string, requireKnown bool) error {
	if ipns == "" {
		return errors.New("当前站点没有 IPNS 身份")
	}
	status, err := c.status(endpoint)
	if err != nil {
		return err
	}
	if status.IPNS == "" {
		if requireKnown {
			return errors.New("固定服务尚未报告站点身份，无法自动核对")
		}
	} else if !MatchesIPNS(status.IPNS, ipns) {
		return errors.New("固定服务报告的 IPNS 与当前站点不一致")
	}
	return c.request(endpoint)
}
func (c *Client) Status(endpoint string) (Status, error) {
	u, err := ParseEndpoint(endpoint)
	if err != nil {
		return Status{}, err
	}
	return c.status(u.String())
}
func (c *Client) request(endpoint string) error {
	resp, err := c.http.Get(endpoint)
	if err != nil {
		return errors.New("无法连接 Pinnable 服务")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return errors.New("Pinnable 未接受固定请求")
	}
	return nil
}
func (c *Client) status(endpoint string) (Status, error) {
	resp, err := c.http.Get(endpoint + "/status")
	if err != nil {
		return Status{}, errors.New("无法连接 Pinnable 服务")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Status{}, errors.New("无法读取 Pinnable 状态")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (256<<10)+1))
	if err != nil || len(b) > 256<<10 {
		return Status{}, errors.New("Pinnable 状态响应无效")
	}
	var status Status
	if json.Unmarshal(b, &status) != nil || status.Status == "" {
		return Status{}, errors.New("Pinnable 状态响应无效")
	}
	return status, nil
}

func configDir(root string, create bool) (string, error) {
	dir := root
	for _, part := range []string{"integrations", "pinnable"} {
		dir = filepath.Join(dir, part)
		info, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) && create {
			if err = os.Mkdir(dir, 0700); err != nil {
				return "", err
			}
			info, err = os.Lstat(dir)
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("固定服务配置目录无效")
		}
	}
	return dir, nil
}

// Save uses Windows user-bound DPAPI; the endpoint is never written as plaintext.
func Save(root, siteID, endpoint string) error {
	if !safeSite.MatchString(siteID) {
		return errors.New("站点标识无效")
	}
	if endpoint == "" {
		dir, err := configDir(root, false)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		path := filepath.Join(dir, siteID+".dat")
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("固定服务配置无效")
		}
		return os.Remove(path)
	}
	u, err := ParseEndpoint(endpoint)
	if err != nil {
		return err
	}
	sealed, err := protect([]byte(u.String()))
	if err != nil {
		return errors.New("无法保护固定服务配置")
	}
	dir, err := configDir(root, true)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".config-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(sealed)
	}
	if err == nil {
		err = f.Sync()
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, siteID+".dat"))
}
func Load(root, siteID string) (string, error) {
	if !safeSite.MatchString(siteID) {
		return "", errors.New("站点标识无效")
	}
	dir, err := configDir(root, false)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, siteID+".dat")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<10 {
		return "", errors.New("固定服务配置无效")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		return "", errors.New("固定服务配置读取失败")
	}
	b, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil || len(b) > 64<<10 {
		return "", errors.New("固定服务配置无效")
	}
	plain, err := unprotect(b)
	if err != nil {
		return "", errors.New("无法解锁固定服务配置")
	}
	u, err := ParseEndpoint(string(plain))
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
