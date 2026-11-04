// Package services implements the external pinning and analytics protocols.
package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	cidlib "github.com/ipfs/go-cid"
)

type Client struct{ http *http.Client }
type serviceStatusError struct{ code int }

func (e *serviceStatusError) Error() string {
	return "服务拒绝请求，请检查凭据与服务状态"
}

func newClient(base *http.Client) *Client {
	c := *base
	c.Timeout = 15 * time.Second
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{http: &c}
}
func NewClient() *Client { return newClient(&http.Client{}) }
func validToken(s string) bool {
	return len(s) > 0 && len(s) <= 4096 && strings.TrimSpace(s) == s && !strings.ContainsAny(s, "\r\n")
}
func (c *Client) request(endpoint, token string, body any, out any) error {
	if !validToken(token) {
		return errors.New("服务令牌无效")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return errors.New("请求无效")
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return errors.New("服务地址无效")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("无法连接服务")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &serviceStatusError{code: resp.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (256<<10)+1))
	if err != nil || len(b) > 256<<10 || json.Unmarshal(b, out) != nil {
		return errors.New("服务响应无效")
	}
	return nil
}
func (c *Client) pin(endpoint, token, name, cid string) (string, error) {
	if _, err := cidlib.Decode(cid); err != nil || strings.TrimSpace(name) == "" || len(name) > 255 {
		return "", errors.New("固定内容无效")
	}
	var result struct {
		RequestID string `json:"requestid"`
		Pin       struct {
			CID string `json:"cid"`
		} `json:"pin"`
	}
	if err := c.request(endpoint, token, map[string]string{"name": name, "cid": cid}, &result); err != nil {
		return "", err
	}
	if !requestIDPattern.MatchString(result.RequestID) || result.Pin.CID != cid {
		return "", errors.New("固定服务响应无效")
	}
	return result.RequestID, nil
}
func (c *Client) PinFilebase(token, name, cid string) (string, error) {
	return c.pin("https://api.filebase.io/v1/ipfs/pins", token, name, cid)
}

