package sitegen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"windows/internal/store"
)

func TestArchiveGroupsArticlesByLocalMonthInDescendingOrder(t *testing.T) {
	previous := time.Local
	time.Local = time.FixedZone("test-east", 8*60*60)
	t.Cleanup(func() { time.Local = previous })
	root := t.TempDir()
	template := filepath.Join(root, "template")
	if err := os.MkdirAll(filepath.Join(template, "templates"), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"template.json":          `{"name":"Plain","generateArchive":true}`,
		"templates/index.html":   `index`,
		"templates/blog.html":    `post`,
		"templates/archive.html": `{% for item in archive_sections %}[{{ item }}:{% for article in archive[item] %}{{ article.title }},{% endfor %}]{% endfor %}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(template, filepath.FromSlash(name)), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	site := store.Site{ID: "SITE", Name: "Site"}
	articles := []store.Article{
		{ID: "JAN", SiteID: site.ID, Title: "January", Created: time.Date(2025, 12, 31, 16, 0, 0, 0, time.UTC)},
		{ID: "FEB2", SiteID: site.ID, Title: "February later", Created: time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC)},
		{ID: "DEC", SiteID: site.ID, Title: "December", Created: time.Date(2025, 12, 30, 20, 0, 0, 0, time.UTC)},
		{ID: "FEB1", SiteID: site.ID, Title: "February first", Created: time.Date(2026, 1, 31, 20, 0, 0, 0, time.UTC)},
	}
	out := filepath.Join(root, "output")
	if err := RenderWithTemplate(out, site, articles, template); err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(filepath.Join(out, "archive.html"))
	if err != nil {
		t.Fatal(err)
	}
	want := `[February 2026:February later,February first,][January 2026:January,][December 2025:December,]`
	if string(archive) != want {
		t.Fatalf("archive = %q, want %q", archive, want)
	}
}

func TestOriginalTemplatesRenderLocalMonthlyArchives(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("set PLANET_TEST_TEMPLATES_DIR to the original template Resources directory")
	}
	previous := time.Local
	time.Local = time.FixedZone("test-east", 8*60*60)
	t.Cleanup(func() { time.Local = previous })
	site := store.Site{ID: "SITE", Name: "Site"}
	articles := []store.Article{
		{ID: "JAN", SiteID: site.ID, Title: "JanuaryPost", Created: time.Date(2025, 12, 31, 16, 0, 0, 0, time.UTC)},
		{ID: "FEB", SiteID: site.ID, Title: "FebruaryPost", Created: time.Date(2026, 1, 31, 20, 0, 0, 0, time.UTC)},
	}
	// Paper is exercised as a rendering sample only; it is not on the install allowlist.
	for _, name := range []string{"plain", "gamedb", "grid", "platinum", "sepia", "paper"} {
		t.Run(name, func(t *testing.T) {
			out := t.TempDir()
			if err := RenderWithTemplate(out, site, articles, filepath.Join(root, name)); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(out, "archive.html"))
			if err != nil {
				t.Fatal(err)
			}
			page := string(data)
			last := -1
			for _, want := range []string{"February 2026", "FebruaryPost", "January 2026", "JanuaryPost"} {
				at := strings.Index(page, want)
				if at <= last {
					t.Fatalf("archive missing %q or has wrong grouping: %s", want, page)
				}
				last = at
			}
		})
	}
}
