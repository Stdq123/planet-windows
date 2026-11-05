package feeds

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchPreservesLargeFeedCompatibility(t *testing.T) {
	prolog := `<?xml version="1.0"?><!--` + strings.Repeat("x", 5000) + `-->`
	cases := map[string]string{
		"rss long prolog": prolog + `<rss version="2.0"><channel><title>Site</title><item><title>Post</title><guid>one</guid></item></channel></rss>`,
		"atom long prolog": prolog + `<feed xmlns="http://www.w3.org/2005/Atom"><title>Site</title><entry><title>Post</title><id>one</id></entry></feed>`,
		"large json": `{"version":"https://jsonfeed.org/version/1.1","title":"Site","items":[{"id":"one","title":"Post","content_text":"` + strings.Repeat("x", 5000) + `"}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			feed, err := Fetch(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			if feed.Title != "Site" || len(feed.Items) != 1 || feed.Items[0].Title != "Post" || feed.Items[0].ID != "one" {
				t.Fatalf("unexpected feed: %#v", feed)
			}
		})
	}
}

func TestFetchRejectsUnknownXMLAndOversizedFeed(t *testing.T) {
	cases := map[string]string{
		"unknown xml": `<?xml version="1.0"?><!--` + strings.Repeat("x", 5000) + `--><unknown><title>Site</title></unknown>`,
		"oversized rss": `<!--` + strings.Repeat("x", 5<<20) + `--><rss version="2.0"><channel><title>Site</title></channel></rss>`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			if _, err := Fetch(server.URL); err == nil {
				t.Fatal("accepted invalid feed")
			} else if name == "oversized rss" && !strings.Contains(err.Error(), "5 MB") {
				t.Fatalf("expected size limit error: %v", err)
			}
		})
	}
}

func TestFetchRSSAndAtom(t *testing.T) {
	cases := []struct{ name, body, title, item string }{
		{"rss", `<?xml version="1.0"?><rss version="2.0"><channel><title>RSS site</title><link>https://example.com/</link><description>Notes</description><item><title>Post one</title><link>https://example.com/1</link><guid>one</guid></item></channel></rss>`, "RSS site", "Post one"},
		{"atom", `<feed xmlns="http://www.w3.org/2005/Atom"><title>Atom site</title><id>https://example.com/</id><updated>2026-01-01T00:00:00Z</updated><entry><title>Post two</title><id>two</id><updated>2026-01-01T00:00:00Z</updated></entry></feed>`, "Atom site", "Post two"},
		{"json", `{"version":"https://jsonfeed.org/version/1.1","title":"JSON site","items":[{"id":"one","title":"Post three","content_text":"Hello"}]}`, "JSON site", "Post three"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.body)) }))
			defer server.Close()
			feed, err := Fetch(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			if feed.Title != tc.title || len(feed.Items) != 1 || feed.Items[0].Title != tc.item {
				t.Fatalf("unexpected feed: %#v", feed)
			}
		})
	}
}

func TestFetchRejectsUnsupportedScheme(t *testing.T) {
	if _, err := Fetch("file:///secret"); err == nil {
		t.Fatal("expected unsupported URL rejection")
	}
}

func TestJSONFeedTextIsNotInterpretedAsHTML(t *testing.T) {
	body := `{"version":"https://jsonfeed.org/version/1.1","title":"Feed","items":[{"id":"1","content_text":"1 < 2\n<example>"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer server.Close()
	feed, err := Fetch(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(feed.Items) != 1 || feed.Items[0].Content != "1 &lt; 2<br>\n&lt;example&gt;" {
		t.Fatalf("items = %#v", feed.Items)
	}
}

func TestFetchExposesAudioEnclosuresAndRejectsUnsafeURLs(t *testing.T) {
	body := `<rss version="2.0"><channel><title>Show</title><item><guid>1</guid><title>Episode</title><enclosure url="https://example.com/episode.mp3" type="audio/mpeg" length="123"/></item><item><guid>2</guid><title>Unsafe</title><enclosure url="file:///secret.mp3" type="audio/mpeg" length="1"/></item></channel></rss>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer server.Close()
	feed, err := Fetch(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(feed.Items) != 2 || feed.Items[0].AudioURL != "https://example.com/episode.mp3" || feed.Items[1].AudioURL != "" {
		t.Fatalf("items=%+v", feed.Items)
	}
}
