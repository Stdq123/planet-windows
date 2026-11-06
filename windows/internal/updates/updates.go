// Package updates authenticates update metadata and installer bytes. It never
// executes downloaded programs or accepts a publisher key from the network.
package updates

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	releaseversion "windows/internal/version"
)

const AppID = "org.planetable.planetwindows"
const MaxInstallerSize int64 = 512 << 20
const maxEnvelope = 64 << 10

type Config struct {
	FeedURL        string `json:"feedURL"`
	PublicKey      string `json:"publicKey"`
	AutoCheck      bool   `json:"autoCheck"`
	HighestVersion string `json:"highestVersion,omitempty"`
	HighestSHA256  string `json:"highestSHA256,omitempty"`
}
type Release struct {
	Schema         int       `json:"schema"`
	App            string    `json:"app"`
	Platform       string    `json:"platform"`
	Version        string    `json:"version"`
	MinimumVersion string    `json:"minimumVersion"`
	URL            string    `json:"url"`
	SHA256         string    `json:"sha256"`
	Size           int64     `json:"size"`
	PublishedAt    time.Time `json:"publishedAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
}
type Client struct{ http *http.Client }

func newClient(base *http.Client) *Client {
	c := *base
	c.Timeout = 3 * time.Minute
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{&c}
}
func NewClient() *Client { return newClient(&http.Client{}) }

func secureURL(s string) (*url.URL, error) {
	u, e := url.Parse(s)
	if e != nil || len(s) > 4096 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return nil, errors.New("更新地址必须是无用户名和片段的 HTTPS URL")
	}
	return u, nil
}
func origin(u *url.URL) string {
	port := u.Port()
	if port == "" {
		port = "443"
	}
	return strings.ToLower(u.Hostname()) + ":" + port
}
func ValidateConfig(cfg Config) (Config, error) {
	cfg.FeedURL = strings.TrimSpace(cfg.FeedURL)
	cfg.PublicKey = strings.TrimSpace(cfg.PublicKey)
	if cfg.FeedURL == "" && cfg.PublicKey == "" {
		cfg.AutoCheck = false
		cfg.HighestVersion = ""
		cfg.HighestSHA256 = ""
		return cfg, nil
	}
	if _, e := secureURL(cfg.FeedURL); e != nil {
		return Config{}, e
	}
	pub, e := base64.StdEncoding.DecodeString(cfg.PublicKey)
	if e != nil || len(pub) != ed25519.PublicKeySize {
		return Config{}, errors.New("发布者公钥必须是 base64 编码的 Ed25519 公钥")
	}
	cfg.PublicKey = base64.StdEncoding.EncodeToString(pub)
	if cfg.HighestVersion != "" {
		if _, e = releaseversion.Parse(cfg.HighestVersion); e != nil || !validHash(cfg.HighestSHA256) {
			return Config{}, errors.New("已保存的更新版本记录无效")
		}
	}
	return cfg, nil
}
func validHash(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && s == strings.ToLower(s)
}
func Compare(a, b string) (int, error) {
	return releaseversion.Compare(a, b)
}

// Both protocol objects are flat: reject duplicate keys, unknown fields and
// trailing JSON, rather than relying on last-key-wins parser behavior.
func strictJSON(b []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	first, e := d.Token()
	if e != nil || first != json.Delim('{') {
		return errors.New("更新清单不是 JSON 对象")
	}
	seen := map[string]bool{}
	allowed := map[string]bool{}
	typ := reflect.TypeOf(out).Elem()
	for i := 0; i < typ.NumField(); i++ {
		key := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if key != "" && key != "-" {
			allowed[key] = true
		}
	}
	for d.More() {
		token, e := d.Token()
		if e != nil {
			return e
		}
		key, ok := token.(string)
		if !ok || seen[key] || !allowed[key] {
			return errors.New("更新清单包含重复字段")
		}
		seen[key] = true
		var raw json.RawMessage
		if e = d.Decode(&raw); e != nil {
			return e
		}
	}
	if _, e = d.Token(); e != nil {
		return e
	}
	if _, e = d.Token(); e != io.EOF {
		return errors.New("更新清单包含多余内容")
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode(out)
}
func Verify(envelope []byte, cfg Config, current string, now time.Time) (Release, error) {
	cfg, e := ValidateConfig(cfg)
	if e != nil {
		return Release{}, e
	}
	if cfg.FeedURL == "" {
		return Release{}, errors.New("尚未设置受信任的更新源和公钥")
	}
	if len(envelope) > maxEnvelope {
		return Release{}, errors.New("更新清单过大")
	}
	var encoded struct {
		Payload   []byte `json:"payload"`
		Signature []byte `json:"signature"`
	}
	if e = strictJSON(envelope, &encoded); e != nil {
		return Release{}, e
	}
	pub, _ := base64.StdEncoding.DecodeString(cfg.PublicKey)
	if len(encoded.Payload) > 32<<10 || !ed25519.Verify(pub, encoded.Payload, encoded.Signature) {
		return Release{}, errors.New("更新清单签名验证失败")
	}
	var r Release
	if e = strictJSON(encoded.Payload, &r); e != nil {
		return Release{}, e
	}
	if r.Schema != 1 || r.App != AppID || r.Platform != "windows-amd64" {
		return Release{}, errors.New("更新清单不适用于本程序或平台")
	}
	if !validHash(r.SHA256) || r.Size <= 0 || r.Size > MaxInstallerSize {
		return Release{}, errors.New("安装包大小或校验值无效")
	}
	feed, _ := secureURL(cfg.FeedURL)
	download, e := secureURL(r.URL)
	if e != nil || origin(feed) != origin(download) || !strings.HasSuffix(strings.ToLower(download.Path), ".exe") {
		return Release{}, errors.New("安装包必须来自更新源的同一 HTTPS 站点")
	}
	cmp, e := Compare(r.Version, current)
	if e != nil || cmp < 0 {
		return Release{}, errors.New("更新版本旧于当前版本或格式无效")
	}
	// Installing a prerelease is the opt-in. A stable installation must not
	// silently switch channels when its configured feed offers a later alpha.
	targetVersion, _ := releaseversion.Parse(r.Version)
	currentVersion, _ := releaseversion.Parse(current)
	if targetVersion.IsPrerelease() && !currentVersion.IsPrerelease() {
		return Release{}, errors.New("稳定版本不接收预发布更新")
	}
	cmp, e = Compare(current, r.MinimumVersion)
	if e != nil || cmp < 0 {
		return Release{}, errors.New("当前版本不满足更新包的最低升级要求")
	}
	if cmp, e = Compare(r.Version, r.MinimumVersion); e != nil || cmp < 0 {
		return Release{}, errors.New("更新最低版本无效")
	}
	if r.PublishedAt.IsZero() || r.ExpiresAt.IsZero() || r.PublishedAt.After(now.Add(5*time.Minute)) || !r.ExpiresAt.After(now) || !r.ExpiresAt.After(r.PublishedAt) || r.ExpiresAt.Sub(r.PublishedAt) > 30*24*time.Hour {
		return Release{}, errors.New("更新清单已过期或有效期无效")
	}
	if cfg.HighestVersion != "" {
		cmp, e = Compare(r.Version, cfg.HighestVersion)
		if e != nil || cmp < 0 || (cmp == 0 && r.SHA256 != cfg.HighestSHA256) {
			return Release{}, errors.New("更新清单发生版本回退或同版本文件变更")
		}
	}
	return r, nil
}
func (c *Client) get(ctx context.Context, endpoint string) (*http.Response, error) {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if e != nil {
		return nil, errors.New("更新请求无效")
	}
	req.Header.Set("Accept-Encoding", "identity")
	resp, e := c.http.Do(req)
	if e != nil {
		return nil, errors.New("无法连接更新服务器")
	}
	if resp.StatusCode != http.StatusOK || (resp.Header.Get("Content-Encoding") != "" && resp.Header.Get("Content-Encoding") != "identity") {
		resp.Body.Close()
		return nil, errors.New("更新服务器返回错误、重定向或不支持的压缩")
	}
	return resp, nil
}
func (c *Client) Check(ctx context.Context, cfg Config, current string, now time.Time) (Release, error) {
	cfg, e := ValidateConfig(cfg)
	if e != nil {
		return Release{}, e
	}
	if cfg.FeedURL == "" {
		return Release{}, errors.New("尚未设置受信任的更新源和公钥")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	resp, e := c.get(ctx, cfg.FeedURL)
	if e != nil {
		return Release{}, e
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, maxEnvelope+1))
	if e != nil {
		return Release{}, errors.New("无法读取更新清单")
	}
	return Verify(b, cfg, current, now)
}
func (c *Client) Download(ctx context.Context, cfg Config, current string, expected Release, directory string, now time.Time) (result string, resultError error) {
	r, e := c.Check(ctx, cfg, current, now)
	if e != nil {
		return "", e
	}
	if r.Version != expected.Version || r.SHA256 != expected.SHA256 || r.Size != expected.Size {
		return "", errors.New("更新内容已变化，请重新检查")
	}
	if cmp, _ := Compare(r.Version, current); cmp <= 0 {
		return "", errors.New("当前版本无需更新")
	}
	if e = ensureDirectory(directory); e != nil {
		return "", e
	}
	unpin, e := pinDownloadDirectory(directory)
	if e != nil {
		return "", e
	}
	defer unpin()
	resp, e := c.get(ctx, r.URL)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	if resp.ContentLength >= 0 && resp.ContentLength != r.Size {
		return "", errors.New("安装包长度与签名清单不符")
	}
	f, e := createDownloadTemp(directory)
	if e != nil {
		return "", e
	}
	published := false
	defer func() {
		if !published {
			if err := discardDownload(f); err != nil {
				resultError = fmt.Errorf("%v；部分更新缓存保留在 %s", resultError, directory)
			}
		}
		if err := f.Close(); err != nil {
			resultError = fmt.Errorf("无法关闭更新缓存文件，保留位置 %s", directory)
		}
	}()
	hash := sha256.New()
	n, e := io.Copy(io.MultiWriter(f, hash), io.LimitReader(resp.Body, r.Size+1))
	if e == nil && (n != r.Size || hex.EncodeToString(hash.Sum(nil)) != r.SHA256) {
		e = errors.New("安装包校验失败，文件未保存")
	}
	if e == nil {
		e = f.Sync()
	}
	if e != nil {
		return "", e
	}
	if _, e = f.Seek(0, io.SeekStart); e != nil {
		return "", e
	}
	diskHash := sha256.New()
	diskSize, e := io.Copy(diskHash, f)
	if e != nil || diskSize != r.Size || hex.EncodeToString(diskHash.Sum(nil)) != r.SHA256 {
		return "", errors.New("磁盘中的更新包校验失败")
	}
	if !r.ExpiresAt.After(time.Now()) {
		return "", errors.New("下载期间更新清单已过期，请重新检查")
	}
	destination := filepath.Join(directory, fmt.Sprintf("planet-windows-%s-%s.exe", r.Version, r.SHA256[:12]))
	if e = publishDownload(f, destination); e != nil {
		return "", errors.New("无法保存更新包，请选择新的空目录")
	}
	published = true
	return destination, nil
}

func ensureDirectory(path string) error {
	absolute, e := filepath.Abs(path)
	if e != nil {
		return e
	}
	// Reject redirected ancestors before creating anything in the cache.
	for p := absolute; ; p = filepath.Dir(p) {
		if info, e := os.Lstat(p); e == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("更新目录不能是链接")
			}
		} else if !errors.Is(e, os.ErrNotExist) {
			return e
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return os.MkdirAll(absolute, 0700)
}
