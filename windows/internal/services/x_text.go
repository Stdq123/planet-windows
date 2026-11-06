package services

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// XPostText preserves the HTTPS article permalink, reserving its t.co weight of
// 23 and a separating newline. Emoji sequences are counted conservatively by
// code point, so complex emoji may cause an earlier (never later) truncation.
func XPostText(title, link string) (string, error) {
	if !utf8.ValidString(title) || !validXArticleURL(link) {
		return "", errors.New("X 发布内容或 HTTPS 文章链接无效")
	}
	title = norm.NFC.String(strings.Join(strings.Fields(title), " "))
	for _, ch := range title {
		if unicode.IsControl(ch) || ch == '\uFEFF' || ch == '\uFFFE' || ch == '\uFFFF' {
			return "", errors.New("X 文章标题包含无效字符")
		}
	}
	if title == "" {
		return link, nil
	}
	const budget = 280 - 23 - 1
	weight := 0
	for _, ch := range title {
		weight += xTitleRuneWeight(ch)
	}
	if weight > budget {
		// The ellipsis U+2026 has weight 2 under twitter-text v3.
		weight, end := 0, 0
		for index, ch := range title {
			cost := xTitleRuneWeight(ch)
			if weight+cost > budget-2 {
				break
			}
			weight += cost
			end = index + utf8.RuneLen(ch)
		}
		title = strings.TrimSpace(title[:end]) + "…"
	}
	return title + "\n" + link, nil
}

func validXArticleURL(link string) bool {
	// twitter-text refuses URL extraction beyond 4096 characters (including
	// scheme accounting). A 4000-byte ceiling is safely below that bound.
	if link == "" || len(link) > 4000 || !utf8.ValidString(link) || strings.ContainsAny(link, "\\\r\n") {
		return false
	}
	for _, ch := range link {
		if unicode.IsSpace(ch) || unicode.IsControl(ch) {
			return false
		}
	}
	u, err := url.ParseRequestURI(link)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Host == "" || strings.HasSuffix(u.Host, ":") {
		return false
	}
	host := u.Hostname()
	if len(host) > 253 || !strings.Contains(host, ".") || !plausibleServerHost.MatchString(host) {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	return true
}

func xTitleRuneWeight(ch rune) int {
	// A short domain embedded in a title can expand to 23 characters on X.
	// Reserve 23 extra for each possible domain separator instead of claiming
	// to implement twitter-text's complete, changing URL extraction grammar.
	if ch == '.' || ch == '\u3002' || ch == '\uFF0E' || ch == '\uFF61' {
		return 25
	}
	// © and ® are in the single-weight range but can be standalone emoji.
	if ch == '\u00A9' || ch == '\u00AE' {
		return 2
	}
	// Official twitter-text/config/v3.json single-weight Unicode ranges.
	if ch <= 4351 || ch >= 8192 && ch <= 8205 || ch >= 8208 && ch <= 8223 || ch >= 8242 && ch <= 8247 {
		return 1
	}
	return 2
}
