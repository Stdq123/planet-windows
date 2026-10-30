package preview

import (
	"bytes"

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
