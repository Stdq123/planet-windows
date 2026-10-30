package sitegen

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"windows/internal/store"
)

func TestRenderProducesSiteAndEscapesTitles(t *testing.T) {
	dir := t.TempDir()
	site := store.Site{ID: "SITE", Name: "A <site>", About: "Notes"}
	article := store.Article{ID: "ABC-123", SiteID: "SITE", Title: "<Unsafe>", Content: "Hello **world**"}
	if err := Render(dir, site, []store.Article{article}); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(dir, "ABC-123", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "A &lt;site&gt;") || !strings.Contains(string(index), "ABC-123/") {
		t.Fatalf("unexpected index: %s", index)
	}
	if !strings.Contains(string(page), "&lt;Unsafe&gt;") || !strings.Contains(string(page), "<strong>world</strong>") {
		t.Fatalf("unexpected page: %s", page)
	}
}

func TestPublicMetadataUsesPlanetDateEncoding(t *testing.T) {
	dir := t.TempDir()
	created := time.Date(2001, 1, 1, 0, 0, 10, 0, time.UTC)
	site := store.Site{ID: "S", Name: "Test", IPNS: "k51test", Created: created}
	article := store.Article{ID: "A", SiteID: "S", Title: "Post", Content: "text", Created: created, Modified: created}
	if err := Render(dir, site, []store.Article{article}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "planet.json"))
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		IPNS     string  `json:"ipns"`
		Created  float64 `json:"created"`
		Articles []struct {
			Link    string  `json:"link"`
			Created float64 `json:"created"`
		} `json:"articles"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatal(err)
	}
	if info.IPNS != "k51test" || info.Created != 10 || len(info.Articles) != 1 || info.Articles[0].Link != "/A/" || info.Articles[0].Created != 10 {
		t.Fatalf("info = %#v", info)
	}
}

func TestArticleFilesMatchPublishedContent(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2001, 1, 1, 0, 0, 10, 0, time.UTC)
	site := store.Site{ID: "S", Name: "Site", Created: now}
	article := store.Article{ID: "A", SiteID: "S", Title: "Title", Content: "Body", Created: now, Modified: now}
	if err := Render(dir, site, []store.Article{article}); err != nil {
		t.Fatal(err)
	}
	markdown, err := os.ReadFile(filepath.Join(dir, "A", "article.md"))
	if err != nil || string(markdown) != "Title\n\nBody" {
		t.Fatalf("markdown=%q err=%v", markdown, err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "A", "article.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		Title   string  `json:"title"`
		Content string  `json:"content"`
		Created float64 `json:"created"`
	}
	if err := json.Unmarshal(b, &metadata); err != nil || metadata.Title != "Title" || metadata.Content != "Body" || metadata.Created != 10 {
		t.Fatalf("metadata=%s err=%v", b, err)
	}
}

func TestRenderRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	err := Render(dir, store.Site{ID: "SITE"}, []store.Article{{ID: "../escape", SiteID: "SITE"}})
	if err == nil {
		t.Fatal("expected invalid article ID error")
	}
}

func TestRenderWithTemplateIncludesAssetsAndArticle(t *testing.T) {
	templateDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(templateDir, "templates"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(templateDir, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"template.json": `{"name":"Test"}`, "templates/index.html": `<h1>{{ planet.name|escape }}</h1>{% for article in articles %}{{ article.title|escape }}{% endfor %}`, "templates/blog.html": `<h1>{{ article.title|escape }}</h1>{{ content_html }}`, "assets/style.css": "body {}"}
	for name, value := range files {
		if err := os.WriteFile(filepath.Join(templateDir, filepath.FromSlash(name)), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	site := store.Site{ID: "SITE", Name: "My Site"}
	article := store.Article{ID: "POST", SiteID: "SITE", Title: "Hello", Content: "**World**"}
	if err := RenderWithTemplate(dir, site, []store.Article{article}, templateDir); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(dir, "POST", "index.html"))
	if err != nil || !strings.Contains(string(page), "<strong>World</strong>") {
		t.Fatalf("page=%q err=%v", page, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "style.css")); err != nil {
		t.Fatal(err)
	}
}

func TestOriginalPlainTemplateProducesWebsite(t *testing.T) {
	dir := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if dir == "" {
		t.Skip("original Plain template unavailable")
	}
	out := t.TempDir()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	site := store.Site{ID: "SITE", Name: "Test site", About: "About **Planet**", Created: now}
	article := store.Article{ID: "POST", SiteID: site.ID, Title: "Example", Content: "**Body**", Created: now, Modified: now}
	if err := RenderWithTemplate(out, site, []store.Article{article}, dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.html", "archive.html", "POST/index.html", "assets/style.css"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(name))); err != nil {
			t.Fatal(err)
		}
	}
	index, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil || !strings.Contains(string(index), "About <strong>Planet</strong>") {
		t.Fatalf("index omits about: %q %v", index, err)
	}
	b, err := os.ReadFile(filepath.Join(out, "POST", "index.html"))
	if err != nil || !strings.Contains(string(b), "<strong>Body</strong>") {
		t.Fatalf("article %q, err %v", b, err)
	}
	archive, err := os.ReadFile(filepath.Join(out, "archive.html"))
	if err != nil || !strings.Contains(string(archive), "Example") {
		t.Fatalf("archive missing post: %q, %v", archive, err)
	}
}

func TestOriginalPlainTemplatePagination(t *testing.T) {
	dir := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if dir == "" {
		t.Skip("original Plain template unavailable")
	}
	site := store.Site{ID: "S", Name: "Site"}
	now := time.Now()
	articles := []store.Article{}
	for i := 0; i < 11; i++ {
		articles = append(articles, store.Article{ID: fmt.Sprintf("POST-%d", i), SiteID: "S", Title: fmt.Sprintf("Post-%d", i), Created: now.Add(-time.Duration(i) * time.Hour)})
	}
	out := t.TempDir()
	if err := RenderWithTemplate(out, site, articles, dir); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(out, "page2.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), "page2.html") || strings.Contains(string(first), "Post-10") || !strings.Contains(string(second), "Post-10") {
		t.Fatal("Plain pagination is incorrect")
	}
}

func TestOriginalTextCompatibleTemplates(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("original template checkouts unavailable")
	}
	for _, name := range []string{"gamedb", "grid"} {
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			site := store.Site{ID: "SITE", Name: "Test site", About: "My description", Created: now}
			article := store.Article{ID: "POST", SiteID: site.ID, Title: "Example", Content: "**Body**", Created: now, Modified: now}
			out := t.TempDir()
			if err := RenderWithTemplate(out, site, []store.Article{article}, filepath.Join(root, name)); err != nil {
				t.Fatal(err)
			}
			for file, expected := range map[string]string{"index.html": "Example", "POST/index.html": "<strong>Body</strong>", "archive.html": "Example"} {
				data, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(file)))
				if err != nil || !strings.Contains(string(data), expected) {
					t.Fatalf("%s missing %q: %v", file, expected, err)
				}
			}
			index, err := os.ReadFile(filepath.Join(out, "index.html"))
			if err != nil || !strings.Contains(string(index), "My description") {
				t.Fatalf("missing site description: %v", err)
			}
			many := []store.Article{}
			for i := 0; i < 13; i++ {
				many = append(many, store.Article{ID: fmt.Sprintf("POST-%d", i), SiteID: site.ID, Title: fmt.Sprintf("Item-%d", i), Created: now.Add(-time.Duration(i) * time.Hour)})
			}
			pages := t.TempDir()
			if err := RenderWithTemplate(pages, site, many, filepath.Join(root, name)); err != nil {
				t.Fatal(err)
			}
			second, err := os.ReadFile(filepath.Join(pages, "page2.html"))
			if err != nil || !strings.Contains(string(second), "Item-12") {
				t.Fatalf("missing second page item: %v", err)
			}
		})
	}
}

func TestRenderProducesRSS(t *testing.T) {
	out := t.TempDir()
	site := store.Site{ID: "S", Name: "Example", IPNS: "k51test"}
	article := store.Article{ID: "A", SiteID: "S", Title: "Post", Content: "Hello"}
	if err := Render(out, site, []store.Article{article}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "rss.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var feed struct {
		Channel struct {
			Title string `xml:"title"`
			Items []struct {
				Title string `xml:"title"`
				Link  string `xml:"link"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(b, &feed); err != nil {
		t.Fatal(err)
	}
	if feed.Channel.Title != "Example" || len(feed.Channel.Items) != 1 || feed.Channel.Items[0].Link != "https://ipfs.io/ipns/k51test/A/" {
		t.Fatalf("feed=%#v", feed)
	}
}

