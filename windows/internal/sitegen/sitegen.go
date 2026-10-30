// Package sitegen builds a basic static site from the local library.
package sitegen

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"windows/internal/preview"
	"windows/internal/store"
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

const header = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Title}}</title><style>body{max-width:740px;margin:70px auto;padding:0 24px;font:17px/1.8 system-ui,sans-serif;color:#262b32}h1,h2{line-height:1.25}a{color:#52649c}article{margin:48px 0}footer{margin-top:80px;color:#888;font-size:13px}pre{overflow:auto;background:#f5f5f5;padding:16px}img{max-width:100%}</style></head><body>`
const footer = `<footer>Published with Planet</footer></body></html>`

var indexTemplate = template.Must(template.New("index").Parse(header + `<header><h1>{{.Site.Name}}</h1><p>{{.Site.About}}</p></header><main>{{range .Articles}}<article><h2><a href="{{.ID}}/">{{.Title}}</a></h2><time>{{.Created.Format "2006-01-02"}}</time></article>{{end}}</main>` + footer))
var postTemplate = template.Must(template.New("post").Parse(header + `<header><a href="../">{{.Site.Name}}</a></header><article><h1>{{.Article.Title}}</h1><time>{{.Article.Created.Format "2006-01-02"}}</time><div>{{.HTML}}</div></article>` + footer))

func Render(dir string, site store.Site, articles []store.Article) error {
	for _, article := range articles {
		if article.SiteID != site.ID || !safeID.MatchString(article.ID) {
			return fmt.Errorf("invalid article ID or site: %q", article.ID)
		}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	items := append([]store.Article{}, articles...)
	sort.Slice(items, func(i, j int) bool { return items[i].Created.After(items[j].Created) })
	type publicArticle struct {
		ID               string   `json:"id"`
		Link             string   `json:"link"`
		Title            string   `json:"title"`
		Content          string   `json:"content"`
		ContentRendered  string   `json:"contentRendered"`
		Created          float64  `json:"created"`
		Modified         float64  `json:"modified"`
		Attachments      []string `json:"attachments,omitempty"`
		ArticleNumber    *int     `json:"articleNumber,omitempty"`
		ArticleReference string   `json:"articleReference,omitempty"`
	}
	seconds := func(t time.Time) float64 { return float64(t.Unix()-978307200) + float64(t.Nanosecond())/1e9 }
	metadata := struct {
		ID       string          `json:"id"`
		Name     string          `json:"name"`
		About    string          `json:"about"`
		IPNS     string          `json:"ipns"`
		Created  float64         `json:"created"`
		Updated  float64         `json:"updated"`
		Articles []publicArticle `json:"articles"`
	}{ID: site.ID, Name: site.Name, About: site.About, IPNS: site.IPNS, Created: seconds(site.Created), Updated: seconds(site.Created), Articles: []publicArticle{}}
	for _, article := range items {
		path := filepath.Join(dir, article.ID)
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
		html, err := preview.Markdown(article.Content)
		if err != nil {
			return err
		}
		public := publicArticle{ID: article.ID, Link: "/" + article.ID + "/", Title: article.Title, Content: article.Content, ContentRendered: html, Created: seconds(article.Created), Modified: seconds(article.Modified), Attachments: article.Attachments, ArticleNumber: article.ArticleNumber, ArticleReference: article.ArticleReference}
		metadata.Articles = append(metadata.Articles, public)
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
			Title   string
			Site    store.Site
			Article store.Article
			HTML    template.HTML
		}{article.Title, site, article, template.HTML(html)})
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
	base := "/"
	if site.IPNS != "" {
		base = "https://ipfs.io/ipns/" + url.PathEscape(site.IPNS) + "/"
	}
	channel := rssChannel{Title: site.Name, Link: base, Description: site.About, Items: []rssItem{}}
	for _, a := range items {
		link := base + a.ID + "/"
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
	f, err := os.Create(filepath.Join(dir, "index.html"))
	if err != nil {
		return err
	}
	err = indexTemplate.Execute(f, struct {
		Title    string
		Site     store.Site
		Articles []store.Article
	}{site.Name, site, items})
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}
