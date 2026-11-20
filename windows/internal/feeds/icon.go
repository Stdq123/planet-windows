package feeds

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/mmcdole/gofeed/atom"
	feedjson "github.com/mmcdole/gofeed/json"
	"golang.org/x/net/html"

	"windows/internal/siteimage"
)

const (
	maxIconFetchBytes = 2 << 20
	maxPageFetchBytes = 1 << 20
	maxIconAttempts   = 6
	maxIconCandidates = 6
	iconFetchTimeout  = 6 * time.Second
	iconHostDNSBudget = 3 * time.Second
)

type iconCandidate struct {
	url     *url.URL
	isApple bool
	isPNG   bool
	size    int
}

func extractFeedIconURLs(parsed *gofeed.Feed, base *url.URL) []*url.URL {
	var raw []string
	if orig, ok := parsed.OriginalFeed().(*feedjson.Feed); ok && orig != nil {
		raw = append(raw, orig.Icon, orig.Favicon)
	}
	if orig, ok := parsed.OriginalFeed().(*atom.Feed); ok && orig != nil {
		raw = append(raw, orig.Icon, orig.Logo)
	}
	if parsed.Image != nil {
		raw = append(raw, parsed.Image.URL)
	}
	if parsed.ITunesExt != nil {
		raw = append(raw, parsed.ITunesExt.Image)
	}
	var out []*url.URL
	seen := map[string]bool{}
	for _, value := range raw {
		resolved := resolveFeedURL(base, strings.TrimSpace(value))
		if resolved == nil || isSVGURL(resolved, "") {
			continue
		}
		key := resolved.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, resolved)
	}
	return out
}

// discoverIcons finds raster website icons declared in an HTML document head.
func discoverIcons(body []byte, page *url.URL) []*url.URL {
	base := page
	baseSeen := false
	var raw []iconCandidate
	tokens := html.NewTokenizer(bytes.NewReader(body))
scan:
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			break scan
		case html.EndTagToken:
			if tokens.Token().Data == "head" {
				break scan
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokens.Token()
			if token.Data == "body" {
				break scan
			}
			if token.Data != "link" && token.Data != "base" {
				continue
			}
			attrs := map[string]string{}
			for _, attr := range token.Attr {
				if _, exists := attrs[attr.Key]; !exists {
					attrs[attr.Key] = strings.TrimSpace(attr.Val)
				}
			}
			if token.Data == "base" {
				if href, exists := attrs["href"]; exists && !baseSeen {
					baseSeen = true
					if resolved := resolveFeedURL(page, href); resolved != nil {
						base = resolved
					}
				}
				continue
			}
			isApple, isIcon := false, false
			for _, rel := range strings.Fields(strings.ToLower(attrs["rel"])) {
				switch rel {
				case "apple-touch-icon", "apple-touch-icon-precomposed":
					isApple = true
				case "icon":
					isIcon = true
				}
			}
			if !isApple && !isIcon {
				continue
			}
			resolved := resolveFeedURL(base, attrs["href"])
			if resolved == nil || isSVGURL(resolved, attrs["type"]) {
				continue
			}
			mediaType, _, _ := mime.ParseMediaType(attrs["type"])
			ext := strings.ToLower(path.Ext(resolved.Path))
			isPNG := mediaType == "image/png" || ext == ".png" || (isApple && mediaType == "" && ext == "")
			raw = append(raw, iconCandidate{
				url:     resolved,
				isApple: isApple,
				isPNG:   isPNG,
				size:    parseIconSizes(attrs["sizes"]),
			})
		}
	}
	sort.SliceStable(raw, func(i, j int) bool {
		ri, rj := iconCandidateRank(raw[i]), iconCandidateRank(raw[j])
		if ri != rj {
			return ri > rj
		}
		if raw[i].size != raw[j].size {
			return raw[i].size > raw[j].size
		}
		if raw[i].isPNG != raw[j].isPNG {
			return raw[i].isPNG
		}
		return raw[i].isApple && !raw[j].isApple
	})
	var out []*url.URL
	seen := map[string]bool{}
	for _, entry := range raw {
		key := entry.url.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, entry.url)
		if len(out) >= maxIconCandidates {
			break
		}
	}
	return out
}

func iconCandidateRank(c iconCandidate) int {
	switch {
	case c.size >= 48 && c.size <= 256:
		return 5
	case c.isApple:
		return 4
	case c.size > 256:
		return 3
	case c.size == 0:
		return 2
	default:
		return 1
	}
}

func parseIconSizes(raw string) int {
	best := 0
	for _, field := range strings.Fields(strings.ToLower(raw)) {
		parts := strings.SplitN(field, "x", 2)
		if len(parts) != 2 {
			continue
		}
		w, errW := strconv.Atoi(parts[0])
		h, errH := strconv.Atoi(parts[1])
		if errW != nil || errH != nil || w <= 0 || h <= 0 {
			continue
		}
		side := w
		if h < side {
			side = h
		}
		if side > best {
			best = side
		}
	}
	return best
}

