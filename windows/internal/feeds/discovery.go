package feeds

import (
	"bytes"
	"mime"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// discoverFeeds follows explicit feed declarations in the document head only.
// Planet templates use rel=preload as=fetch in place of rel=alternate.
func discoverFeeds(body []byte, page *url.URL) []*url.URL {
	base := page
	baseSeen := false
	var links []string
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
			alternate, preload := false, false
			for _, rel := range strings.Fields(strings.ToLower(attrs["rel"])) {
				alternate = alternate || rel == "alternate"
				preload = preload || rel == "preload"
			}
			mediaType, _, err := mime.ParseMediaType(attrs["type"])
			if err != nil {
				continue
			}
			isFeed := mediaType == "application/rss+xml" || mediaType == "application/atom+xml" || mediaType == "application/feed+json"
			if isFeed && (alternate || (preload && strings.EqualFold(attrs["as"], "fetch"))) {
				links = append(links, attrs["href"])
			}
		}
	}
	var candidates []*url.URL
	seen := map[string]bool{}
	current := *page
	current.Fragment = ""
	seen[current.String()] = true
	for _, href := range links {
		resolved := resolveFeedURL(base, href)
		if resolved == nil || seen[resolved.String()] {
			continue
		}
		seen[resolved.String()] = true
		candidates = append(candidates, resolved)
		if len(candidates) == 5 {
			break
		}
	}
	return candidates
}

func resolveFeedURL(base *url.URL, href string) *url.URL {
	if href == "" {
		return nil
	}
	ref, err := url.Parse(href)
	if err != nil {
		return nil
	}
	resolved := base.ResolveReference(ref)
	if (resolved.Scheme != "http" && resolved.Scheme != "https") || resolved.Host == "" || resolved.User != nil {
		return nil
	}
	resolved.Fragment = ""
	return resolved
}
