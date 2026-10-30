package feeds

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
