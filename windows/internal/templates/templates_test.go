package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStencilSubsetIncludesAndFilters(t *testing.T) {
	previous := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = previous })
	dir := t.TempDir()
	files := map[string]string{"base.html": `<main>{% block body %}{% endblock %}</main>`, "index.html": `{% extends 'base.html' %}{% block body %}{% if article.title.count > 0 %}{{ article.title|escape }}{% endif %} {{ article.created|formatDateC }}{% endblock %}`}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	value, err := Render(dir, "index.html", map[string]any{"article": map[string]any{"title": "<Hello>", "created": time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(value, "&lt;Hello&gt;") || !strings.Contains(value, "2026-01-02T03:04:05Z") {
		t.Fatalf("render=%q", value)
	}
}

func TestTemplateCannotReadOutsideDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "templates")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(`{% include '../secret.txt' %}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Render(dir, "index.html", nil); err == nil {
		t.Fatal("template escaped root")
	}
}

func TestUnknownFilterFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(`{{ value|notSupported }}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Render(dir, "index.html", map[string]any{"value": "x"}); err == nil {
		t.Fatal("unsupported filter was accepted")
	}
}

func TestOriginalDateFiltersUseSystemTimeZone(t *testing.T) {
	previous := time.Local
	t.Cleanup(func() { time.Local = previous })
	dir := t.TempDir()
	// These filters are used by the original Plain, Paper and RSS templates.
	source := `{{ article.created|ymd }}|{{ article.created|formatDate }}|{{ article.created|formatDateC }}|{{ article.created|mdyydot }}|{{ article.created|rfc822 }}`
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	east := time.FixedZone("test-east", 8*60*60)
	west := time.FixedZone("test-west", -7*60*60)
	other := time.FixedZone("source-zone", 5*60*60)
	for _, tc := range []struct {
		name    string
		local   *time.Location
		created time.Time
		want    string
	}{
		{"east", east, time.Date(2025, time.December, 31, 20, 5, 6, 0, time.UTC), `Jan 1, 2026|Jan 1, 2026, 4:05:06 AM|2026-01-01T04:05:06+08:00|1.1.26|Thu, 01 Jan 2026 04:05:06 +0800`},
		{"source-zone-does-not-override-local", east, time.Date(2026, time.January, 1, 1, 5, 6, 0, other), `Jan 1, 2026|Jan 1, 2026, 4:05:06 AM|2026-01-01T04:05:06+08:00|1.1.26|Thu, 01 Jan 2026 04:05:06 +0800`},
		{"west", west, time.Date(2025, time.December, 31, 3, 5, 6, 0, time.UTC), `Dec 30, 2025|Dec 30, 2025, 8:05:06 PM|2025-12-30T20:05:06-07:00|12.30.25|Tue, 30 Dec 2025 20:05:06 -0700`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			time.Local = tc.local
			out, err := Render(dir, "index.html", map[string]any{"article": map[string]any{"created": tc.created}})
			if err != nil {
				t.Fatal(err)
			}
			if out != tc.want {
				t.Fatalf("got %q, want %q", out, tc.want)
			}
		})
	}
}

func TestOriginalPlainAndPaperRenderLocalArticleDate(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TAG_TEMPLATE_ROOT")
	if root == "" {
		t.Skip("set PLANET_TEST_TAG_TEMPLATE_ROOT to the original template Resources directory")
	}
	previous := time.Local
	time.Local = time.FixedZone("test-east", 8*60*60)
	t.Cleanup(func() { time.Local = previous })
	article := map[string]any{"id": "ARTICLE-1", "title": "Post", "link": "/ARTICLE-1/", "created": time.Date(2025, time.December, 31, 20, 5, 6, 0, time.UTC)}
	context := map[string]any{"planet": map[string]any{"name": "Site"}, "articles": []map[string]any{article}}
	for _, tc := range []struct{ name, want string }{
		{"plain", `<div class="item-date">Jan 1, 2026</div>`},
		{"paper", `<time datetime="2026-01-01T04:05:06+08:00" data-f="num">Jan 1, 2026</time>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Render(filepath.Join(root, tc.name, "templates"), "index.html", context)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("missing local date %q in %s", tc.want, out)
			}
		})
	}
}

func TestOriginalCreatedTimestampUsesDateAndRetainsFraction(t *testing.T) {
	dir := t.TempDir()
	// Sepia and Memories put this expression in data-article-created;
	// Croptop uses it to compute a JavaScript Date.
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(`{{ article.created.timeIntervalSince1970 }}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		article map[string]any
		want    string
	}{
		{"date only", map[string]any{"created": time.Unix(1767323045, 250000000)}, "1767323045.25"},
		{"date wins over legacy field", map[string]any{"created": time.Unix(1767323045, 250000000), "createdTimestamp": int64(1)}, "1767323045.25"},
		{"before epoch", map[string]any{"created": time.Unix(-1, 250000000)}, "-0.75"},
		{"legacy field", map[string]any{"createdTimestamp": int64(42)}, "42"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Render(dir, "index.html", map[string]any{"article": tc.article})
			if err != nil {
				t.Fatal(err)
			}
			if out != tc.want {
				t.Fatalf("got %q, want %q", out, tc.want)
			}
		})
	}
}