type PinStatus struct {
	RequestID string `json:"requestid"`
	Status    string `json:"status"`
	Pin       struct {
		CID string `json:"cid"`
	} `json:"pin"`
}

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,200}$`)

func (c *Client) filebaseStatus(endpoint, token, id string) (PinStatus, error) {
	if !requestIDPattern.MatchString(id) || !validToken(token) {
		return PinStatus{}, errors.New("固定请求标识无效")
	}
	req, err := http.NewRequest(http.MethodGet, endpoint+"/"+id, nil)
	if err != nil {
		return PinStatus{}, errors.New("固定服务地址无效")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.http.Do(req)
	if err != nil {
		return PinStatus{}, errors.New("无法连接固定服务")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return PinStatus{}, errors.New("无法查询固定状态，请检查凭据")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (256<<10)+1))
	var status PinStatus
	if err != nil || len(b) > 256<<10 || json.Unmarshal(b, &status) != nil || status.RequestID != id || status.Status == "" || status.Pin.CID == "" {
		return PinStatus{}, errors.New("固定状态响应无效")
	}
	return status, nil
}
func (c *Client) FilebaseStatus(token, id string) (PinStatus, error) {
	return c.filebaseStatus("https://api.filebase.io/v1/ipfs/pins", token, id)
}

type Pinner interface {
	PinFilebase(token, name, cid string) (string, error)
}

// PinPublished creates independent pins, so no other account's pin is replaced.
func PinPublished(root, siteID, cid string, pinner Pinner) error {
	return pinPublished(root, siteID, cid, pinner, false)
}
func RetryPublished(root, siteID, cid string, pinner Pinner) error {
	return pinPublished(root, siteID, cid, pinner, true)
}
func pinPublished(root, siteID, cid string, pinner Pinner, force bool) error {
	cfg, err := LoadConfig(root, siteID, "filebase")
	if err != nil {
		return err
	}
	if cfg.Token == "" {
		return nil
	}
	if !force && cfg.PendingCID != "" {
		return errors.New("上次固定请求结果不确定，请先到 Filebase 检查后再手动重试")
	}
	if !force && cfg.LastCID == cid {
		return nil
	}
	cfg.PendingCID = cid
	if err = SaveConfig(root, siteID, "filebase", cfg); err != nil {
		return errors.New("无法保存固定请求状态")
	}
	id, err := pinner.PinFilebase(cfg.Token, cfg.Name, cid)
	if err != nil {
		return errors.New("固定请求结果不确定，请先到 Filebase 检查，避免重复计费")
	}
	cfg.LastCID = cid
	cfg.RequestID = id
	cfg.PendingCID = ""
	if err = SaveConfig(root, siteID, "filebase", cfg); err != nil {
		return errors.New("固定请求已发送，但保存本机状态失败；请到 Filebase 检查")
	}
	return nil
}

type Stats struct {
	Visitors  int `json:"visitors"`
	Pageviews int `json:"pageviews"`
}

func (c *Client) stats(endpoint, token, domain string) (Stats, error) {
	return c.statsRange(endpoint, token, domain, "7d")
}
func (c *Client) statsRange(endpoint, token, domain, period string) (Stats, error) {
	if period != "day" && period != "7d" && period != "30d" {
		return Stats{}, errors.New("统计时间范围无效")
	}
	if strings.TrimSpace(domain) == "" {
		return Stats{}, errors.New("统计域名无效")
	}
	var result struct {
		Results []struct {
			Metrics []int `json:"metrics"`
		} `json:"results"`
	}
	err := c.request(endpoint, token, map[string]any{"site_id": domain, "date_range": period, "metrics": []string{"visitors", "pageviews"}}, &result)
	if err != nil {
		return Stats{}, err
	}
	if len(result.Results) != 1 || len(result.Results[0].Metrics) != 2 {
		return Stats{}, errors.New("统计服务响应无效")
	}
	return Stats{result.Results[0].Metrics[0], result.Results[0].Metrics[1]}, nil
}
func (c *Client) PlausibleStats(token, domain string) (Stats, error) {
	return c.stats("https://plausible.io/api/v2/query", token, domain)
}

var plausibleServerHost = regexp.MustCompile(`(?i)^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*$`)

// PlausibleServerOrigin accepts an explicitly configured HTTPS origin only.
// API keys are never sent across redirects or to a URL embedded in a backup.
func PlausibleServerOrigin(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "https://plausible.io"
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") || strings.ContainsAny(value, "\r\n\\") || strings.HasSuffix(u.Host, ":") {
		return "", errors.New("Plausible 服务地址必须是 HTTPS 域名或 IP，不含路径和凭据")
	}
	host := u.Hostname()
	if host == "" || strings.Contains(host, "%") || (net.ParseIP(host) == nil && (len(host) > 253 || !plausibleServerHost.MatchString(host))) {
		return "", errors.New("Plausible 服务地址无效")
	}
	if port := u.Port(); port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 {
			return "", errors.New("Plausible 服务端口无效")
		}
	}
	return "https://" + strings.ToLower(u.Host), nil
}
func (c *Client) PlausibleStatsAt(server, token, domain string) (Stats, error) {
	return c.PlausibleStatsAtRange(server, token, domain, "7d")
}
func (c *Client) PlausibleStatsAtRange(server, token, domain, period string) (Stats, error) {
	origin, err := PlausibleServerOrigin(server)
	if err != nil {
		return Stats{}, err
	}
	stats, err := c.statsRange(origin+"/api/v2/query", token, domain, period)
	var status *serviceStatusError
	if errors.As(err, &status) && (status.code == http.StatusNotFound || status.code == http.StatusMethodNotAllowed) {
		return c.legacyPlausibleStats(origin, token, domain, period)
	}
	return stats, err
}

// Older self-hosted Plausible versions implement only the v1 aggregate API.
func (c *Client) legacyPlausibleStats(origin, token, domain, period string) (Stats, error) {
	if !validToken(token) || strings.TrimSpace(domain) == "" || (period != "day" && period != "7d" && period != "30d") {
		return Stats{}, errors.New("统计配置无效")
	}
	query := url.Values{"site_id": {domain}, "period": {period}, "metrics": {"visitors,pageviews"}}
	req, err := http.NewRequest(http.MethodGet, origin+"/api/v1/stats/aggregate?"+query.Encode(), nil)
	if err != nil {
		return Stats{}, errors.New("服务地址无效")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Stats{}, errors.New("无法连接服务")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Stats{}, &serviceStatusError{code: resp.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (256<<10)+1))
	var result struct {
		Results struct {
			Visitors struct {
				Value *int `json:"value"`
			} `json:"visitors"`
			Pageviews struct {
				Value *int `json:"value"`
			} `json:"pageviews"`
		} `json:"results"`
	}
	if err != nil || len(b) > 256<<10 || json.Unmarshal(b, &result) != nil || result.Results.Visitors.Value == nil || result.Results.Pageviews.Value == nil {
		return Stats{}, errors.New("统计服务响应无效")
	}
	return Stats{Visitors: *result.Results.Visitors.Value, Pageviews: *result.Results.Pageviews.Value}, nil
}