func TestTemplatePaginationIncludesEveryArticle(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "templates"), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"template.json": `{"name":"Test","idealItemsPerPage":2,"generateIndexPagination":true}`, "templates/index.html": `{% for article in articles %}{{ article.title }};{% endfor %}{% if previous_page %}prev={{ previous_page }}{% endif %}{% if next_page %}next={{ next_page }}{% endif %}`, "templates/blog.html": `{{ content_html }}`}
	for name, value := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	site := store.Site{ID: "S", Name: "Site"}
	now := time.Now()
	articles := []store.Article{{ID: "A", SiteID: "S", Title: "One", Created: now}, {ID: "B", SiteID: "S", Title: "Two", Created: now.Add(-time.Hour)}, {ID: "C", SiteID: "S", Title: "Three", Created: now.Add(-2 * time.Hour)}}
	out := t.TempDir()
	if err := RenderWithTemplate(out, site, articles, root); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(out, "page2.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), "One;Two;") || strings.Contains(string(first), "Three;") || !strings.Contains(string(first), "next=page2.html") || !strings.Contains(string(second), "Three;") || !strings.Contains(string(second), "prev=page1.html") {
		t.Fatalf("first=%s second=%s", first, second)
	}
}

func TestPublicMetadataIncludesAttachments(t *testing.T) {
	dir := t.TempDir()
	site := store.Site{ID: "SITE", Name: "Site"}
	post := store.Article{ID: "POST", SiteID: site.ID, Title: "Post", Attachments: []string{"photo.png"}}
	if err := Render(dir, site, []store.Article{post}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"planet.json", filepath.Join("POST", "article.json")} {
		b, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `"attachments"`) || !strings.Contains(string(b), `"photo.png"`) {
			t.Fatalf("metadata missing attachments: %s", b)
		}
	}
}
