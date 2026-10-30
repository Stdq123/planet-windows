package preview

import (
	"strings"
	"testing"
)

func TestMarkdownAndUnsafeHTML(t *testing.T) {
	html, err := Markdown("# Hello\n\n**world**\n\n<script>alert(1)</script>")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<h1>Hello</h1>") || !strings.Contains(html, "<strong>world</strong>") {
		t.Fatalf("unexpected output: %s", html)
	}
	if strings.Contains(html, "<script>") {
		t.Fatalf("unsafe HTML was rendered: %s", html)
	}
}

func TestLineBreakMatchesPlanetRendering(t *testing.T) {
	html, err := Markdown("first\nsecond")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "first<br>\nsecond") {
		t.Fatalf("line break not rendered: %q", html)
	}
}

func TestFootnotesAreRendered(t *testing.T) {
	html, err := Markdown("Text[^note]\n\n[^note]: Footnote body")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "[^note]") || !strings.Contains(html, "Footnote body") || !strings.Contains(html, "footnote") {
		t.Fatalf("footnote not rendered: %q", html)
	}
}
