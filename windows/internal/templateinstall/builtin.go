package templateinstall

import (
	"embed"
	"encoding/base64"
	"fmt"
	"io/fs"
	"mime"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"windows/internal/sitegen"
	"windows/internal/store"
)

//go:embed builtins
var bundled embed.FS

type Builtin struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Build       int    `json:"build"`
}

func Builtins() []Builtin {
	return []Builtin{{"plain", "Plain", "简洁的文字博客", 132}, {"gamedb", "8-bit", "像素游戏风格", 50}, {"grid", "Grid", "适合图片的网格布局", 99}, {"platinum", "Platinum", "经典桌面窗口风格", 20}, {"sepia", "Sepia", "暖色调的微博客", 90}}
}

func ExtractBuiltin(id string) (string, func(), error) {
	valid := false
	for _, item := range Builtins() {
		if item.ID == id {
			valid = true
			break
		}
	}
	if !valid {
		return "", nil, fmt.Errorf("未知的内置模板")
	}
	root, err := os.MkdirTemp("", "planet-builtin-template-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	base := "builtins/" + id
	err = fs.WalkDir(bundled, base, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(name, base), "/")
		if rel == "" {
			return nil
		}
		target := filepath.Join(root, filepath.FromSlash(rel))
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		data, err := bundled.ReadFile(name)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0600)
	})
	if err == nil {
		err = Verify(root)
	}
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return root, cleanup, nil
}

var cssURL = regexp.MustCompile(`url\(\s*["']?([^"')]+)["']?\s*\)`)

// BuiltinPreview renders synthetic content, with local assets as data URLs.
// Scripts, network access and navigation are blocked by the caller's sandbox/CSP.
func BuiltinPreview(id string) (string, error) {
	source, cleanup, err := ExtractBuiltin(id)
	if err != nil {
		return "", err
	}
	defer cleanup()
	dir, err := os.MkdirTemp("", "planet-template-preview-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	site := store.Site{ID: "00000000-0000-4000-8000-000000000001", Name: "My Planet", About: "A little space on the web", Created: now}
	articles := []store.Article{{ID: "00000000-0000-4000-8000-000000000002", SiteID: site.ID, Title: "Hello, Planet", Content: "Welcome to my corner of the web. Here are some thoughts and stories.", Created: now, Modified: now}, {ID: "00000000-0000-4000-8000-000000000003", SiteID: site.ID, Title: "A new beginning", Content: "Notes, photographs, and things worth sharing.", Created: now.Add(-24 * time.Hour), Modified: now}}
	if err = sitegen.RenderWithTemplate(dir, site, articles, source); err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		return "", err
	}
	page := string(b)
	assetURL := func(name string) string {
		name = path.Clean(strings.TrimSpace(name))
		if strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
			return ""
		}
		data, e := os.ReadFile(filepath.Join(source, "assets", filepath.FromSlash(name)))
		if e != nil || len(data) > 1<<20 {
			return ""
		}
		typ := mime.TypeByExtension(path.Ext(name))
		if typ == "" {
			typ = "application/octet-stream"
		}
		return "data:" + typ + ";base64," + base64.StdEncoding.EncodeToString(data)
	}
	_ = filepath.WalkDir(filepath.Join(source, "assets"), func(file string, entry fs.DirEntry, e error) error {
		if e != nil || entry.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(filepath.Join(source, "assets"), file)
		if e != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if strings.ToLower(path.Ext(rel)) != ".css" {
			return nil
		}
		data, e := os.ReadFile(file)
		if e != nil {
			return nil
		}
		css := cssURL.ReplaceAllStringFunc(string(data), func(match string) string {
			parts := cssURL.FindStringSubmatch(match)
			if len(parts) < 2 {
				return "url()"
			}
			value := assetURL(path.Join(path.Dir(rel), parts[1]))
			if value == "" {
				return "url()"
			}
			return "url('" + value + "')"
		})
		url := "data:text/css;base64," + base64.StdEncoding.EncodeToString([]byte(css))
		pattern := regexp.MustCompile(`(?:\./)?assets/` + regexp.QuoteMeta(rel) + `(?:\?[^"'\s>]*)?`)
		page = pattern.ReplaceAllStringFunc(page, func(string) string { return url })
		return nil
	})
	_ = filepath.WalkDir(filepath.Join(source, "assets"), func(file string, entry fs.DirEntry, e error) error {
		if e != nil || entry.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(filepath.Join(source, "assets"), file)
		if e != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		ext := strings.ToLower(path.Ext(rel))
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" && ext != ".webp" && ext != ".svg" {
			return nil
		}
		value := assetURL(rel)
		if value == "" {
			return nil
		}
		pattern := regexp.MustCompile(`(?:\./)?assets/` + regexp.QuoteMeta(rel) + `(?:\?[^"'\s>]*)?`)
		page = pattern.ReplaceAllStringFunc(page, func(string) string { return value })
		return nil
	})
	// Keep the doctype first so the template is rendered in standards mode.
	csp := `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline' data:; font-src data:; base-uri 'none'; form-action 'none'">`
	head := regexp.MustCompile(`(?i)<head(?:\s[^>]*)?>`)
	if loc := head.FindStringIndex(page); loc != nil {
		page = page[:loc[1]] + csp + page[loc[1]:]
	} else {
		page = csp + page
	}
	return page, nil
}
