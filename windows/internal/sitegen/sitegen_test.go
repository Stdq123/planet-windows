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

	"golang.org/x/net/html"
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

func TestRenderAppliesPlausibleTrackingOverride(t *testing.T) {
	dir := t.TempDir()
	site := store.Site{ID: "SITE", Name: "Site", PlausibleOverride: &store.PlausibleTracking{Enabled: true, Domain: "example.com"}}
	if err := Render(dir, site, nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `data-domain="example.com"`) || !strings.Contains(string(b), `https://plausible.io/js/plausible.local.js`) {
		t.Fatalf("tracking missing: %s", b)
	}
}

func TestRenderUsesSiteSpecificPlausibleScript(t *testing.T) {
	dir := t.TempDir()
	site := store.Site{ID: "SITE", Name: "Site", PlausibleOverride: &store.PlausibleTracking{Enabled: true, Domain: "example.com", ScriptID: "pa-ABC_123"}}
	article := store.Article{ID: "ARTICLE", SiteID: "SITE", Title: "Post"}
	if err := Render(dir, site, []store.Article{article}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"index.html", "ARTICLE/index.html"} {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		page := string(b)
		if !strings.Contains(page, "https://plausible.io/js/pa-ABC_123.js") || !strings.Contains(page, "plausible.init()") || strings.Contains(page, "plausible.local.js") {
			t.Fatalf("incorrect tracking in %s: %s", path, page)
		}
	}
}

func TestRenderUsesValidatedSelfHostedPlausibleServer(t *testing.T) {
	dir := t.TempDir()
	site := store.Site{ID: "SITE", Name: "Site", PlausibleOverride: &store.PlausibleTracking{Enabled: true, Domain: "example.com", ScriptID: "pa-Test123", Server: "https://stats.example.com:8443"}}
	if err := Render(dir, site, nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "https://stats.example.com:8443/js/pa-Test123.js") {
		t.Fatalf("wrong host: %s", b)
	}
	site.PlausibleOverride.Server = "https://user:secret@evil.example/path"
	if err := Render(t.TempDir(), site, nil); err == nil {
		t.Fatal("unsafe server accepted")
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
	for _, name := range []string{"gamedb", "grid", "platinum"} {
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

func TestOriginalPlatinumTemplateRendersArticleAndArchive(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("original templates unavailable")
	}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	site := store.Site{ID: "SITE", Name: "Test site", About: "My description", Created: now}
	article := store.Article{ID: "POST", SiteID: site.ID, Title: "Example", Content: "**Body**", Created: now, Modified: now}
	out := t.TempDir()
	if err := RenderWithTemplate(out, site, []store.Article{article}, filepath.Join(root, "platinum")); err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{"index.html": "Example", "POST/index.html": "<strong>Body</strong>", "archive.html": "Example"} {
		data, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(file)))
		if err != nil || !strings.Contains(string(data), want) {
			t.Fatalf("%s missing %q: %v", file, want, err)
		}
	}
}

func TestOriginalSepiaTemplateRendersArticleMetadataAndArchive(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("original templates unavailable")
	}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	site := store.Site{ID: "SITE", Name: "Site", Created: now}
	article := store.Article{ID: "POST", SiteID: site.ID, Title: "Example", Content: "**Body**", Created: now, Modified: now, ArticleReference: "SITE-1", Attachments: []string{`quote"image.png`}}
	out := t.TempDir()
	if err := RenderWithTemplate(out, site, []store.Article{article}, filepath.Join(root, "sepia")); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Example", `data-article-created="1767323045"`, "SITE-1", `data-page-type="index"`} {
		if !strings.Contains(string(index), want) {
			t.Fatalf("index missing %q", want)
		}
	}
	if strings.Contains(string(index), `quote"image.png`) {
		t.Fatal("unsafe attachment name in HTML attribute")
	}
	page, err := os.ReadFile(filepath.Join(out, "POST", "index.html"))
	if err != nil || !strings.Contains(string(page), "<strong>Body</strong>") || !strings.Contains(string(page), `data-page-type="blog"`) {
		t.Fatalf("article page=%s err=%v", page, err)
	}
	archive, err := os.ReadFile(filepath.Join(out, "archive.html"))
	if err != nil || !strings.Contains(string(archive), "Example") {
		t.Fatalf("archive=%s err=%v", archive, err)
	}
}

