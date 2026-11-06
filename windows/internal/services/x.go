package services

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// XCredentials are OAuth 1.0a user credentials, stored only in DPAPI config.
type XCredentials struct {
	APIKey            string `json:"apiKey"`
	APISecret         string `json:"apiSecret"`
	AccessToken       string `json:"accessToken"`
	AccessTokenSecret string `json:"accessTokenSecret"`
}

func ValidXCredentials(credentials XCredentials) bool {
	for _, value := range []string{credentials.APIKey, credentials.APISecret, credentials.AccessToken, credentials.AccessTokenSecret} {
		if len(value) == 0 || len(value) > 4096 {
			return false
		}
		for _, ch := range value {
			if ch < '!' || ch > '~' {
				return false
			}
		}
	}
	return true
}

func validXConfig(cfg Config) bool {
	return cfg.Name == "X" && cfg.XCredentials != nil && ValidXCredentials(*cfg.XCredentials) && cfg.Token == cfg.XCredentials.AccessToken
}

type XPoster interface {
	CreateXPost(XCredentials, string) (string, error)
}

const (
	xUncertain    = "X 请求结果不确定，请先到 X 检查后再重试。"
	xUnauthorized = "X 凭据无效或已过期，请检查四项 OAuth 凭据。"
	xForbidden    = "X 拒绝发布，请检查应用的读写权限与账号访问权限。"
	xBilling      = "X API 额度不足，请检查开发者账户的用量和付费状态。"
	xRateLimited  = "X 请求过于频繁，请稍后手动重试。"
	xRejected     = "X 拒绝发布，请检查文章内容和开发者账户状态。"
)

// XPostError contains only a safe message and an HTTP outcome, never API bodies.
type XPostError struct {
	statusCode int
	message    string
}

func (e *XPostError) Error() string { return e.message }

// XPostRejected is true only when X returned a definite HTTP 4xx rejection.
// Connection errors, redirects, 5xx and malformed successes remain uncertain.
func XPostRejected(err error) bool {
	var postErr *XPostError
	return errors.As(err, &postErr) && postErr.statusCode >= 400 && postErr.statusCode < 500
}

// XPostMessage is safe for both UI display and durable per-article records.
func XPostMessage(err error) string {
	var postErr *XPostError
	if errors.As(err, &postErr) && validXErrorMessage(postErr.message) && postErr.message != "" {
		return postErr.message
	}
	return xUncertain
}

func validXErrorMessage(message string) bool {
	switch message {
	case "", xUncertain, xUnauthorized, xForbidden, xBilling, xRateLimited, xRejected:
		return true
	default:
		return false
	}
}

func xResponseError(status int) error {
	message := xUncertain
	switch status {
	case http.StatusUnauthorized:
		message = xUnauthorized
	case http.StatusForbidden:
		message = xForbidden
	case http.StatusPaymentRequired:
		message = xBilling
	case http.StatusTooManyRequests:
		message = xRateLimited
	default:
		if status >= 400 && status < 500 {
			message = xRejected
		}
	}
	return &XPostError{statusCode: status, message: message}
}

func validXPostID(id string) bool {
	if id == "" || len(id) > 20 || id[0] == '0' {
		return false
	}
	for _, ch := range id {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	_, err := strconv.ParseUint(id, 10, 64)
	return err == nil
}

func (c *Client) CreateXPost(credentials XCredentials, text string) (string, error) {
	return c.createXPost("https://api.x.com/2/tweets", credentials, text)
}

func (c *Client) createXPost(endpoint string, credentials XCredentials, text string) (string, error) {
	if !ValidXCredentials(credentials) {
		return "", errors.New("X 凭据配置无效")
	}
	if !utf8.ValidString(text) || strings.TrimSpace(text) == "" || len(text) > 32768 {
		return "", errors.New("X 发布内容无效")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return "", errors.New("X 服务地址无效")
	}
	payload, err := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: text})
	if err != nil {
		return "", errors.New("X 发布内容无效")
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", errors.New("X 服务地址无效")
	}
	nonce := make([]byte, 24)
	if _, err = rand.Read(nonce); err != nil {
		return "", errors.New("无法生成 X 请求签名")
	}
	header, err := xOAuthHeader(req.Method, endpoint, credentials, hex.EncodeToString(nonce), strconv.FormatInt(time.Now().Unix(), 10))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", header)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	// Do not provide replay support for a request that creates public content.
	req.GetBody = nil
	client := newClient(c.http)
	resp, err := client.http.Do(req)
	if err != nil {
		return "", xResponseError(0)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", xResponseError(resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (256<<10)+1))
	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err != nil || len(data) > 256<<10 || json.Unmarshal(data, &result) != nil || !validXPostID(result.Data.ID) {
		return "", xResponseError(0)
	}
	return result.Data.ID, nil
}

// JSON payloads are not OAuth form parameters. Only query parameters and the
// OAuth header fields enter the signature (OAuth Core 1.0a section 9.1.1).
// Explicit nonce/time keep signing independently testable against spec vectors.
func xOAuthHeader(method, endpoint string, credentials XCredentials, nonce, timestamp string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return "", errors.New("X 服务地址无效")
	}
	oauth := map[string]string{
		"oauth_consumer_key":     credentials.APIKey,
		"oauth_nonce":            nonce,
		"oauth_signature_method": "HMAC-SHA1",
		"oauth_timestamp":        timestamp,
		"oauth_token":            credentials.AccessToken,
		"oauth_version":          "1.0",
	}
	params, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", errors.New("X 服务地址无效")
	}
	for key, value := range oauth {
		params.Set(key, value)
	}
	var pairs []string
	for key, values := range params {
		for _, value := range values {
			pairs = append(pairs, xPercentEncode(key)+"="+xPercentEncode(value))
		}
	}
	sort.Strings(pairs)
	u.RawQuery, u.Fragment, u.RawFragment = "", "", ""
	u.Scheme, u.Host = strings.ToLower(u.Scheme), strings.ToLower(u.Host)
	if u.Scheme == "https" && u.Port() == "443" || u.Scheme == "http" && u.Port() == "80" {
		u.Host = strings.TrimSuffix(u.Host, ":"+u.Port())
	}
	if u.Path == "" {
		u.Path = "/"
	}
	base := strings.ToUpper(method) + "&" + xPercentEncode(u.String()) + "&" + xPercentEncode(strings.Join(pairs, "&"))
	mac := hmac.New(sha1.New, []byte(xPercentEncode(credentials.APISecret)+"&"+xPercentEncode(credentials.AccessTokenSecret)))
	_, _ = mac.Write([]byte(base))
	oauth["oauth_signature"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	var fields []string
	for key, value := range oauth {
		fields = append(fields, xPercentEncode(key)+`="`+xPercentEncode(value)+`"`)
	}
	sort.Strings(fields)
	return "OAuth " + strings.Join(fields, ", "), nil
}

func xPercentEncode(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}
