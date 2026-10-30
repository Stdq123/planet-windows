package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStencilSubsetIncludesAndFilters(t *testing.T) {
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