func TestOriginalCroptopBlogRendersArticleCreatedTimestamp(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TAG_TEMPLATE_ROOT")
	if root == "" {
		t.Skip("set PLANET_TEST_TAG_TEMPLATE_ROOT to the original template Resources directory")
	}
	context := map[string]any{
		"article": map[string]any{"id": "ARTICLE-1", "title": "Post", "created": time.Unix(1767323045, 250000000)},
		"planet":  map[string]any{"name": "Site"},
	}
	out, err := Render(filepath.Join(root, "croptop", "templates"), "blog.html", context)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `Math.floor(1767323045.25 * 1000)`) {
		t.Fatal("original Croptop blog did not render the correct timestamp")
	}
}

func TestOriginalItemTemplatesRenderArticleCreatedTimestamp(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TAG_TEMPLATE_ROOT")
	if root == "" {
		t.Skip("set PLANET_TEST_TAG_TEMPLATE_ROOT to the original template Resources directory")
	}
	for _, name := range []string{"sepia", "memories"} {
		t.Run(name, func(t *testing.T) {
			context := map[string]any{
				"article": map[string]any{
					"id": "ARTICLE-1", "title": "Post", "content": "Text",
					"created": time.Unix(1767323045, 250000000),
				},
			}
			out, err := Render(filepath.Join(root, name, "templates"), "modules/item.html", context)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, `data-article-created="1767323045.25"`) {
				t.Fatalf("incorrect timestamp in %s", out)
			}
		})
	}
}

func TestOriginalAbsoluteImageURLFilterKeepsExternalURLs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(`{{ content|absoluteImageURL:".",article.id }}`), 0600); err != nil {
		t.Fatal(err)
	}
	content := `<img src=" picture.png "><img src="https://example.com/a.png"><img src="/root.png"><img src="data:image/png;base64,AA"><img src="#x">`
	out, err := Render(dir, "index.html", map[string]any{"content": content, "article": map[string]any{"id": "ARTICLE-1"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`src="./ARTICLE-1/picture.png"`, `src="https://example.com/a.png"`, `src="/root.png"`, `src="data:image/png;base64,AA"`, `src="#x"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s in %s", want, out)
		}
	}
}

func TestAbsoluteImageFilterRewriteDoesNotAlterStringLiterals(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(`{{ "literal|absoluteImageURL:'.',article.id" }}`), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := Render(dir, "index.html", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != `literal|absoluteImageURL:'.',article.id` {
		t.Fatalf("literal changed: %q", out)
	}
}

func TestOriginalRSSMarkdownImagesUseSitePrefixForEachArticle(t *testing.T) {
	dir := t.TempDir()
	// This expression and loop are used by Planet/Templates/RSS.xml.
	template := `{% for article in planet.articles %}{{ article.content|md2html|absoluteImageURL:root_prefix,article.id }}{% endfor %}`
	if err := os.WriteFile(filepath.Join(dir, "rss.xml"), []byte(template), 0600); err != nil {
		t.Fatal(err)
	}
	context := map[string]any{
		"root_prefix": "https://example.com/ipns/site",
		"planet": map[string]any{"articles": []map[string]any{
			{"id": "ARTICLE-1", "content": "![one](one.png) ![external](https://other.example/image.png)"},
			{"id": "ARTICLE-2", "content": "![two](two.png)"},
		}},
	}
	out, err := Render(dir, "rss.xml", context)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`src="https://example.com/ipns/site/ARTICLE-1/one.png"`, `src="https://other.example/image.png"`, `src="https://example.com/ipns/site/ARTICLE-2/two.png"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s in %s", want, out)
		}
	}
}

