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
	"strings"
	"time"

	"github.com/flosch/pongo2/v6"
	"windows/internal/preview"
)

var countPattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_.]*)\.count\b`)
var nilPattern = regexp.MustCompile(`\bnil\b`)

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
				return pongo2.AsValue(date.Format(format)), nil
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
				segment = countPattern.ReplaceAllString(segment, "${1}|length")
				segment = nilPattern.ReplaceAllString(segment, "None")
				out.WriteString(segment)
				start = i
				quote = c
			}
		}
		segment := tag[start:]
		if quote == 0 {
			segment = countPattern.ReplaceAllString(segment, "${1}|length")
			segment = nilPattern.ReplaceAllString(segment, "None")
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
