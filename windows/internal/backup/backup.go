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
	"windows/internal/services"
	"windows/internal/store"
)

var uuidPattern = regexp.MustCompile(`(?i)^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

type Parsed struct {
	Site     store.Site
	Articles []store.Article
	Raw      json.RawMessage
}
type rawArticle struct {
	ID            string          `json:"id"`
	Title         string          `json:"title"`
	Content       string          `json:"content"`
	Created       *float64        `json:"created"`
	Modified      *float64        `json:"modified"`
	Attachments   []string        `json:"attachments"`
	Tags          json.RawMessage `json:"tags"`
	ArticleNumber *int            `json:"articleNumber"`
	AudioFilename string          `json:"audioFilename"`
	VideoFilename string          `json:"videoFilename"`
}
type rawPlanet struct {
	ID                string       `json:"id"`
	Name              string       `json:"name"`
	Slug              string       `json:"slug"`
	About             string       `json:"about"`
	IPNS              string       `json:"ipns"`
	Created           *float64     `json:"created"`
	TemplateName      string       `json:"templateName"`
	NextArticleNumber int          `json:"nextArticleNumber"`
	Articles          []rawArticle `json:"articles"`
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
	if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
		suffix := []rune(base[3:])
		if len(suffix) == 1 && ((suffix[0] >= '1' && suffix[0] <= '9') || suffix[0] == '¹' || suffix[0] == '²' || suffix[0] == '³') {
			return false
		}
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
	out, err := ParseMetadata(b)
	if err != nil {
		return Parsed{}, err
	}
	for _, article := range out.Articles {
		info, err := os.Lstat(filepath.Join(dir, article.ID))
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return Parsed{}, fmt.Errorf("备份缺少文章目录 %s", article.ID)
		}
	}
	return out, nil
}

// ParseMetadata decodes private backup metadata without reading a key or files.
// Callers must validate the corresponding archive directories separately.
func ParseMetadata(b []byte) (Parsed, error) {
	if len(b) > 16<<20 {
		return Parsed{}, errors.New("备份元数据过大")
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
	out := Parsed{Site: store.Site{ID: strings.ToUpper(p.ID), Name: p.Name, About: p.About, IPNS: p.IPNS, Created: created, Imported: true, NextArticleNumber: p.NextArticleNumber}, Articles: []store.Article{}, Raw: append(json.RawMessage(nil), b...)}
	var extension struct {
		ScriptID json.RawMessage `json:"windowsPlausibleScriptID"`
		Enabled  json.RawMessage `json:"plausibleEnabled"`
		Domain   json.RawMessage `json:"plausibleDomain"`
		Server   json.RawMessage `json:"plausibleAPIServer"`
	}
	if err := json.Unmarshal(b, &extension); err != nil {
		return Parsed{}, err
	}
	if len(extension.ScriptID) > 0 && string(extension.ScriptID) != "null" {
		var id, domain string
		var enabled bool
		if json.Unmarshal(extension.ScriptID, &id) != nil || json.Unmarshal(extension.Enabled, &enabled) != nil || json.Unmarshal(extension.Domain, &domain) != nil || id == "" || !enabled || !store.ValidPlausibleDomain(domain) || !store.ValidPlausibleScriptID(id) {
			return Parsed{}, errors.New("备份中的 Plausible 站点专属脚本配置无效")
		}
		var server string
		if len(extension.Server) > 0 && string(extension.Server) != "null" {
			if json.Unmarshal(extension.Server, &server) != nil {
				return Parsed{}, errors.New("备份中的 Plausible 服务地址无效")
			}
		}
		origin, err := services.PlausibleServerOrigin(server)
		if err != nil {
			return Parsed{}, err
		}
		if origin == "https://plausible.io" {
			origin = ""
		}
		out.Site.PlausibleOverride = &store.PlausibleTracking{Enabled: true, Domain: domain, ScriptID: id, Server: origin}
	}
	seen := map[string]bool{}
	for _, a := range p.Articles {
		id := strings.ToUpper(a.ID)
		if !uuidPattern.MatchString(id) || seen[id] || a.Created == nil {
			return Parsed{}, errors.New("备份文章标识或日期无效")
		}
		seen[id] = true
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
		var tags map[string]string
		if len(a.Tags) > 0 && string(a.Tags) != "null" {
			if json.Unmarshal(a.Tags, &tags) != nil {
				tags = nil
			}
		}
		out.Articles = append(out.Articles, store.Article{ID: id, SiteID: out.Site.ID, Title: a.Title, Content: a.Content, Created: at, Modified: modified, Attachments: a.Attachments, Tags: tags, ArticleNumber: a.ArticleNumber, AudioFilename: a.AudioFilename, VideoFilename: a.VideoFilename})
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
	if !site.Imported {
		put(root, "doNotIndex", site.DoNotIndex)
		profile, err := store.NormalizeSiteProfile(store.SiteProfile{Domain: site.Domain, TwitterUsername: site.TwitterUsername, GithubUsername: site.GithubUsername, TelegramUsername: site.TelegramUsername, MastodonUsername: site.MastodonUsername, DiscordLink: site.DiscordLink})
		if err != nil {
			return nil, err
		}
		for key, value := range map[string]string{"domain": profile.Domain, "twitterUsername": profile.TwitterUsername, "githubUsername": profile.GithubUsername, "telegramUsername": profile.TelegramUsername, "mastodonUsername": profile.MastodonUsername, "discordLink": profile.DiscordLink} {
			if value != "" {
				put(root, key, value)
			} else {
				delete(root, key)
			}
		}
		if site.PodcastLanguage != "" {
			put(root, "podcastLanguage", site.PodcastLanguage)
		} else {
			delete(root, "podcastLanguage")
		}
		put(root, "podcastExplicit", site.PodcastExplicit)
		categories := map[string][]string{}
		for _, category := range site.PodcastCategories {
			categories[category] = []string{}
		}
		if len(categories) > 0 {
			put(root, "podcastCategories", categories)
		} else {
			delete(root, "podcastCategories")
		}
		if site.Slug != "" {
			put(root, "slug", site.Slug)
		} else {
			delete(root, "slug")
		}
		if site.AuthorName != "" {
			put(root, "authorName", site.AuthorName)
		} else {
			delete(root, "authorName")
		}
	}
	tagSource := original
	if len(tagSource) == 0 {
		tagSource = json.RawMessage(`{"articles":[]}`)
	}
	tags, _, err := ImportedTags(tagSource, articles)
	if err != nil {
		return nil, err
	}
	if _, present := root["tags"]; present || len(tags) > 0 {
		put(root, "tags", tags)
	}
	if site.PlausibleOverride != nil {
		site = store.ApplyPlausibleOverride(site)
		// A Windows override can replace the original key even when its server
		// is unchanged. Preserve the old key privately without reactivating it.
		key := root["plausibleAPIKey"]
		if len(key) > 0 && string(key) != "null" && string(key) != `""` {
			var previous []json.RawMessage
			if existing := root["windowsPreviousPlausibleConfig"]; len(existing) > 0 && string(existing) != "null" {
				if json.Unmarshal(existing, &previous) != nil {
					return nil, errors.New("备份中的历史 Plausible 配置无效")
				}
			}
			record := map[string]json.RawMessage{"apiKey": key, "domain": root["plausibleDomain"], "server": root["plausibleAPIServer"]}
			b, err := json.Marshal(record)
			if err != nil {
				return nil, err
			}
			previous = append(previous, b)
			put(root, "windowsPreviousPlausibleConfig", previous)
			delete(root, "plausibleAPIKey")
		}
		put(root, "plausibleEnabled", site.PlausibleEnabled)
		domain, server := site.PlausibleDomain, site.PlausibleAPIServer
		if !site.PlausibleEnabled {
			domain = site.PlausibleOverride.Domain
			if domain != "" && !store.ValidPlausibleDomain(domain) {
				return nil, errors.New("Plausible 站点域名无效")
			}
			if site.PlausibleOverride.Server != "" {
				origin, err := services.PlausibleServerOrigin(site.PlausibleOverride.Server)
				if err != nil {
					return nil, err
				}
				server = strings.TrimPrefix(origin, "https://")
			}
		}
		put(root, "plausibleDomain", domain)
		put(root, "plausibleAPIServer", server)
		if site.PlausibleEnabled && site.PlausibleScriptID != "" {
			if !store.ValidPlausibleScriptID(site.PlausibleScriptID) {
				return nil, errors.New("Plausible 脚本 ID 无效")
			}
			put(root, "windowsPlausibleScriptID", site.PlausibleScriptID)
		} else {
			delete(root, "windowsPlausibleScriptID")
		}
	}
	put(root, "ipns", site.IPNS)
	put(root, "created", appleDate(site.Created))
	nextNumber := site.NextArticleNumber
	var oldNext int
	if b := root["nextArticleNumber"]; len(b) > 0 && json.Unmarshal(b, &oldNext) != nil {
		return nil, errors.New("invalid backup article counter")
	}
	if oldNext > nextNumber {
		nextNumber = oldNext
	}
	for _, article := range articles {
		if article.ArticleNumber != nil && *article.ArticleNumber > 0 && *article.ArticleNumber >= nextNumber {
			if *article.ArticleNumber == int(^uint(0)>>1) {
				return nil, errors.New("article number limit reached")
			}
			nextNumber = *article.ArticleNumber + 1
		}
	}
	if nextNumber > 0 {
		put(root, "nextArticleNumber", nextNumber)
	}
	put(root, "updated", appleDate(time.Now().UTC()))
	if site.LastPublished != nil && site.LastPublishedCID != "" {
		put(root, "lastPublished", appleDate(*site.LastPublished))
		put(root, "lastPublishedCID", site.LastPublishedCID)
	}
	if site.TemplateName != "" || len(root["templateName"]) == 0 {
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
		if !site.Imported {
			if article.Slug != "" {
				if !store.ValidArticleSlug(article.Slug) {
					return nil, errors.New("文章自定义路径无效")
				}
				put(item, "slug", article.Slug)
				put(item, "link", "/"+article.Slug+"/")
			} else {
				delete(item, "slug")
				put(item, "link", "/"+article.ID+"/")
			}
			if article.ArticleType != 0 && article.ArticleType != 1 {
				return nil, errors.New("文章类型无效")
			}
			if !store.ValidArticleExternalLink(article.ExternalLink) {
				return nil, errors.New("文章外部链接无效")
			}
			put(item, "articleType", article.ArticleType)
			if article.ExternalLink != "" {
				put(item, "externalLink", article.ExternalLink)
			} else {
				delete(item, "externalLink")
			}
			put(item, "isIncludedInNavigation", article.IsIncludedInNavigation)
			if article.IsIncludedInNavigation || article.NavigationWeight != 1 {
				put(item, "navigationWeight", article.NavigationWeight)
			} else {
				delete(item, "navigationWeight")
			}
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
		if article.Tags != nil {
			if len(article.Tags) > 0 {
				put(item, "tags", article.Tags)
			} else {
				delete(item, "tags")
			}
		}
		if article.ArticleNumber != nil {
			put(item, "articleNumber", *article.ArticleNumber)
		}
		if article.AudioFilename != "" {
			put(item, "audioFilename", article.AudioFilename)
		} else {
			delete(item, "audioFilename")
		}
		if article.VideoFilename != "" {
			put(item, "videoFilename", article.VideoFilename)
		} else {
			delete(item, "videoFilename")
		}
		if len(article.Attachments) > 0 {
			put(item, "attachments", article.Attachments)
		} else {
			delete(item, "attachments")
		}
		items = append(items, item)
	}
	put(root, "articles", items)
	return json.MarshalIndent(root, "", "  ")
}

// CheckUnchanged guards original public pages until the source template can be rebuilt.
func CheckUnchanged(original json.RawMessage, site store.Site, articles []store.Article) error {
	if site.PlausibleOverride != nil {
		var fields map[string]json.RawMessage
		if json.Unmarshal(original, &fields) != nil {
			return errors.New("备份元数据无效")
		}
		if key := fields["plausibleAPIKey"]; len(key) > 0 && string(key) != "null" && string(key) != `""` {
			return errors.New("导出前需要安全保留原版统计密钥，请选择支持的模板重建备份")
		}
	}
	var previous rawPlanet
	if err := json.Unmarshal(original, &previous); err != nil {
		return err
	}
	if site.ContentRevision != 0 || previous.Name != site.Name || previous.About != site.About || (site.TemplateName != "" && previous.TemplateName != site.TemplateName) || len(previous.Articles) != len(articles) {
		return errors.New("导入站点已修改，暂不能生成与网页内容一致的备份")
	}
	byID := map[string]rawArticle{}
	for _, a := range previous.Articles {
		byID[strings.ToUpper(a.ID)] = a
	}
	for _, article := range articles {
		old, ok := byID[strings.ToUpper(article.ID)]
		var oldTags map[string]string
		tagErr := error(nil)
		if len(old.Tags) > 0 && string(old.Tags) != "null" {
			tagErr = json.Unmarshal(old.Tags, &oldTags)
		}
		if !ok || old.Title != article.Title || old.Content != article.Content || !sameAttachments(old.Attachments, article.Attachments) || (article.Tags != nil && (tagErr != nil || !sameTags(oldTags, article.Tags))) {
			return errors.New("导入站点已修改，暂不能生成与网页内容一致的备份")
		}
	}
	return nil
}

func sameTags(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func sameAttachments(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