func isSVGURL(u *url.URL, rawType string) bool {
	if mediaType, _, err := mime.ParseMediaType(rawType); err == nil && mediaType == "image/svg+xml" {
		return true
	}
	return strings.EqualFold(path.Ext(u.Path), ".svg")
}

func fetchSiteIcon(sourceURL, feedURL *url.URL, siteLink string, feedIcons, pageIcons []*url.URL) string {
	ctx, cancel := context.WithTimeout(context.Background(), iconFetchTimeout)
	defer cancel()
	allowedHosts := map[string]bool{}
	if sourceURL != nil {
		allowedHosts[strings.ToLower(sourceURL.Hostname())] = true
	}
	if feedURL != nil {
		allowedHosts[strings.ToLower(feedURL.Hostname())] = true
	}
	client := &http.Client{
		Timeout: iconFetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many icon redirects")
			}
			return allowedIconURL(ctx, allowedHosts, req.URL)
		},
	}

	seen := map[string]bool{}
	attempts := 0
	tryList := func(urls []*url.URL) string {
		for _, target := range urls {
			if target == nil || ctx.Err() != nil || attempts >= maxIconAttempts {
				return ""
			}
			key := target.String()
			if seen[key] {
				continue
			}
			seen[key] = true
			if err := allowedIconURL(ctx, allowedHosts, target); err != nil {
				continue
			}
			attempts++
			data, err := fetchBounded(ctx, client, target, maxIconFetchBytes)
			if err != nil {
				continue
			}
			if encoded, err := siteimage.NormalizeIcon(data); err == nil && encoded != "" {
				return encoded
			}
		}
		return ""
	}

	if encoded := tryList(feedIcons); encoded != "" {
		return encoded
	}

	var homepage *url.URL
	if pageIcons != nil {
		homepage = sourceURL
		if encoded := tryList(pageIcons); encoded != "" {
			return encoded
		}
	} else {
		homepage = resolveSiteHomepage(feedURL, siteLink)
		if homepage != nil && !sameRequestURL(homepage, feedURL) {
			if err := allowedIconURL(ctx, allowedHosts, homepage); err == nil {
				if doc, err := fetchHTMLPage(ctx, client, homepage); err == nil && doc.isHTML {
					homepage = doc.url
					if encoded := tryList(discoverIcons(doc.body, doc.url)); encoded != "" {
						return encoded
					}
				}
			}
		}
	}

	var fallbacks []*url.URL
	for _, origin := range []*url.URL{homepage, sourceURL, feedURL} {
		if origin == nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" {
			continue
		}
		fallbacks = append(fallbacks, &url.URL{Scheme: origin.Scheme, Host: origin.Host, Path: "/favicon.ico"})
	}
	return tryList(fallbacks)
}

func resolveSiteHomepage(feedURL *url.URL, siteLink string) *url.URL {
	if feedURL != nil && strings.TrimSpace(siteLink) != "" {
		if resolved := resolveFeedURL(feedURL, strings.TrimSpace(siteLink)); resolved != nil {
			return resolved
		}
	}
	if feedURL != nil && (feedURL.Scheme == "http" || feedURL.Scheme == "https") && feedURL.Host != "" {
		return &url.URL{Scheme: feedURL.Scheme, Host: feedURL.Host, Path: "/"}
	}
	return nil
}

func sameRequestURL(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	pa, pb := a.Path, b.Path
	if pa == "" {
		pa = "/"
	}
	if pb == "" {
		pb = "/"
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host) && pa == pb && a.RawQuery == b.RawQuery
}

func fetchHTMLPage(ctx context.Context, client *http.Client, u *url.URL) (feedDocument, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return feedDocument{}, err
	}
	req.Header.Set("User-Agent", "Planet-Windows/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return feedDocument{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return feedDocument{}, errors.New("homepage request failed")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPageFetchBytes))
	if err != nil {
		return feedDocument{}, err
	}
	contentType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	isHTML := contentType == "text/html" || contentType == "application/xhtml+xml" || strings.HasPrefix(http.DetectContentType(body), "text/html")
	return feedDocument{body: body, url: resp.Request.URL, isHTML: isHTML}, nil
}

func fetchBounded(ctx context.Context, client *http.Client, u *url.URL, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Planet-Windows/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("icon request failed")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, errors.New("icon response too large")
	}
	return body, nil
}

func allowedIconURL(ctx context.Context, allowedHosts map[string]bool, target *url.URL) error {
	if target == nil || target.User != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		return errors.New("invalid icon url")
	}
	host := strings.ToLower(strings.Trim(target.Hostname(), "[]"))
	if host == "" {
		return errors.New("invalid icon host")
	}
	if allowedHosts[host] {
		return nil
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".test") || strings.HasSuffix(host, ".invalid") {
		return errors.New("non-public icon host")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return errors.New("private icon ip")
		}
		return nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, iconHostDNSBudget)
	defer cancel()
	records, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
	if err != nil || len(records) == 0 {
		return errors.New("icon host resolution failed")
	}
	for _, record := range records {
		if !isPublicIP(record.IP) {
			return errors.New("private icon ip")
		}
	}
	return nil
}

func isPublicIP(ip net.IP) bool {
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
}
