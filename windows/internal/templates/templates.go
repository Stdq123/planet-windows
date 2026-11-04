// Package templates supports a documented subset of Planet's Stencil syntax.
package templates

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/flosch/pongo2/v6"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"windows/internal/preview"
)

var countPattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_.]*)\.count\b`)
var indexedCountPattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_.]*\s*\[\s*[A-Za-z_][A-Za-z0-9_.]*\s*\])\.count\b`)
var nilPattern = regexp.MustCompile(`\bnil\b`)
var relativeImageFilter = regexp.MustCompile(`^(\{\{\s*[A-Za-z_][A-Za-z0-9_.]*\s*)\|absoluteImageURL\s*:\s*["']\.["']\s*,\s*article\.id(\s*\}\})$`)
var rssImageFilter = regexp.MustCompile(`^(\{\{\s*[A-Za-z_][A-Za-z0-9_.]*\s*\|md2html\s*)\|absoluteImageURL\s*:\s*root_prefix\s*,\s*article\.id(\s*\}\})$`)
var imageScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
var safeArticleID = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

type imageWithSitePrefix struct {
	body   string
	prefix string
}

func init() {
	// Stencil emits values verbatim unless the template asks for escaping.
	pongo2.SetAutoescape(false)
	if err := pongo2.ReplaceFilter("escapejs", func(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		var out strings.Builder
		for _, r := range in.String() {
			if r < 32 || strings.ContainsRune(`\'\"<>&=-;`, r) || r == '\u2028' || r == '\u2029' {
				fmt.Fprintf(&out, `\u%04X`, r)
			} else {
				out.WriteRune(r)
			}
		}
		return pongo2.AsValue(out.String()), nil
	}); err != nil {
		panic(err)
	}
	for name, layout := range map[string]string{"formatDate": "Jan 2, 2006, 3:04:05 PM", "formatDateC": time.RFC3339, "ymd": "Jan 2, 2006", "mdyydot": "1.2.06", "rfc822": time.RFC1123Z} {
		format := layout
		if err := pongo2.RegisterFilter(name, func(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
			if date, ok := in.Interface().(time.Time); ok {
				// Planet's DateFormatter uses the user's system time zone.
				return pongo2.AsValue(date.In(time.Local).Format(format)), nil
			}
			return in, nil
		}); err != nil {
			panic(err)
		}
	}
	if err := pongo2.RegisterFilter("md2html", func(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		html, err := preview.Markdown(in.String())
		if err != nil {
			return nil, &pongo2.Error{Sender: "md2html", OrigError: err}
		}
		return pongo2.AsValue(html), nil
	}); err != nil {
		panic(err)
	}
	if err := pongo2.RegisterFilter("hhmmss", func(in, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		n := in.Integer()
		return pongo2.AsValue(fmt.Sprintf("%02d:%02d:%02d", n/3600, (n%3600)/60, n%60)), nil
	}); err != nil {
		panic(err)
	}
	if err := pongo2.RegisterFilter("planetUnixTime", func(in, fallback *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		if date, ok := in.Interface().(time.Time); ok {
			seconds := float64(date.Unix()) + float64(date.Nanosecond())/1e9
			return pongo2.AsValue(strconv.FormatFloat(seconds, 'f', -1, 64)), nil
		}
		return fallback, nil
	}); err != nil {
		panic(err)
	}
	// Pongo filters accept one argument. Preserve the first Stencil argument
	// until the following absoluteImageURL filter receives the article ID.
	if err := pongo2.RegisterFilter("planetImageSitePrefix", func(in, arg *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		return pongo2.AsValue(imageWithSitePrefix{body: in.String(), prefix: arg.String()}), nil
	}); err != nil {
		panic(err)
	}
	if err := pongo2.RegisterFilter("absoluteImageURL", func(in, arg *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		id := arg.String()
		if !safeArticleID.MatchString(id) {
			return nil, &pongo2.Error{Sender: "absoluteImageURL", OrigError: errors.New("invalid article ID")}
		}
		content, prefix := in.String(), "."
		if value, ok := in.Interface().(imageWithSitePrefix); ok {
			content, prefix = value.body, value.prefix
		}
		body := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
		nodes, err := html.ParseFragment(strings.NewReader(content), body)
		if err != nil {
			return nil, &pongo2.Error{Sender: "absoluteImageURL", OrigError: err}
		}
		var visit func(*html.Node)
		visit = func(node *html.Node) {
			if node.Type == html.ElementNode && node.DataAtom == atom.Img {
				for i := range node.Attr {
					if node.Attr[i].Key != "src" {
						continue
					}
					src := strings.Trim(node.Attr[i].Val, " \t\n\r\f")
					if src != "" && !strings.ContainsAny(src[:1], "/?#") && !imageScheme.MatchString(src) {
						node.Attr[i].Val = prefix + "/" + id + "/" + src
					}
				}
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				visit(child)
			}
		}
		var out bytes.Buffer
		for _, node := range nodes {
			visit(node)
			if err := html.Render(&out, node); err != nil {
				return nil, &pongo2.Error{Sender: "absoluteImageURL", OrigError: err}
			}
		}
		return pongo2.AsValue(out.String()), nil
	}); err != nil {
		panic(err)
	}
}

