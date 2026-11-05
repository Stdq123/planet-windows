package templateinstall

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestPlatinumStylesheetAcceptsLineEndingsButRejectsContentChanges(t *testing.T) {
	for _, ending := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(ending.name, func(t *testing.T) {
			source, err := fs.Sub(bundled, "builtins/platinum")
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			if err := os.CopyFS(root, source); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "assets", "rss.xsl")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
			data = bytes.ReplaceAll(data, []byte("\n"), []byte(ending.value))
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := Verify(root); err != nil {
				t.Fatalf("valid %s stylesheet rejected: %v", ending.name, err)
			}
			if err := os.WriteFile(path, append(data, []byte("<!-- changed -->")...), 0600); err != nil {
				t.Fatal(err)
			}
			if err := Verify(root); err == nil {
				t.Fatal("modified stylesheet accepted")
			}
		})
	}
}