func TestSepiaAttachmentDataRemainsValidJSONAndUsesURLSafeNames(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("original templates unavailable")
	}
	now := time.Now()
	site := store.Site{ID: "S", Name: "Site", Created: now}
	post := store.Article{ID: "P", SiteID: "S", Title: "Test", Created: now, Modified: now, Attachments: []string{"a,]# \".png"}}
	out := t.TempDir()
	if err := RenderWithTemplate(out, site, []store.Article{post}, filepath.Join(root, "sepia")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := html.Parse(strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	var data string
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		for _, a := range n.Attr {
			if a.Key == "data-attachments" {
				data = a.Val
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)
	var values []string
	if err := json.Unmarshal([]byte(strings.ReplaceAll(data, ",]", "]")), &values); err != nil || len(values) != 1 || values[0] != "a%2C%5D%23%20%22.png" {
		t.Fatalf("attachments=%q parsed=%v err=%v", data, values, err)
	}
}

func TestSepiaSocialImageUsesSiteAvatarAndArticleImage(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("original templates unavailable")
	}
	now := time.Now()
	site := store.Site{ID: "S", Name: "Site", IPNS: "k51test", Created: now}
	posts := []store.Article{{ID: "P", SiteID: "S", Title: "Image", Created: now, Modified: now, Attachments: []string{"hero #1.png"}}, {ID: "Q", SiteID: "S", Title: "Plain", Created: now, Modified: now}}
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "avatar.png"), []byte("avatar"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RenderWithTemplate(out, site, posts, filepath.Join(root, "sepia")); err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{"index.html": "https://k51test.eth.sucks/avatar.png", "P/index.html": "https://k51test.eth.sucks/P/hero%20%231.png", "Q/index.html": "https://k51test.eth.sucks/avatar.png"} {
		b, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(file)))
		if err != nil || !strings.Contains(string(b), want) {
			t.Fatalf("%s missing %q: %v", file, want, err)
		}
	}
}

func TestSepiaSettingsColorIconsAsWellAsText(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("original templates unavailable")
	}
	site := store.Site{ID: "S", Name: "Site", TemplateSettings: map[string]string{"highlightColor": "#FF0000", "foregroundSecondaryColor": "#123456"}}
	out := t.TempDir()
	if err := RenderWithTemplate(out, site, nil, filepath.Join(root, "sepia")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--icon-hover-filter: url(#planet-highlight-filter)", `id="planet-highlight-filter"`, `flood-color="#FF0000"`, `id="planet-secondary-filter"`, `flood-color="#123456"`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("missing %q", want)
		}
	}
}

func TestSepiaSettingsFileContainsOnlyEditableValues(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("original templates unavailable")
	}
	site := store.Site{ID: "S", Name: "Site", TemplateSettings: map[string]string{"backgroundColor": "#123456", "showRef": "true"}}
	out := t.TempDir()
	if err := RenderWithTemplate(out, site, nil, filepath.Join(root, "sepia")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "templateSettings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]string
	if err = json.Unmarshal(b, &values); err != nil {
		t.Fatal(err)
	}
	if values["backgroundColor"] != "#123456" || values["showRef"] != "true" || values["linkColor"] == "" {
		t.Fatalf("settings=%v", values)
	}
	if _, ok := values["highlightColorFilter"]; ok {
		t.Fatal("derived filter was exposed as editable setting")
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

func TestMediaFieldsReachPublicMetadataAndBasicPage(t *testing.T) {
	dir := t.TempDir()
	site := store.Site{ID: "site", Name: "Site", Created: time.Now()}
	duration := 12
	size := int64(1234)
	post := store.Article{ID: "post", SiteID: site.ID, Title: "Media", Created: time.Now(), Modified: time.Now(), AudioFilename: "audio.mp3", VideoFilename: "video.mp4", AudioDuration: &duration, AudioByteLength: &size, Attachments: []string{"audio.mp3", "video.mp4"}}
	if err := Render(dir, site, []store.Article{post}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "post", "article.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`"audioFilename": "audio.mp3"`, `"videoFilename": "video.mp4"`, `"audioDuration": 12`, `"audioByteLength": 1234`, `"hasAudio": true`, `"hasVideo": true`} {
		if !strings.Contains(string(b), value) {
			t.Fatalf("missing %s in %s", value, b)
		}
	}
	page, err := os.ReadFile(filepath.Join(dir, "post", "index.html"))
	if err != nil || !strings.Contains(string(page), "<audio") || !strings.Contains(string(page), "<video") {
		t.Fatalf("page=%s err=%v", page, err)
	}
}

func TestVerifiedTemplatesRenderAudioAndVideo(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("verified templates required")
	}
	site := store.Site{ID: "site", Name: "Site", Created: time.Now()}
	post := store.Article{ID: "post", SiteID: "site", Title: "Media", Created: time.Now(), Modified: time.Now(), AudioFilename: "sound #1.mp3", VideoFilename: "clip.mp4", Attachments: []string{"sound #1.mp3", "clip.mp4"}}
	for _, name := range []string{"plain", "gamedb", "grid"} {
		t.Run(name, func(t *testing.T) {
			out := t.TempDir()
			if err := RenderWithTemplate(out, site, []store.Article{post}, filepath.Join(root, name)); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(filepath.Join(out, "post", "index.html"))
			if err != nil || !strings.Contains(string(b), "sound%20%231.mp3") || !strings.Contains(string(b), "clip.mp4") {
				t.Fatalf("page=%s err=%v", b, err)
			}
		})
	}
}
