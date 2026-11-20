package feeds

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"windows/internal/siteimage"
)

func testPNG(t *testing.T, size int, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func testICO(pngData []byte, side byte) []byte {
	header := make([]byte, 22)
	binary.LittleEndian.PutUint16(header[2:4], 1)
	binary.LittleEndian.PutUint16(header[4:6], 1)
	header[6], header[7] = side, side
	binary.LittleEndian.PutUint16(header[10:12], 1)
	binary.LittleEndian.PutUint16(header[12:14], 32)
	binary.LittleEndian.PutUint32(header[14:18], uint32(len(pngData)))
	binary.LittleEndian.PutUint32(header[18:22], 22)
	return append(header, pngData...)
}

func TestFetchExtractsIconFromFeedMetadata(t *testing.T) {
	icon := testPNG(t, 32, color.NRGBA{R: 20, G: 140, B: 220, A: 255})
	cases := []struct {
		name string
		body func(base string) string
	}{
		{
			name: "rss image",
			body: func(base string) string {
				return fmt.Sprintf(`<rss version="2.0"><channel><title>RSS</title><image><url>%s/logo.png</url></image><item><guid>1</guid><title>Post</title></item></channel></rss>`, base)
			},
		},
		{
			name: "atom icon",
			body: func(base string) string {
				return fmt.Sprintf(`<feed xmlns="http://www.w3.org/2005/Atom"><title>Atom</title><icon>%s/logo.png</icon><entry><id>1</id><title>Post</title></entry></feed>`, base)
			},
		},
		{
			name: "json feed icon",
			body: func(base string) string {
				return fmt.Sprintf(`{"version":"https://jsonfeed.org/version/1.1","title":"JSON","icon":"%s/logo.png","items":[{"id":"1","title":"Post","content_text":"Hi"}]}`, base)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/feed":
					fmt.Fprint(w, tc.body(server.URL))
				case "/logo.png":
					w.Header().Set("Content-Type", "image/png")
					_, _ = w.Write(icon)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			feed, err := Fetch(server.URL + "/feed")
			if err != nil {
				t.Fatal(err)
			}
			img, _, err := siteimage.DecodePNG(feed.IconPNG)
			if err != nil || img.Bounds().Dx() != 32 || img.Bounds().Dy() != 32 {
				t.Fatalf("expected 32x32 feed icon, got err=%v", err)
			}
		})
	}
}

func TestFetchDiscoversHomepageIconAndSkipsSVG(t *testing.T) {
	apple := testPNG(t, 64, color.NRGBA{R: 240, G: 90, B: 40, A: 255})
	small := testPNG(t, 16, color.NRGBA{R: 10, G: 10, B: 10, A: 255})
	svgRequested := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<!doctype html><html><head>
				<link rel="icon" type="image/svg+xml" href="/icon.svg">
				<link rel="icon" sizes="16x16" href="/small.png">
				<link rel="apple-touch-icon" sizes="64x64" href="/avatar.png">
				<link rel="alternate" type="application/rss+xml" href="/rss.xml">
			</head><body>Home</body></html>`)
		case "/rss.xml":
			fmt.Fprint(w, `<rss version="2.0"><channel><title>Site</title><item><guid>1</guid><title>Post</title></item></channel></rss>`)
		case "/icon.svg":
			svgRequested = true
			fmt.Fprint(w, `<svg></svg>`)
		case "/avatar.png":
			_, _ = w.Write(apple)
		case "/small.png":
			_, _ = w.Write(small)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	feed, err := Fetch(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if svgRequested {
		t.Fatal("expected SVG icon candidate to be skipped")
	}
	img, _, err := siteimage.DecodePNG(feed.IconPNG)
	if err != nil || img.Bounds().Dx() != 64 || img.Bounds().Dy() != 64 {
		t.Fatalf("expected 64x64 apple-touch-icon, got err=%v", err)
	}
}

func TestFetchDirectFeedResolvesHomepageAndFaviconICO(t *testing.T) {
	ico := testICO(testPNG(t, 32, color.NRGBA{R: 50, G: 190, B: 100, A: 255}), 32)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rss.xml":
			fmt.Fprintf(w, `<rss version="2.0"><channel><title>Blog</title><link>%s/</link><item><guid>1</guid><title>Post</title></item></channel></rss>`, server.URL)
		case "/":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<!doctype html><html><head><title>Blog</title></head><body>Hi</body></html>`)
		case "/favicon.ico":
			w.Header().Set("Content-Type", "image/x-icon")
			_, _ = w.Write(ico)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	feed, err := Fetch(server.URL + "/rss.xml")
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := siteimage.DecodePNG(feed.IconPNG)
	if err != nil || img.Bounds().Dx() != 32 || img.Bounds().Dy() != 32 {
		t.Fatalf("expected 32x32 favicon.ico, got err=%v", err)
	}
}

