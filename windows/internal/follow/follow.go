// Package follow reads Planet sites through Kubo without a public HTTP gateway.
package follow

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	ensip15 "github.com/adraffy/go-ens-normalize/ensip15"

	"windows/internal/backup"
	"windows/internal/feeds"
	"windows/internal/preview"
)

type Node interface {
	Resolve(string) (string, error)
	Read(string, string) ([]byte, error)
	Pin(string) error
}
type Result struct {
	Feed feeds.Feed
	CID  string
}

var ipnsName = regexp.MustCompile(`^(?:k51|bafz|Qm|12D3Koo)[A-Za-z0-9]{8,}$`)
var articleID = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

func Normalize(address string) (string, error) {
	name := strings.TrimSpace(address)
	for _, prefix := range []string{"planet://", "ipns://", "/ipns/"} {
		if strings.HasPrefix(strings.ToLower(name), prefix) {
			name = name[len(prefix):]
			break
		}
	}
	name = strings.TrimSuffix(name, "/")
	if ipnsName.MatchString(name) {
		return name, nil
	}
	normalized, err := ensip15.Shared().Normalize(name)
	if err == nil && strings.HasSuffix(normalized, ".eth") {
		return normalized, nil
	}
	return "", errors.New("请输入 Planet/IPNS 名称或 ENS .eth 域名")
}
func Fetch(node Node, address string, pin bool) (Result, error) {
	name, err := Normalize(address)
	if err != nil {
		return Result{}, err
	}
	resolved, err := node.Resolve(name)
	if err != nil {
		return Result{}, fmt.Errorf("解析站点失败：%w", err)
	}
	parts := strings.SplitN(resolved, "/", 2)
	cid := parts[0]
	basePath := ""
	if !regexp.MustCompile(`^[A-Za-z0-9-]+$`).MatchString(cid) {
		return Result{}, errors.New("站点 CID 无效")
	}
	if len(parts) > 1 {
		basePath = parts[1]
		for _, part := range strings.Split(basePath, "/") {
			if !backup.SafeName(part) {
				return Result{}, errors.New("站点子路径无效")
			}
		}
	}
	metadataPath := "planet.json"
	if basePath != "" {
		metadataPath = basePath + "/" + metadataPath
	}
	raw, err := node.Read(cid, metadataPath)
	if err != nil {
		return Result{}, fmt.Errorf("读取站点失败：%w", err)
	}
	var planet struct {
		Name     string `json:"name"`
		About    string `json:"about"`
		Articles []struct {
			ID            string   `json:"id"`
			Link          string   `json:"link"`
			Title         string   `json:"title"`
			Content       string   `json:"content"`
			Rendered      string   `json:"contentRendered"`
			Created       *float64 `json:"created"`
			Attachments   []string `json:"attachments"`
			AudioFilename string   `json:"audioFilename"`
		} `json:"articles"`
	}
	if err = json.Unmarshal(raw, &planet); err != nil || planet.Name == "" || planet.Articles == nil || len(planet.Articles) > 100000 {
		return Result{}, errors.New("Planet 公开元数据无效")
	}
	base := &url.URL{Scheme: "https", Host: "ipfs.io", Path: "/ipns/" + name + "/"}
	feed := feeds.Feed{Title: planet.Name, Description: planet.About, Link: base.String(), Items: []feeds.Item{}}
	for _, a := range planet.Articles {
		if !articleID.MatchString(a.ID) || len(a.ID) > 128 {
			return Result{}, errors.New("文章标识无效")
		}
		for _, name := range a.Attachments {
			if !backup.SafeName(name) || strings.HasSuffix(strings.ToLower(name), ".key") {
				return Result{}, errors.New("附件名称无效")
			}
		}
		if a.AudioFilename != "" {
			found := false
			for _, n := range a.Attachments {
				if n == a.AudioFilename {
					found = true
					break
				}
			}
			if !found {
				return Result{}, errors.New("音频附件未登记")
			}
		}
		link := a.Link
		if link == "" {
			link = "/" + a.ID + "/"
		}
		u, e := url.Parse(link)
		if e != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") || u.RawQuery != "" || u.Fragment != "" || strings.Contains(u.Path, `\`) {
			return Result{}, errors.New("文章路径无效")
		}
		for _, segment := range strings.Split(u.Path, "/") {
			if segment == ".." || segment == "." {
				return Result{}, errors.New("文章路径无效")
			}
		}
		ref := &url.URL{Scheme: "https", Host: "ipfs.io", Path: "/ipns/" + name + u.Path}
		content := a.Rendered
		if content == "" {
			content, e = preview.Markdown(a.Content)
			if e != nil {
				return Result{}, e
			}
		}
		date := ""
		if a.Created != nil && *a.Created > -62135596800 && *a.Created < 242640921599 {
			date = time.Unix(int64(*a.Created)+978307200, 0).UTC().Format(time.RFC3339)
		}
		feed.Items = append(feed.Items, feeds.Item{ID: a.ID, Title: a.Title, Link: ref.String(), Content: content, Published: date, ContentCID: cid, ContentPath: basePath, ArticleID: a.ID, Attachments: a.Attachments, AudioFilename: a.AudioFilename})
	}
	if pin {
		if err = node.Pin(cid); err != nil {
			return Result{}, fmt.Errorf("固定站点失败：%w", err)
		}
	}
	return Result{Feed: feed, CID: cid}, nil
}