type loader struct{ root string }

func (l loader) Abs(_ string, name string) string {
	if filepath.IsAbs(name) {
		return filepath.Clean(name)
	}
	return filepath.Join(l.root, filepath.FromSlash(name))
}
func (l loader) Get(path string) (io.Reader, error) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(l.root, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, errors.New("template path is outside its directory")
	}
	info, err := os.Stat(real)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4<<20 {
		return nil, errors.New("invalid or oversized template")
	}
	b, err := os.ReadFile(real)
	if err != nil {
		return nil, err
	}
	return bytes.NewBufferString(rewrite(string(b))), nil
}

// rewrite changes Stencil expressions only outside quoted strings inside tags.
func rewrite(source string) string {
	var out strings.Builder
	for len(source) > 0 {
		open := strings.Index(source, "{%")
		expression := strings.Index(source, "{{")
		if open < 0 || (expression >= 0 && expression < open) {
			open = expression
		}
		if open < 0 {
			out.WriteString(source)
			break
		}
		out.WriteString(source[:open])
		source = source[open:]
		closeTag := "%}"
		if strings.HasPrefix(source, "{{") {
			closeTag = "}}"
		}
		end := strings.Index(source, closeTag)
		if end < 0 {
			out.WriteString(source)
			break
		}
		tag := source[:end+2]
		tag = relativeImageFilter.ReplaceAllString(tag, "${1}|absoluteImageURL:article.id${2}")
		tag = rssImageFilter.ReplaceAllString(tag, "${1}|planetImageSitePrefix:root_prefix|absoluteImageURL:article.id${2}")
		// Sepia uses Stencil's comma as an AND in this condition.
		if tag == "{% if article.content.count == 0, article.tags.count > 0 %}" {
			tag = "{% if article.content.count == 0 and article.tags.count > 0 %}"
		}
		source = source[end+2:]
		start := 0
		var quote byte
		escaped := false
		for i := 0; i < len(tag); i++ {
			c := tag[i]
			if quote != 0 {
				if escaped {
					escaped = false
				} else if c == '\\' {
					escaped = true
				} else if c == quote {
					quote = 0
					out.WriteString(tag[start : i+1])
					start = i + 1
				}
				continue
			}
			if c == '\'' || c == '"' {
				segment := tag[start:i]
				segment = indexedCountPattern.ReplaceAllString(segment, "${1}|length")
				segment = countPattern.ReplaceAllString(segment, "${1}|length")
				segment = nilPattern.ReplaceAllString(segment, "None")
				segment = strings.ReplaceAll(segment, "article.created.timeIntervalSince1970", "article.created|planetUnixTime:article.createdTimestamp")
				out.WriteString(segment)
				start = i
				quote = c
			}
		}
		segment := tag[start:]
		if quote == 0 {
			segment = indexedCountPattern.ReplaceAllString(segment, "${1}|length")
			segment = countPattern.ReplaceAllString(segment, "${1}|length")
			segment = nilPattern.ReplaceAllString(segment, "None")
			segment = strings.ReplaceAll(segment, "article.created.timeIntervalSince1970", "article.created|planetUnixTime:article.createdTimestamp")
		}
		out.WriteString(segment)
	}
	return out.String()
}

func Render(dir, name string, context map[string]any) (string, error) {
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	set := pongo2.NewSet("planet", loader{root: root})
	// SSI can read arbitrary files through its own implementation, so it is unavailable.
	if err := set.BanTag("ssi"); err != nil {
		return "", err
	}
	tpl, err := set.FromFile(name)
	if err != nil {
		return "", err
	}
	return tpl.Execute(pongo2.Context(context))
}