func TestFetchRejectsUnsafeCrossHostPrivateIconURLs(t *testing.T) {
	privateRequested := false
	privateServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		privateRequested = true
		_, _ = w.Write(testPNG(t, 32, color.NRGBA{R: 1, G: 2, B: 3, A: 255}))
	}))
	defer privateServer.Close()

	// Different loopback host ("localhost" vs "127.0.0.1") must be treated as cross-host non-public.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rss.xml" {
			fmt.Fprintf(w, `<rss version="2.0"><channel><title>Site</title><image><url>%s/icon.png</url></image><item><guid>1</guid><title>Post</title></item></channel></rss>`, "http://localhost"+privateServer.URL[len("http://127.0.0.1"):])
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	feed, err := Fetch(server.URL + "/rss.xml")
	if err != nil {
		t.Fatal(err)
	}
	if privateRequested || feed.IconPNG != "" {
		t.Fatalf("cross-host localhost icon should be blocked: requested=%v iconLen=%d", privateRequested, len(feed.IconPNG))
	}
}

func TestFetchFallsBackToAvatarPNGWhenNoIconDeclared(t *testing.T) {
	avatar := testPNG(t, 32, color.NRGBA{R: 30, G: 160, B: 210, A: 255})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<!doctype html><html><head><link rel="alternate" type="application/rss+xml" href="/rss.xml"></head><body>Home</body></html>`)
		case "/rss.xml":
			fmt.Fprint(w, `<rss version="2.0"><channel><title>Site</title><item><guid>1</guid><title>Post</title></item></channel></rss>`)
		case "/avatar.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(avatar)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	feed, err := Fetch(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := siteimage.DecodePNG(feed.IconPNG)
	if err != nil || img.Bounds().Dx() != 32 || img.Bounds().Dy() != 32 {
		t.Fatalf("expected 32x32 avatar.png fallback, got err=%v", err)
	}
}

func TestFetchDirectFeedFallsBackToAvatarPNGOverFavicon(t *testing.T) {
	avatar := testPNG(t, 48, color.NRGBA{R: 220, G: 80, B: 40, A: 255})
	ico := testICO(testPNG(t, 16, color.NRGBA{R: 10, G: 20, B: 30, A: 255}), 16)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rss.xml":
			fmt.Fprintf(w, `<rss version="2.0"><channel><title>Blog</title><link>%s/</link><item><guid>1</guid><title>Post</title></item></channel></rss>`, server.URL)
		case "/":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<!doctype html><html><head><title>Blog</title></head><body>Hi</body></html>`)
		case "/avatar.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(avatar)
		case "/favicon.ico":
			w.Header().Set("Content-Type", "image/x-icon")
			_, _ = w.Write(ico)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	feed, err := Fetch(server.URL + "/rss.xml")
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := siteimage.DecodePNG(feed.IconPNG)
	if err != nil || img.Bounds().Dx() != 48 || img.Bounds().Dy() != 48 {
		t.Fatalf("expected 48x48 avatar.png over favicon.ico, got err=%v", err)
	}
}

func TestFetchResolvesSubpathAvatarPNGForGatewayFeed(t *testing.T) {
	avatar := testPNG(t, 40, color.NRGBA{R: 70, G: 190, B: 110, A: 255})
	ico := testICO(testPNG(t, 16, color.NRGBA{R: 10, G: 20, B: 30, A: 255}), 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ipns/k51test/rss.xml":
			fmt.Fprint(w, `<rss version="2.0"><channel><title>Gateway Site</title><item><guid>1</guid><title>Post</title></item></channel></rss>`)
		case "/ipns/k51test/avatar.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(avatar)
		case "/favicon.ico":
			w.Header().Set("Content-Type", "image/x-icon")
			_, _ = w.Write(ico)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	feed, err := Fetch(server.URL + "/ipns/k51test/rss.xml")
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := siteimage.DecodePNG(feed.IconPNG)
	if err != nil || img.Bounds().Dx() != 40 || img.Bounds().Dy() != 40 {
		t.Fatalf("expected 40x40 subpath avatar.png, got err=%v", err)
	}
}

func TestFetchFallsBackToFeedDirectoryWhenSiteLinkFails(t *testing.T) {
	avatar := testPNG(t, 36, color.NRGBA{R: 140, G: 90, B: 230, A: 255})
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/blog/rss.xml":
			fmt.Fprintf(w, `<rss version="2.0"><channel><title>Blog</title><link>%s/missing/</link><item><guid>1</guid><title>Post</title></item></channel></rss>`, server.URL)
		case "/blog/avatar.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(avatar)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	feed, err := Fetch(server.URL + "/blog/rss.xml")
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := siteimage.DecodePNG(feed.IconPNG)
	if err != nil || img.Bounds().Dx() != 36 || img.Bounds().Dy() != 36 {
		t.Fatalf("expected 36x36 /blog/avatar.png, got err=%v", err)
	}
}
