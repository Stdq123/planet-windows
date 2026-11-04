package sitegen

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"windows/internal/store"
)

func TestPodcastFeedContainsOnlyAudioBlogPosts(t *testing.T) {
	size := int64(1234)
	duration := 95
	site := store.Site{ID: "S", Name: "My Podcast", About: "Notes", Domain: "example.com", AuthorName: "Ada", PodcastLanguage: "zh-CN", PodcastExplicit: true, PodcastCategories: []string{"Technology"}}
	posts := []store.Article{{ID: "A", SiteID: "S", Title: "Episode", Created: time.Now(), AudioFilename: "my audio.mp3", AudioByteLength: &size, AudioDuration: &duration}, {ID: "PAGE", SiteID: "S", Title: "Not an episode", ArticleType: 1, AudioFilename: "page.mp3"}, {ID: "TEXT", SiteID: "S", Title: "Text"}}
	dir := t.TempDir()
	if err := Render(dir, site, posts); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "podcast.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err = xml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	for _, want := range []string{"Episode", "https://example.com/A/my%20audio.mp3", `length="1234"`, "Technology", "zh-CN", "00:01:35"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %s: %s", want, b)
		}
	}
	if strings.Contains(string(b), "Not an episode") || strings.Contains(string(b), ">Text<") {
		t.Fatalf("non-podcast item in feed: %s", b)
	}
}

func TestOriginalTemplatesExposePodcastFeed(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("verified templates unavailable")
	}
	for _, name := range []string{"plain", "sepia"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			site := store.Site{ID: "S", Name: "Podcast", IPNS: "k51test"}
			a := store.Article{ID: "A", SiteID: "S", Title: "Episode", AudioFilename: "episode.mp3"}
			if err := RenderWithTemplate(dir, site, []store.Article{a}, filepath.Join(root, name)); err != nil {
				t.Fatal(err)
			}
			page, err := os.ReadFile(filepath.Join(dir, "index.html"))
			if err != nil || !strings.Contains(string(page), "podcast.xml") {
				t.Fatalf("podcast link missing: %v", err)
			}
		})
	}
}