func TestOriginalTagCountsRenderAndSelectPlural(t *testing.T) {
	dir := t.TempDir()
	// Platinum's tags.html uses the indexed count in both output and a condition.
	source := `{% for tag in tags %}{{ tag }}={{ tag_articles[tag].count }} post{% if tag_articles[tag].count != 1 %}s{% endif %};{% endfor %}{{ "tag_articles[tag].count" }}`
	if err := os.WriteFile(filepath.Join(dir, "tags.html"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	context := map[string]any{
		"tags":         []string{"one", "two", "empty"},
		"tag_articles": map[string][]string{"one": {"a"}, "two": {"a", "b"}, "empty": {}},
	}
	out, err := Render(dir, "tags.html", context)
	if err != nil {
		t.Fatal(err)
	}
	want := `one=1 post;two=2 posts;empty=0 posts;tag_articles[tag].count`
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestOriginalTagTemplatesRenderCounts(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TAG_TEMPLATE_ROOT")
	if root == "" {
		t.Skip("set PLANET_TEST_TAG_TEMPLATE_ROOT to the original template Resources directory")
	}
	context := map[string]any{
		"planet":       map[string]any{"name": "Example"},
		"tags":         map[string]string{"one": "One", "two": "Two"},
		"tag_articles": map[string][]string{"one": {"a"}, "two": {"a", "b"}},
	}
	for _, tc := range []struct {
		name  string
		wants []string
	}{
		{"platinum", []string{"1 post", "2 posts"}},
		{"paper", []string{`<span class="n">1</span>`, `<span class="n">2</span>`}},
		{"memories", []string{`<span class="tag-cloud-item-count">1</span>`, `<span class="tag-cloud-item-count">2</span>`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Render(filepath.Join(root, tc.name, "templates"), "tags.html", context)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.wants {
				if !strings.Contains(out, want) {
					t.Fatalf("missing %q in %s", want, out)
				}
			}
		})
	}
}

func TestOriginalRSSTemplateRenders(t *testing.T) {
	dir := os.Getenv("PLANET_TEST_RSS_TEMPLATE")
	if dir == "" {
		t.Skip("set PLANET_TEST_RSS_TEMPLATE to Planet's original Templates directory")
	}
	context := map[string]any{
		"root_prefix": "https://example.com/site",
		"planet": map[string]any{
			"name": "Example", "about": "About",
			"articles": []map[string]any{{
				"id": "ARTICLE-1", "title": "Post", "content": "![photo](photo.png)",
				"created": time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			}},
		},
	}
	out, err := Render(dir, "RSS.xml", context)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<title>Example</title>`, `<title>Post</title>`, `src="https://example.com/site/ARTICLE-1/photo.png"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s in %s", want, out)
		}
	}
}

func TestEscapeJSPreservesEmojiAndLiteralBackslash(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(`{{ value|escapejs }}`), 0600); err != nil {
		t.Fatal(err)
	}
	value, err := Render(dir, "index.html", map[string]any{"value": "😀\\n<"})
	if err != nil {
		t.Fatal(err)
	}
	if value != `😀\u005Cn\u003C` {
		t.Fatalf("got %q", value)
	}
}

func TestOriginalPlainTemplateRenders(t *testing.T) {
	dir := os.Getenv("PLANET_TEST_TEMPLATE")
	if dir == "" {
		t.Skip("set PLANET_TEST_TEMPLATE to original Plain templates directory")
	}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	article := map[string]any{"id": "A", "title": "Example", "link": "/A/", "content": "Body", "contentRendered": "<p>Body</p>", "created": now, "externalLink": "", "hasVideo": false, "hasAudio": false, "heroImage": nil}
	context := map[string]any{"planet": map[string]any{"name": "Test site", "about": "About", "ipns": "test"}, "articles": []map[string]any{article}, "article": article, "assets_prefix": "./", "page_title": "Test site", "content_html": "<p>Body</p>", "article_type": 0}
	index, err := Render(dir, "index.html", context)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(index, "Example") || !strings.Contains(index, "Test site") {
		t.Fatal("missing site or article in index")
	}
	context["assets_prefix"] = "../"
	post, err := Render(dir, "blog.html", context)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(post, "Example") || !strings.Contains(post, "<p>Body</p>") {
		t.Fatal("missing title or content in post")
	}
}
