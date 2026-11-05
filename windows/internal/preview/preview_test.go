package preview

import (
	"fmt"
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

func TestMarkdownRejectsEncodedAndAutomaticScriptURLs(t *testing.T) {
	for _, source := range []string{
		`[click](&#106;avascript:alert(1))`,
		`<javascript:alert(document.domain)>`,
		`![image](&#106;avascript:alert(1))`,
	} {
		t.Run(source, func(t *testing.T) {
			output, err := Markdown(source)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output, `href="javascript:`) || strings.Contains(output, `src="javascript:`) {
				t.Fatalf("active script URL reached the preview: %s", output)
			}
		})
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

func TestMarkdownPreviewEmbedsOnlyResolvedLocalImages(t *testing.T) {
	source := "![local](%E5%9B%BE%20%E7%89%87.png) ![remote](https://example.com/x.png) ![outside](../secret.png) ![inline](data:image/png;base64,AAAA)"
	output, err := MarkdownWithImages(source, func(name string) ([]byte, string, error) {
		if name == "图 片.png" {
			return []byte("image"), "image/png", nil
		}
		return nil, "", fmt.Errorf("unknown")
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, `src="data:image/png;base64,aW1hZ2U="`) {
		t.Fatalf("local image missing: %s", output)
	}
	if strings.Contains(output, "https://example.com") || strings.Contains(output, "../secret.png") || strings.Contains(output, "base64,AAAA") {
		t.Fatalf("untrusted images retained: %s", output)
	}
}

func TestHTMLPreviewEmbedsOnlyRegisteredImages(t *testing.T) {
	result, err := HTMLWithImages(`<p><img src="./picture.png"><img src="https://example.test/a.png"></p>`, func(name string) ([]byte, string, error) {
		if name == "picture.png" {
			return []byte("image"), "image/png", nil
		}
		return nil, "", fmt.Errorf("unknown")
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "data:image/png;base64,aW1hZ2U=") || strings.Contains(result, "https://example.test") {
		t.Fatalf("preview=%s", result)
	}
}
