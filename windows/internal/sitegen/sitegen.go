// Package sitegen builds a basic static site from the local library.
package sitegen

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"windows/internal/preview"
	"windows/internal/services"
	"windows/internal/siteimage"
	"windows/internal/store"
	"windows/internal/tagname"
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

func articleLink(article store.Article) string {
	if article.Slug != "" {
		return "/" + article.Slug + "/"
	}
	return "/" + article.ID + "/"
}

const header = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Title}}</title>{{if .Site.AvatarPNG}}<link rel="icon" href="{{.IconPrefix}}favicon.ico">{{end}}{{if .Site.PlausibleEnabled}}{{if .Site.PlausibleScriptID}}<script async src="https://{{.Site.PlausibleAPIServer}}/js/{{.Site.PlausibleScriptID}}.js"></script><script>window.plausible=window.plausible||function(){(plausible.q=plausible.q||[]).push(arguments)},plausible.init=plausible.init||function(i){plausible.o=i||{}};plausible.init()</script>{{else}}<script defer data-domain="{{.Site.PlausibleDomain}}" src="https://{{.Site.PlausibleAPIServer}}/js/plausible.local.js"></script>{{end}}{{end}}<style>body{max-width:740px;margin:70px auto;padding:0 24px;font:17px/1.8 system-ui,sans-serif;color:#262b32}h1,h2{line-height:1.25}a{color:#52649c}article{margin:48px 0}footer{margin-top:80px;color:#888;font-size:13px}pre{overflow:auto;background:#f5f5f5;padding:16px}img{max-width:100%}</style></head><body>`
const footer = `<footer>Published with Planet</footer></body></html>`

var indexTemplate = template.Must(template.New("index").Funcs(template.FuncMap{"articleHref": relativeArticleLink}).Parse(header + `<header>{{if .Site.AvatarPNG}}<img src="avatar.png" alt="" width="96" height="96" style="object-fit:cover;border-radius:50%">{{end}}<h1>{{.Site.Name}}</h1><p>{{.Site.About}}</p><nav>{{range .Navigation}}<a href="{{.Href}}">{{.Title}}</a> {{end}}{{if .Site.Tags}}<a href="tags.html">标签</a>{{end}} {{if .HasPodcast}}<a href="podcast.xml">播客</a>{{end}} {{range .Social}}<a href="{{.Href}}" rel="noopener noreferrer">{{.Title}}</a> {{end}}</nav></header><main>{{range .Articles}}<article><h2><a href="{{articleHref .}}">{{.Title}}</a></h2><time>{{.Created.Format "2006-01-02"}}</time></article>{{end}}</main>` + footer))
var postTemplate = template.Must(template.New("post").Parse(header + `{{if .Article.ExternalLink}}<script>if(!/Planet\/\S*/.test(navigator.userAgent)){window.location.href={{.Article.ExternalLink}};}</script>{{end}}<header><a href="../">{{.Site.Name}}</a><nav>{{range .Navigation}}<a href="{{.Href}}">{{.Title}}</a> {{end}}</nav></header><article><h1>{{.Article.Title}}</h1><time>{{.Article.Created.Format "2006-01-02"}}</time>{{if .Article.AudioFilename}}<audio controls preload="metadata" src="{{.Article.AudioFilename}}"></audio>{{end}}{{if .Article.VideoFilename}}<video controls preload="metadata" src="{{.Article.VideoFilename}}"></video>{{end}}<div>{{.HTML}}</div>{{if .Article.Tags}}<p>标签：{{range $key,$label := .Article.Tags}} <a href="../{{$key}}.html">{{$label}}</a>{{end}}</p>{{end}}</article>` + footer))
var tagIndexTemplate = template.Must(template.New("tag-index").Parse(header + `<header><a href="./">{{.Site.Name}}</a></header><main><h1>标签</h1>{{range $key,$label:=.Site.Tags}}<p><a href="{{$key}}.html">{{$label}}</a></p>{{end}}</main>` + footer))
var tagPageTemplate = template.Must(template.New("tag-page").Funcs(template.FuncMap{"articleHref": relativeArticleLink}).Parse(header + `<header><a href="./">{{.Site.Name}}</a> · <a href="tags.html">标签</a></header><main><h1>{{.Label}}</h1>{{range .Articles}}<article><h2><a href="{{articleHref .}}">{{.Title}}</a></h2><time>{{.Created.Format "2006-01-02"}}</time></article>{{end}}</main>` + footer))

func Render(dir string, site store.Site, articles []store.Article) error {
	if !site.Imported {
		var err error
		site, err = normalizedSiteProfile(site)
		if err != nil {
			return err
		}
	}
	if !site.Imported {
		if err := validateArticleSettings(articles); err != nil {
			return err
		}
	}
	var err error
	site, err = prepareTags(site, articles)
	if err != nil {
		return err
	}
	site = store.ApplyPlausibleOverride(site)
	if site.PlausibleEnabled {
		origin, err := services.PlausibleServerOrigin(site.PlausibleAPIServer)
		if err != nil {
			return err
		}
		site.PlausibleAPIServer = strings.TrimPrefix(origin, "https://")
	}
	if !store.ValidPlausibleScriptID(site.PlausibleScriptID) {
		return fmt.Errorf("Plausible 脚本 ID 无效")
	}
	for _, article := range articles {
		if article.SiteID != site.ID || !safeID.MatchString(article.ID) {
			return fmt.Errorf("invalid article ID or site: %q", article.ID)
		}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if !site.Imported {
		if err := siteimage.WriteFiles(dir, site.AvatarPNG); err != nil {
			return fmt.Errorf("生成站点头像失败：%w", err)
		}
	}
	robots := ""
	if site.DoNotIndex {
		robots = "User-agent: *\nDisallow: /"
	}
	if err := os.WriteFile(filepath.Join(dir, "robots.txt"), []byte(robots), 0600); err != nil {
		return err
	}
	items := append([]store.Article{}, articles...)
	sort.Slice(items, func(i, j int) bool { return items[i].Created.After(items[j].Created) })
	type publicArticle struct {
		ID               string            `json:"id"`
		Link             string            `json:"link"`
		Slug             string            `json:"slug,omitempty"`
		ArticleType      int               `json:"articleType,omitempty"`
		ExternalLink     string            `json:"externalLink,omitempty"`
		Tags             map[string]string `json:"tags,omitempty"`
		Title            string            `json:"title"`
		Content          string            `json:"content"`
		ContentRendered  string            `json:"contentRendered"`
		Created          float64           `json:"created"`
		Modified         float64           `json:"modified"`
		Attachments      []string          `json:"attachments,omitempty"`
		ArticleNumber    *int              `json:"articleNumber,omitempty"`
		ArticleReference string            `json:"articleReference,omitempty"`
		HasAudio         bool              `json:"hasAudio"`
		HasVideo         bool              `json:"hasVideo"`
		AudioFilename    string            `json:"audioFilename,omitempty"`
		VideoFilename    string            `json:"videoFilename,omitempty"`
		AudioDuration    *int              `json:"audioDuration,omitempty"`
		AudioByteLength  *int64            `json:"audioByteLength,omitempty"`
	}
	seconds := func(t time.Time) float64 { return float64(t.Unix()-978307200) + float64(t.Nanosecond())/1e9 }
	metadata := struct {
		ID                 string              `json:"id"`
		Name               string              `json:"name"`
		About              string              `json:"about"`
		Slug               string              `json:"slug,omitempty"`
		AuthorName         string              `json:"authorName,omitempty"`
		Domain             string              `json:"domain,omitempty"`
		TwitterUsername    string              `json:"twitterUsername,omitempty"`
		GithubUsername     string              `json:"githubUsername,omitempty"`
		TelegramUsername   string              `json:"telegramUsername,omitempty"`
		MastodonUsername   string              `json:"mastodonUsername,omitempty"`
		DiscordLink        string              `json:"discordLink,omitempty"`
		PodcastLanguage    string              `json:"podcastLanguage,omitempty"`
		PodcastExplicit    bool                `json:"podcastExplicit,omitempty"`
		PodcastCategories  map[string][]string `json:"podcastCategories,omitempty"`
		IPNS               string              `json:"ipns"`
		Created            float64             `json:"created"`
		Updated            float64             `json:"updated"`
		PlausibleEnabled   bool                `json:"plausibleEnabled,omitempty"`
		PlausibleDomain    string              `json:"plausibleDomain,omitempty"`
		PlausibleAPIServer string              `json:"plausibleAPIServer,omitempty"`
		Tags               map[string]string   `json:"tags,omitempty"`
		Articles           []publicArticle     `json:"articles"`
	}{ID: site.ID, Name: site.Name, About: site.About, Slug: site.Slug, AuthorName: site.AuthorName, Domain: site.Domain, TwitterUsername: site.TwitterUsername, GithubUsername: site.GithubUsername, TelegramUsername: site.TelegramUsername, MastodonUsername: site.MastodonUsername, DiscordLink: site.DiscordLink, PodcastLanguage: site.PodcastLanguage, PodcastExplicit: site.PodcastExplicit, PodcastCategories: podcastCategoryMap(site.PodcastCategories), IPNS: site.IPNS, Created: seconds(site.Created), Updated: seconds(site.Created), PlausibleEnabled: site.PlausibleEnabled, PlausibleDomain: site.PlausibleDomain, PlausibleAPIServer: site.PlausibleAPIServer, Tags: site.Tags, Articles: []publicArticle{}}
	for _, article := range items {
		path := filepath.Join(dir, article.ID)
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
		html, err := preview.Markdown(article.Content)
		if err != nil {
			return err
		}
		public := publicArticle{ID: article.ID, Link: articleLink(article), Slug: article.Slug, ArticleType: article.ArticleType, ExternalLink: article.ExternalLink, Tags: article.Tags, Title: article.Title, Content: article.Content, ContentRendered: html, Created: seconds(article.Created), Modified: seconds(article.Modified), Attachments: article.Attachments, ArticleNumber: article.ArticleNumber, ArticleReference: article.ArticleReference, HasAudio: article.AudioFilename != "", HasVideo: article.VideoFilename != "", AudioFilename: article.AudioFilename, VideoFilename: article.VideoFilename, AudioDuration: article.AudioDuration, AudioByteLength: article.AudioByteLength}
		if article.ArticleType != 1 {
			metadata.Articles = append(metadata.Articles, public)
		}
		articleJSON, err := json.MarshalIndent(public, "", "  ")
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(path, "article.json"), articleJSON, 0600); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(path, "article.md"), []byte(article.Title+"\n\n"+article.Content), 0600); err != nil {
			return err
		}
		if seconds(article.Modified) > metadata.Updated {
			metadata.Updated = seconds(article.Modified)
		}
		f, err := os.Create(filepath.Join(path, "index.html"))
		if err != nil {
			return err
		}
		err = postTemplate.Execute(f, struct {
			Title      string
			Site       store.Site
			Article    store.Article
			HTML       template.HTML
			Navigation []navigationItem
			IconPrefix string
		}{article.Title, site, article, template.HTML(html), navigationItems(items, "../"), "../"})
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
	}
	encoded, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "planet.json"), encoded, 0600); err != nil {
		return err
	}
	type rssItem struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		GUID        string `xml:"guid"`
		Description string `xml:"description"`
		PubDate     string `xml:"pubDate"`
	}
	type rssChannel struct {
		Title       string    `xml:"title"`
		Link        string    `xml:"link"`
		Description string    `xml:"description"`
		Items       []rssItem `xml:"item"`
	}
	base := store.CanonicalSiteBase(site.Domain, site.IPNS)
	channel := rssChannel{Title: site.Name, Link: base, Description: site.About, Items: []rssItem{}}
	for _, a := range items {
		if a.ArticleType == 1 {
			continue
		}
		link := base + strings.TrimPrefix(articleLink(a), "/")
		channel.Items = append(channel.Items, rssItem{Title: a.Title, Link: link, GUID: link, Description: a.Content, PubDate: a.Created.UTC().Format(time.RFC1123Z)})
	}
	feed := struct {
		XMLName xml.Name   `xml:"rss"`
		Version string     `xml:"version,attr"`
		Channel rssChannel `xml:"channel"`
	}{Version: "2.0", Channel: channel}
	rss, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return err
	}
	rss = append([]byte(xml.Header), rss...)
	if err = os.WriteFile(filepath.Join(dir, "rss.xml"), rss, 0600); err != nil {
		return err
	}
	hasPodcast, err := renderPodcast(dir, site, items)
	if err != nil {
		return err
	}
	if len(site.Tags) > 0 {
		index, err := os.Create(filepath.Join(dir, "tags.html"))
		if err != nil {
			return err
		}
		err = tagIndexTemplate.Execute(index, struct {
			Title      string
			Site       store.Site
			IconPrefix string
		}{site.Name + " - 标签", site, ""})
		if closeErr := index.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		for key, label := range site.Tags {
			var tagged []store.Article
			for _, article := range items {
				if article.ArticleType == 1 {
					continue
				}
				if _, ok := article.Tags[key]; ok {
					tagged = append(tagged, article)
				}
			}
			page, err := os.Create(filepath.Join(dir, key+".html"))
			if err != nil {
				return err
			}
			err = tagPageTemplate.Execute(page, struct {
				Title      string
				Site       store.Site
				Label      string
				Articles   []store.Article
				IconPrefix string
			}{site.Name + " - " + label, site, label, tagged, ""})
			if closeErr := page.Close(); err == nil {
				err = closeErr
			}
			if err != nil {
				return err
			}
		}
	}
	f, err := os.Create(filepath.Join(dir, "index.html"))
	if err != nil {
		return err
	}
	blogs := make([]store.Article, 0, len(items))
	for _, article := range items {
		if article.ArticleType != 1 {
			blogs = append(blogs, article)
		}
	}
	err = indexTemplate.Execute(f, struct {
		Title      string
		Site       store.Site
		Articles   []store.Article
		Navigation []navigationItem
		IconPrefix string
		Social     []navigationItem
		HasPodcast bool
	}{site.Name, site, blogs, navigationItems(items, ""), "", socialLinks(site), hasPodcast})
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

func prepareTags(site store.Site, articles []store.Article) (store.Site, error) {
	seen := map[string]string{}
	if site.Tags == nil {
		site.Tags = map[string]string{}
	}
	validate := func(tags map[string]string) error {
		for key, label := range tags {
			if !tagname.Valid(key, label) {
				return fmt.Errorf("标签名称或路径无效")
			}
			folded := tagname.FoldKey(key)
			if old, ok := seen[folded]; ok && old != key {
				return fmt.Errorf("标签路径存在大小写冲突")
			}
			seen[folded] = key
		}
		return nil
	}
	if err := validate(site.Tags); err != nil {
		return site, err
	}
	for _, article := range articles {
		if err := validate(article.Tags); err != nil {
			return site, err
		}
		for key, label := range article.Tags {
			if _, ok := site.Tags[key]; !ok {
				site.Tags[key] = label
			}
		}
	}
	return site, nil
}
