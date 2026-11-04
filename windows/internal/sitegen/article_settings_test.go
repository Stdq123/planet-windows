package sitegen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"windows/internal/store"
)

func TestArticleSettingsRenderPageNavigationAndAliases(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	site := store.Site{ID: "SITE", Name: "Site"}
	articles := []store.Article{
		{ID: "BLOG", SiteID: "SITE", Title: "Blog post", Slug: "hello", Content: "Blog text", Created: now, Modified: now, IsIncludedInNavigation: true, NavigationWeight: 5},
		{ID: "PAGE", SiteID: "SITE", Title: "About page", Slug: "about", Content: "Page text", Created: now, Modified: now, ArticleType: 1, IsIncludedInNavigation: true, NavigationWeight: 1, ExternalLink: "https://example.com/about"},
	}
	if err := Render(dir, site, articles); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "BLOG", "image.png"), []byte("image"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "BLOG", "planet.key"), []byte("declared public attachment"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteAliases(dir, articles); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"BLOG/index.html", "hello/index.html", "hello/image.png", "hello/planet.key", "PAGE/index.html", "about/index.html"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "hello/") || !strings.Contains(string(index), "https://example.com/about") {
		t.Fatalf("links missing: %s", index)
	}
	page, err := os.ReadFile(filepath.Join(dir, "PAGE", "index.html"))
	if err != nil || !strings.Contains(string(page), `https://example.com/about`) || !strings.Contains(string(page), "location") {
		t.Fatalf("external link does not redirect: %s %v", page, err)
	}
	if strings.Index(string(index), "About page") > strings.Index(string(index), "Blog post") {
		t.Fatalf("navigation order incorrect: %s", index)
	}
	rss, err := os.ReadFile(filepath.Join(dir, "rss.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rss), "About page") || !strings.Contains(string(rss), "Blog post") {
		t.Fatalf("page must be absent from RSS: %s", rss)
	}
	var public struct {
		Articles []struct {
			ID string `json:"id"`
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "planet.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &public); err != nil {
		t.Fatal(err)
	}
	if len(public.Articles) != 1 || public.Articles[0].ID != "BLOG" {
		t.Fatalf("planet feed includes a page: %s", data)
	}
}

func TestRendererRejectsUnsafeOrCollidingSlugsBeforeWriting(t *testing.T) {
	for _, slug := range []string{"../evil", "assets", "OTHER"} {
		dir := t.TempDir()
		err := Render(dir, store.Site{ID: "S"}, []store.Article{{ID: "ONE", SiteID: "S", Slug: slug}, {ID: "OTHER", SiteID: "S"}})
		if err == nil {
			t.Errorf("accepted %q", slug)
		}
	}
}

func TestTemplateArticleSettingsExposeNavigationAndExcludePagesFromLists(t *testing.T) {
	templateDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(templateDir, "templates"), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"template.json": `{"name":"Test","generateArchive":true}`, "templates/index.html": `{% for n in site_navigation %}[{{ n.title }}:{{ n.slug }}:{{ n.externalLink }}:{{ n.weight }}]{% endfor %}|{% for a in articles %}{{ a.title }} {% endfor %}`, "templates/blog.html": `{{ article.title }} type={{ article_type }} link={{ article.externalLink }}`, "templates/archive.html": `{% for section in archive_sections %}{% for a in archive[section] %}{{ a.title }} {% endfor %}{% endfor %}`}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(templateDir, filepath.FromSlash(name)), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	articles := []store.Article{{ID: "POST", SiteID: "S", Title: "Blog", Created: time.Now(), IsIncludedInNavigation: true, NavigationWeight: 5}, {ID: "PAGE", SiteID: "S", Title: "About", Slug: "about", ArticleType: 1, ExternalLink: "https://example.com/", IsIncludedInNavigation: true, NavigationWeight: 1}}
	dir := t.TempDir()
	if err := RenderWithTemplate(dir, store.Site{ID: "S"}, articles, templateDir); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "[About:about:https://example.com/:1][Blog:POST::5]|Blog") {
		t.Fatalf("navigation or list incorrect: %s", index)
	}
	page, err := os.ReadFile(filepath.Join(dir, "PAGE", "index.html"))
	if err != nil || !strings.Contains(string(page), "type=1 link=https://example.com/") {
		t.Fatalf("page=%s, err=%v", page, err)
	}
	archive, err := os.ReadFile(filepath.Join(dir, "archive.html"))
	if err != nil || strings.Contains(string(archive), "About") {
		t.Fatalf("page in archive: %s, %v", archive, err)
	}
}

func TestVerifiedTemplateNavigationEscapesTitles(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("verified templates unavailable")
	}
	for _, name := range []string{"plain", "gamedb", "grid", "platinum", "sepia"} {
		t.Run(name, func(t *testing.T) {
			a := store.Article{ID: "POST", SiteID: "S", Title: `<img src=x onerror=alert(1)>`, IsIncludedInNavigation: true}
			dir := t.TempDir()
			if err := RenderWithTemplate(dir, store.Site{ID: "S", Name: "Site"}, []store.Article{a}, filepath.Join(root, name)); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(filepath.Join(dir, "index.html"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(b), a.Title) {
				t.Fatalf("unescaped title in navigation: %s", b)
			}
		})
	}
}

func TestVerifiedTemplatesRenderPageAndNavigation(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("verified templates unavailable")
	}
	for _, name := range []string{"plain", "gamedb", "grid", "platinum", "sepia"} {
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			articles := []store.Article{{ID: "POST", SiteID: "S", Title: "First story", Created: now, Slug: "story"}, {ID: "PAGE", SiteID: "S", Title: "About page", Created: now, Slug: "about", ArticleType: 1, IsIncludedInNavigation: true, NavigationWeight: 1}}
			dir := t.TempDir()
			if err := RenderWithTemplate(dir, store.Site{ID: "S", Name: "Site"}, articles, filepath.Join(root, name)); err != nil {
				t.Fatal(err)
			}
			if err := WriteAliases(dir, articles); err != nil {
				t.Fatal(err)
			}
			index, err := os.ReadFile(filepath.Join(dir, "index.html"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(index), "about/") || !strings.Contains(string(index), "About page") {
				t.Fatalf("navigation missing about page in %s", name)
			}
			if _, err := os.Stat(filepath.Join(dir, "about", "index.html")); err != nil {
				t.Fatal(err)
			}
			rss, err := os.ReadFile(filepath.Join(dir, "rss.xml"))
			if err != nil || strings.Contains(string(rss), "About page") || !strings.Contains(string(rss), "First story") {
				t.Fatalf("invalid RSS for %s: %v", name, err)
			}
		})
	}
}

func TestVerifiedTemplatesApplyExternalLinkRedirect(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("verified templates unavailable")
	}
	for _, name := range []string{"plain", "gamedb", "grid", "platinum", "sepia"} {
		t.Run(name, func(t *testing.T) {
			a := store.Article{ID: "POST", SiteID: "S", Title: "Link", ExternalLink: "https://example.com/target"}
			dir := t.TempDir()
			if err := RenderWithTemplate(dir, store.Site{ID: "S", Name: "Site"}, []store.Article{a}, filepath.Join(root, name)); err != nil {
				t.Fatal(err)
			}
			page, err := os.ReadFile(filepath.Join(dir, "POST", "index.html"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(page), "window.location.href") || !strings.Contains(string(page), "https://example.com/target") {
				t.Fatalf("redirect absent in %s", name)
			}
		})
	}
}
