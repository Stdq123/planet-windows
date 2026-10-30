// Package backup reads and updates Planet's .planet directory metadata.
package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"windows/internal/preview"
	"windows/internal/store"
)

var uuidPattern = regexp.MustCompile(`(?i)^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

type Parsed struct {
	Site     store.Site
	Articles []store.Article
	Raw      json.RawMessage
}
type rawArticle struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Content  string   `json:"content"`
	Created  *float64 `json:"created"`
	Modified *float64 `json:"modified"`
}
type rawPlanet struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	About        string       `json:"about"`
	IPNS         string       `json:"ipns"`
	Created      *float64     `json:"created"`
	TemplateName string       `json:"templateName"`
	Articles     []rawArticle `json:"articles"`
}

func SafeName(name string) bool {
	if name == "" || name == "." || name == ".." || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") || strings.ContainsAny(name, `<>:"/\|?*`) {
		return false
	}
	for _, r := range name {
		if r < 32 {
			return false
		}
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" {
		return false
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return false
	}
	return true
}

func fromAppleDate(value float64) (time.Time, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < -62135596800 || value > 252423993599 {
		return time.Time{}, errors.New("invalid date")
	}
	seconds, fraction := math.Modf(value)
	return time.Unix(int64(seconds)+978307200, int64(fraction*1e9)).UTC(), nil
}
func appleDate(value time.Time) float64 {
	return float64(value.Unix()-978307200) + float64(value.Nanosecond())/1e9
}

func Read(dir string) (Parsed, error) {
	if !strings.EqualFold(filepath.Ext(dir), ".planet") {
		return Parsed{}, errors.New("请选择 .planet 备份目录")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Parsed{}, errors.New("备份目录无效")
	}
	for _, name := range []string{"planet.json", "planet.key"} {
		info, err := os.Lstat(filepath.Join(dir, name))
		if err != nil || !info.Mode().IsRegular() {
			return Parsed{}, fmt.Errorf("备份缺少有效的 %s", name)
		}
	}
	info, err = os.Stat(filepath.Join(dir, "planet.json"))
	if err != nil {
		return Parsed{}, err
	}
	if info.Size() > 16<<20 {
		return Parsed{}, errors.New("备份元数据过大")
	}
	b, err := os.ReadFile(filepath.Join(dir, "planet.json"))
	if err != nil {
		return Parsed{}, err
	}
	var p rawPlanet
	if err := json.Unmarshal(b, &p); err != nil {
		return Parsed{}, fmt.Errorf("备份元数据无效：%w", err)
	}
	if !uuidPattern.MatchString(p.ID) || p.Name == "" || p.IPNS == "" || p.Created == nil || p.TemplateName == "" || p.Articles == nil {
		return Parsed{}, errors.New("备份缺少必需的站点信息")
	}
	created, err := fromAppleDate(*p.Created)
	if err != nil {
		return Parsed{}, err
	}
	out := Parsed{Site: store.Site{ID: strings.ToUpper(p.ID), Name: p.Name, About: p.About, IPNS: p.IPNS, Created: created, Imported: true}, Articles: []store.Article{}, Raw: append(json.RawMessage(nil), b...)}
	seen := map[string]bool{}
	for _, a := range p.Articles {
		id := strings.ToUpper(a.ID)
		if !uuidPattern.MatchString(id) || seen[id] || a.Created == nil {
			return Parsed{}, errors.New("备份文章标识或日期无效")
		}
		seen[id] = true
		info, err := os.Lstat(filepath.Join(dir, id))
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return Parsed{}, fmt.Errorf("备份缺少文章目录 %s", id)
		}
		at, err := fromAppleDate(*a.Created)
		if err != nil {
			return Parsed{}, err
		}
		modified := at
		if a.Modified != nil {
			modified, err = fromAppleDate(*a.Modified)
			if err != nil {
				return Parsed{}, err
			}
		}
		out.Articles = append(out.Articles, store.Article{ID: id, SiteID: out.Site.ID, Title: a.Title, Content: a.Content, Created: at, Modified: modified})
	}
	return out, nil
}

func Merge(original json.RawMessage, site store.Site, articles []store.Article) ([]byte, error) {
	var root map[string]json.RawMessage
	if len(original) > 0 {
		if err := json.Unmarshal(original, &root); err != nil {
			return nil, err
		}
	} else {
		root = map[string]json.RawMessage{}
	}
	if root == nil {
		return nil, errors.New("invalid backup metadata")
	}
	old := []map[string]json.RawMessage{}
	if len(root["articles"]) > 0 {
		if err := json.Unmarshal(root["articles"], &old); err != nil {
			return nil, err
		}
	}
	byID := map[string]map[string]json.RawMessage{}
	for _, item := range old {
		var id string
		_ = json.Unmarshal(item["id"], &id)
		byID[strings.ToUpper(id)] = item
	}
	put := func(m map[string]json.RawMessage, key string, value any) { b, _ := json.Marshal(value); m[key] = b }
	put(root, "id", site.ID)
	put(root, "name", site.Name)
	put(root, "about", site.About)
	put(root, "ipns", site.IPNS)
	put(root, "created", appleDate(site.Created))
	put(root, "updated", appleDate(time.Now().UTC()))
	if site.LastPublished != nil && site.LastPublishedCID != "" {
		put(root, "lastPublished", appleDate(*site.LastPublished))
		put(root, "lastPublishedCID", site.LastPublishedCID)
	}
	if len(root["templateName"]) == 0 {
		name := site.TemplateName
		if name == "" {
			name = "Plain"
		}
		put(root, "templateName", name)
	}
	items := make([]map[string]json.RawMessage, 0, len(articles))
	for _, article := range articles {
		if !uuidPattern.MatchString(article.ID) || article.SiteID != site.ID {
			return nil, errors.New("invalid article")
		}
		item := byID[strings.ToUpper(article.ID)]
		if item == nil {
			item = map[string]json.RawMessage{}
			put(item, "link", "/"+article.ID+"/")
			put(item, "starType", 0)
		}
		var oldContent string
		_ = json.Unmarshal(item["content"], &oldContent)
		if oldContent != article.Content {
			html, err := preview.Markdown(article.Content)
			if err != nil {
				return nil, err
			}
			put(item, "contentRendered", html)
		}
		put(item, "id", article.ID)
		put(item, "title", article.Title)
		put(item, "content", article.Content)
		put(item, "created", appleDate(article.Created))
		put(item, "modified", appleDate(article.Modified))
		if len(original) == 0 && len(article.Attachments) > 0 {
			put(item, "attachments", article.Attachments)
		}
		items = append(items, item)
	}
	put(root, "articles", items)
	return json.MarshalIndent(root, "", "  ")
}

// CheckUnchanged guards original public pages until the source template can be rebuilt.
func CheckUnchanged(original json.RawMessage, site store.Site, articles []store.Article) error {
	var previous rawPlanet
	if err := json.Unmarshal(original, &previous); err != nil {
		return err
	}
	if previous.Name != site.Name || previous.About != site.About || len(previous.Articles) != len(articles) {
		return errors.New("导入站点已修改，暂不能生成与网页内容一致的备份")
	}
	byID := map[string]rawArticle{}
	for _, a := range previous.Articles {
		byID[strings.ToUpper(a.ID)] = a
	}
	for _, article := range articles {
		old, ok := byID[strings.ToUpper(article.ID)]
		if !ok || old.Title != article.Title || old.Content != article.Content {
			return errors.New("导入站点已修改，暂不能生成与网页内容一致的备份")
		}
	}
	return nil
}
