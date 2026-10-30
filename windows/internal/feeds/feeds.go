// Package feeds reads RSS, Atom and JSON Feed documents.
package feeds

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

type Feed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Link        string `json:"link"`
	Items       []Item `json:"items"`
}

type Item struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Link        string `json:"link"`
	Description string `json:"description"`
	Content     string `json:"content"`
	Published   string `json:"published"`
}

func Fetch(address string) (Feed, error) {
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return Feed{}, errors.New("订阅地址必须是 HTTP 或 HTTPS 地址")
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || (req.URL.Scheme != "http" && req.URL.Scheme != "https") || req.URL.User != nil {
			return errors.New("invalid feed redirect")
		}
		return nil
	}}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return Feed{}, err
	}
	req.Header.Set("User-Agent", "Planet-Windows/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return Feed{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Feed{}, fmt.Errorf("订阅请求失败：HTTP %d", resp.StatusCode)
	}
	const limit = 5 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return Feed{}, err
	}
	if len(body) > limit {
		return Feed{}, errors.New("订阅内容超过 5 MB")
	}
	parsed, err := gofeed.NewParser().Parse(strings.NewReader(string(body)))
	if err != nil {
		return Feed{}, fmt.Errorf("无法解析订阅：%w", err)
	}
	out := Feed{Title: parsed.Title, Description: parsed.Description, Link: parsed.Link, Items: []Item{}}
	var jsonFeed struct {
		Version string `json:"version"`
		Items   []struct {
			ID      string  `json:"id"`
			HTML    *string `json:"content_html"`
			Text    *string `json:"content_text"`
			Summary string  `json:"summary"`
		} `json:"items"`
	}
	_ = json.Unmarshal(body, &jsonFeed)
	plain := map[string]string{}
	if strings.HasPrefix(jsonFeed.Version, "https://jsonfeed.org/version/") {
		for _, entry := range jsonFeed.Items {
			if entry.ID != "" && (entry.HTML == nil || *entry.HTML == "") {
				value := entry.Summary
				if entry.Text != nil {
					value = *entry.Text
				}
				plain[entry.ID] = strings.ReplaceAll(html.EscapeString(value), "\n", "<br>\n")
			}
		}
	}
	for _, entry := range parsed.Items {
		content := entry.Content
		if text, ok := plain[entry.GUID]; ok {
			content = text
		}
		out.Items = append(out.Items, Item{ID: entry.GUID, Title: entry.Title, Link: entry.Link, Description: entry.Description, Content: content, Published: entry.Published})
	}
	return out, nil
}
