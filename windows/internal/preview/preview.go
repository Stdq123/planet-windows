package preview

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

func Markdown(source string) (string, error) {
	var out bytes.Buffer
	if err := goldmark.New(goldmark.WithExtensions(extension.GFM, extension.Footnote), goldmark.WithRendererOptions(html.WithHardWraps())).Convert([]byte(source), &out); err != nil {
		return "", err
	}
	return out.String(), nil
}

// MarkdownWithImages replaces registered relative images with isolated data URLs.
func MarkdownWithImages(source string, resolve func(string) ([]byte, string, error)) (string, error) {
	rendered, err := Markdown(source)
	if err != nil {
		return "", err
	}
	return HTMLWithImages(rendered, resolve)
}

// HTMLWithImages restricts image sources in externally supplied HTML to resolved local files.
func HTMLWithImages(rendered string, resolve func(string) ([]byte, string, error)) (string, error) {
	return HTMLWithSources(rendered, func(raw string) ([]byte, string, error) {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, "", err
		}
		u.Path = strings.TrimPrefix(u.Path, "./")
		if u.IsAbs() || u.Host != "" || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(u.Path, `/\`) || u.Path == "" {
			return nil, "", fmt.Errorf("image path invalid")
		}
		return resolve(u.Path)
	})
}

// HTMLWithSources replaces every image source using the supplied offline resolver.
func HTMLWithSources(rendered string, resolve func(string) ([]byte, string, error)) (string, error) {
	context := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := xhtml.ParseFragment(strings.NewReader(rendered), context)
	if err != nil {
		return "", err
	}
	cache := map[string]string{}
	total := 0
	encode := func(raw string) string {
		if value, ok := cache[raw]; ok {
			return value
		}
		data, mime, err := resolve(raw)
		if err != nil || (mime != "image/png" && mime != "image/jpeg" && mime != "image/gif" && mime != "image/webp") || total+len(data) > 16<<20 {
			cache[raw] = ""
			return ""
		}
		total += len(data)
		value := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
		cache[raw] = value
		return value
	}
	var visit func(*xhtml.Node)
	visit = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && (node.Data == "img" || (node.Data == "source" && node.Parent != nil && node.Parent.Data == "picture")) {
			for i := range node.Attr {
				attr := &node.Attr[i]
				if attr.Key == "src" && node.Data == "img" {
					attr.Val = encode(attr.Val)
				}
				if attr.Key == "srcset" {
					raw := attr.Val
					attr.Val = ""
					if strings.Contains(strings.ToLower(raw), "data:") {
						continue
					}
					var entries []string
					for _, part := range strings.Split(raw, ",") {
						fields := strings.Fields(part)
						if len(fields) < 1 || len(fields) > 2 {
							continue
						}
						value := encode(fields[0])
						if value != "" {
							if len(fields) == 2 {
								value += " " + fields[1]
							}
							entries = append(entries, value)
						}
					}
					attr.Val = strings.Join(entries, ", ")
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}

	var result bytes.Buffer
	for _, node := range nodes {
		visit(node)
		if err := xhtml.Render(&result, node); err != nil {
			return "", err
		}
	}
	return result.String(), nil
}
