package services

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var cfAccountPattern = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
var cfProjectPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func ValidCloudflareConfig(account, project, token string) bool {
	return cfAccountPattern.MatchString(account) && cfProjectPattern.MatchString(project) && validToken(token)
}

type CloudflareClient struct {
	http *http.Client
	base string
}

func NewCloudflareClient() *CloudflareClient {
	return newCloudflareClient(&http.Client{}, "https://api.cloudflare.com/client/v4")
}
func newCloudflareClient(base *http.Client, endpoint string) *CloudflareClient {
	c := *base
	c.Timeout = 5 * time.Minute
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &CloudflareClient{http: &c, base: strings.TrimRight(endpoint, "/")}
}

func (c *CloudflareClient) request(method, path, token, contentType string, body []byte) (int, []byte, error) {
	req, err := http.NewRequest(method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, errors.New("Cloudflare 请求无效")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return 0, nil, errors.New("无法连接 Cloudflare")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return response.StatusCode, nil, errors.New("Cloudflare 响应无效")
	}
	return response.StatusCode, data, nil
}
func cfResult(data []byte, out any) error {
	var envelope struct {
		Success bool            `json:"success"`
		Result  json.RawMessage `json:"result"`
	}
	if json.Unmarshal(data, &envelope) != nil || !envelope.Success {
		return errors.New("Cloudflare 拒绝请求")
	}
	if out != nil && json.Unmarshal(envelope.Result, out) != nil {
		return errors.New("Cloudflare 响应格式无效")
	}
	return nil
}
func (c *CloudflareClient) jsonRequest(method, path, token string, payload any, out any) (int, error) {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return 0, err
		}
	}
	status, data, err := c.request(method, path, token, "application/json", body)
	if err != nil {
		return status, err
	}
	if status < 200 || status >= 300 {
		return status, fmt.Errorf("Cloudflare 请求失败（HTTP %d）", status)
	}
	return status, cfResult(data, out)
}

type cfAsset struct {
	path, hash, mime string
	data             []byte
}

func collectCFAssets(dir string) ([]cfAsset, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	var assets []cfAsset
	var total int64
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("发布目录包含链接")
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return errors.New("发布目录包含特殊文件")
		}
		if info.Size() > 25<<20 {
			return fmt.Errorf("文件 %s 超过 Cloudflare Pages 25 MiB 限制", filepath.ToSlash(rel))
		}
		total += info.Size()
		if total > 256<<20 || len(assets) >= 20000 {
			return errors.New("发布站点文件过多或总大小超过 256 MiB")
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		actual, err := f.Stat()
		if err != nil || !os.SameFile(info, actual) {
			f.Close()
			return errors.New("发布文件读取期间发生变化")
		}
		data, err := io.ReadAll(io.LimitReader(f, (25<<20)+1))
		f.Close()
		if err != nil || len(data) > 25<<20 {
			return errors.New("无法读取发布文件")
		}
		hash := sha256.Sum256(data)
		kind := mime.TypeByExtension(filepath.Ext(path))
		if kind == "" {
			kind = "application/octet-stream"
		}
		assets = append(assets, cfAsset{path: "/" + filepath.ToSlash(rel), hash: hex.EncodeToString(hash[:16]), mime: kind, data: data})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(assets) == 0 {
		return nil, errors.New("没有可发布的文件")
	}
	return assets, nil
}

