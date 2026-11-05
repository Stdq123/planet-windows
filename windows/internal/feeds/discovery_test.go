package feeds

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const discoveryRSS = `<rss version="2.0"><channel><title>Discovered site</title><item><guid>one</guid><title>Post</title><enclosure url="audio.mp3" type="audio/mpeg" length="1"/></item></channel></rss>`

func TestFetchDiscoversAdvertisedFeeds(t *testing.T) {
	for _, tc := range []struct{ name, head, body, title string }{
		{"planet preload", `<link rel="preload" href="./planet.json" as="fetch" type="application/json"><link rel="preload" href="./rss.xml" as="fetch" type="application/rss+xml">`, discoveryRSS, "Discovered site"},
		{"RSS alternate", `<link rel="alternate" type="application/rss+xml" href="./rss.xml">`, discoveryRSS, "Discovered site"},
		{"Atom alternate", `<link rel="alternate" type="application/atom+xml" href="./rss.xml">`, `<feed xmlns="http://www.w3.org/2005/Atom"><title>Atom</title><id>site</id><entry><id>one</id><title>Post</title></entry></feed>`, "Atom"},
		{"JSON alternate", `<link rel="alternate" type="application/feed+json" href="./rss.xml">`, `{"version":"https://jsonfeed.org/version/1.1","title":"JSON","items":[{"id":"one","title":"Post","content_text":"Hello"}]}`, "JSON"},
		{"case and MIME parameters", `<LINK REL="ALTERNATE stylesheet" TYPE="Application/RSS+XML; charset=utf-8" HREF="./rss.xml">`, discoveryRSS, "Discovered site"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/":
					w.Header().Set("Content-Type", "text/html")
					fmt.Fprintf(w, "<!doctype html><html><head>%s</head><body>Home</body></html>", tc.head)
				case "/rss.xml":
					fmt.Fprint(w, tc.body)
				default:
					t.Errorf("unexpected request: %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			feed, err := Fetch(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			if feed.Title != tc.title || len(feed.Items) != 1 || feed.Items[0].Title != "Post" {
				t.Fatalf("feed = %+v", feed)
			}
		})
	}
}

func TestFetchDiscoveryResolvesRedirectsAndHTMLBase(t *testing.T) {
	for _, head := range []string{
		`<link rel="alternate" type="application/rss+xml" href="../feeds/rss.xml">`,
		`<link rel="alternate" type="application/rss+xml" href="rss.xml"><base href="../feeds/">`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/start":
				http.Redirect(w, r, "/blog/", http.StatusFound)
			case "/blog/":
				fmt.Fprintf(w, "<!doctype html><head>%s</head>", head)
			case "/feeds/rss.xml":
				http.Redirect(w, r, "/media/feed.xml", http.StatusFound)
			case "/media/feed.xml":
				fmt.Fprint(w, discoveryRSS)
			default:
				http.NotFound(w, r)
			}
		}))
		feed, err := Fetch(server.URL + "/start")
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(feed.Items) != 1 || feed.Items[0].AudioURL != server.URL+"/media/audio.mp3" {
			t.Fatalf("relative enclosure resolved against the wrong document: %+v", feed.Items)
		}
	}
}

func TestFetchDiscoverySkipsUnsafeDuplicateAndBrokenLinks(t *testing.T) {
	brokenRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			fmt.Fprintf(w, `<!doctype html><head>
			<link rel="alternate" type="application/rss+xml" href="file:///secret">
			<link rel="alternate" type="application/rss+xml" href="http://user:pass@%s/private">
			<link rel="alternate" type="application/rss+xml" href="#same-page">
			<link rel="alternate" type="application/rss+xml" href="/broken#one">
			<link rel="alternate" type="application/rss+xml" href="/broken#two">
			<link rel="alternate" type="application/rss+xml" href="/invalid">
			<link rel="alternate" type="application/rss+xml" href="/rss.xml">
			</head>`, r.Host)
		case "/broken":
			brokenRequests++
			http.NotFound(w, r)
		case "/invalid":
			fmt.Fprint(w, "not a feed")
		case "/rss.xml":
			fmt.Fprint(w, discoveryRSS)
		default:
			t.Errorf("unexpected request: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	feed, err := Fetch(server.URL + "/")
	if err != nil || feed.Title != "Discovered site" || brokenRequests != 1 {
		t.Fatalf("feed=%+v err=%v broken requests=%d", feed, err, brokenRequests)
	}
}

func TestFetchDiscoveryDoesNotRecursivelyCrawlPages(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprint(w, `<!doctype html><head><link rel="alternate" type="application/rss+xml" href="/another"></head>`)
	}))
	defer server.Close()
	if _, err := Fetch(server.URL); err == nil || requests != 2 {
		t.Fatalf("err=%v requests=%d", err, requests)
	}
}

func TestFetchPageWithoutFeedExplainsWhatAddressIsNeeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><html><head><title>No feeds</title></head><body>Home</body></html>`)
	}))
	defer server.Close()
	if _, err := Fetch(server.URL); err == nil || !strings.Contains(err.Error(), "订阅源地址") {
		t.Fatalf("expected actionable no-feed error, got %v", err)
	}
}