// Deploy uses Cloudflare Pages Direct Upload; the caller supplies an already
// generated public snapshot and explicitly configured account credentials.
func (c *CloudflareClient) Deploy(dir, account, project, token string) (string, error) {
	if !ValidCloudflareConfig(account, project, token) {
		return "", errors.New("Cloudflare 账号、项目或令牌无效")
	}
	assets, err := collectCFAssets(dir)
	if err != nil {
		return "", err
	}
	projectPath := "/accounts/" + account + "/pages/projects/" + project
	var existing struct {
		Name             string `json:"name"`
		ProductionBranch string `json:"production_branch"`
	}
	status, err := c.jsonRequest(http.MethodGet, projectPath, token, nil, &existing)
	if err != nil && status == http.StatusNotFound {
		_, err = c.jsonRequest(http.MethodPost, "/accounts/"+account+"/pages/projects", token, map[string]string{"name": project, "production_branch": "main"}, &existing)
	}
	if err != nil {
		return "", err
	}
	if existing.Name != "" && existing.Name != project {
		return "", errors.New("Cloudflare 项目不匹配")
	}
	var auth struct {
		JWT string `json:"jwt"`
	}
	if _, err = c.jsonRequest(http.MethodGet, projectPath+"/upload-token", token, nil, &auth); err != nil {
		return "", err
	}
	if !validToken(auth.JWT) {
		return "", errors.New("Cloudflare 上传凭据无效")
	}
	manifest := map[string]string{}
	byHash := map[string]cfAsset{}
	for _, asset := range assets {
		manifest[asset.path] = asset.hash
		byHash[asset.hash] = asset
	}
	hashes := make([]string, 0, len(byHash))
	for hash := range byHash {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)
	var missing []string
	if _, err = c.jsonRequest(http.MethodPost, "/pages/assets/check-missing", auth.JWT, map[string]any{"hashes": hashes}, &missing); err != nil {
		return "", err
	}
	batch := []map[string]any{}
	batchSize := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		_, e := c.jsonRequest(http.MethodPost, "/pages/assets/upload", auth.JWT, batch, nil)
		batch = nil
		batchSize = 0
		return e
	}
	seen := map[string]bool{}
	for _, hash := range missing {
		asset, ok := byHash[hash]
		if !ok || seen[hash] {
			return "", errors.New("Cloudflare 返回未知或重复文件标识")
		}
		seen[hash] = true
		if batchSize+len(asset.data) > 10<<20 && len(batch) > 0 {
			if err = flush(); err != nil {
				return "", err
			}
		}
		batch = append(batch, map[string]any{"key": asset.hash, "value": base64.StdEncoding.EncodeToString(asset.data), "metadata": map[string]string{"contentType": asset.mime}, "base64": true})
		batchSize += len(asset.data)
	}
	if err = flush(); err != nil {
		return "", err
	}
	if _, err = c.jsonRequest(http.MethodPost, "/pages/assets/upsert-hashes", auth.JWT, map[string]any{"hashes": hashes}, nil); err != nil {
		return "", err
	}
	var payload bytes.Buffer
	form := multipart.NewWriter(&payload)
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	if err = form.WriteField("manifest", string(manifestJSON)); err != nil {
		return "", err
	}
	if existing.ProductionBranch != "" {
		if err = form.WriteField("branch", existing.ProductionBranch); err != nil {
			return "", err
		}
	}
	if err = form.Close(); err != nil {
		return "", err
	}
	status, data, err := c.request(http.MethodPost, projectPath+"/deployments", token, form.FormDataContentType(), payload.Bytes())
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("Cloudflare 部署失败（HTTP %d）", status)
	}
	var result struct {
		URL         string   `json:"url"`
		Aliases     []string `json:"aliases"`
		Environment string   `json:"environment"`
	}
	if err = cfResult(data, &result); err != nil {
		return "", err
	}
	for _, candidate := range append(result.Aliases, result.URL) {
		if !strings.Contains(candidate, "://") {
			candidate = "https://" + candidate
		}
		parsed, e := url.Parse(candidate)
		if e == nil && parsed.Scheme == "https" && parsed.User == nil && strings.HasSuffix(strings.ToLower(parsed.Hostname()), ".pages.dev") {
			return "https://" + parsed.Hostname() + "/", nil
		}
	}
	return "", errors.New("Cloudflare 已创建部署，但未返回可用的 Pages 地址")
}
