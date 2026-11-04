package store

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// CanonicalSiteBase is used in generated feeds. ENS names use a public HTTPS
// resolver; the application's gateway selector independently controls copies.
func CanonicalSiteBase(domain, ipns string) string {
	if ValidSiteDomain(domain) {
		switch {
		case strings.HasSuffix(domain, ".eth"):
			return "https://" + domain + ".limo/"
		case strings.HasSuffix(domain, ".sol"):
			return "https://" + domain + ".build/"
		case strings.HasSuffix(domain, ".bit"):
			return "https://" + domain + ".site/"
		default:
			return "https://" + domain + "/"
		}
	}
	if ipns != "" {
		return "https://ipfs.io/ipns/" + url.PathEscape(ipns) + "/"
	}
	return "/"
}

type SiteProfile struct {
	Domain           string `json:"domain"`
	TwitterUsername  string `json:"twitterUsername"`
	GithubUsername   string `json:"githubUsername"`
	TelegramUsername string `json:"telegramUsername"`
	MastodonUsername string `json:"mastodonUsername"`
	DiscordLink      string `json:"discordLink"`
}

var socialNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)
var mastodonNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)
var siteTLDPattern = regexp.MustCompile(`^[a-z]{2,63}$`)

// ValidSiteDomain accepts a DNS name without protocol, port or path.
func ValidSiteDomain(value string) bool {
	if value == "" || !strings.Contains(value, ".") || !ValidPlausibleDomain(value) {
		return false
	}
	parts := strings.Split(strings.ToLower(value), ".")
	return siteTLDPattern.MatchString(parts[len(parts)-1])
}

func NormalizeSiteProfile(p SiteProfile) (SiteProfile, error) {
	p.Domain = strings.ToLower(strings.TrimSpace(p.Domain))
	if p.Domain != "" && !ValidSiteDomain(p.Domain) {
		return p, errors.New("域名无效，请勿包含协议、端口或路径")
	}
	for _, value := range []*string{&p.TwitterUsername, &p.GithubUsername, &p.TelegramUsername} {
		*value = strings.TrimPrefix(strings.TrimSpace(*value), "@")
		if *value != "" && !socialNamePattern.MatchString(*value) {
			return p, errors.New("社交账号只能包含字母、数字、下划线或连字符")
		}
	}
	p.MastodonUsername = strings.TrimSpace(p.MastodonUsername)
	if p.MastodonUsername != "" {
		parts := strings.Split(strings.TrimPrefix(p.MastodonUsername, "@"), "@")
		if len(parts) != 2 || !mastodonNamePattern.MatchString(parts[0]) || !ValidSiteDomain(strings.ToLower(parts[1])) {
			return p, errors.New("Mastodon 账号格式应为 @用户@域名")
		}
		p.MastodonUsername = "@" + parts[0] + "@" + strings.ToLower(parts[1])
	}
	p.DiscordLink = strings.TrimSpace(p.DiscordLink)
	if p.DiscordLink != "" && (!strings.HasPrefix(p.DiscordLink, "https://") || !ValidArticleExternalLink(p.DiscordLink)) {
		return p, errors.New("Discord 链接必须是有效的 HTTPS 地址")
	}
	return p, nil
}

// UpdateSiteProfile saves public identity fields for a native site.
func (s *Store) UpdateSiteProfile(siteID string, p SiteProfile) (Site, error) {
	p, err := NormalizeSiteProfile(p)
	if err != nil {
		return Site{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Sites = append([]Site{}, s.data.Sites...)
	for i := range next.Sites {
		site := &next.Sites[i]
		if site.ID != siteID || site.Removed || site.DeletionPending {
			continue
		}
		if site.Imported {
			return Site{}, errors.New("导入站点暂不支持修改域名或社交资料")
		}
		if site.Domain == p.Domain && site.TwitterUsername == p.TwitterUsername && site.GithubUsername == p.GithubUsername && site.TelegramUsername == p.TelegramUsername && site.MastodonUsername == p.MastodonUsername && site.DiscordLink == p.DiscordLink {
			return cloneSite(*site), nil
		}
		if site.ContentRevision == ^uint64(0) {
			return Site{}, errors.New("站点修订版本已达到上限")
		}
		site.Domain, site.TwitterUsername, site.GithubUsername, site.TelegramUsername, site.MastodonUsername, site.DiscordLink = p.Domain, p.TwitterUsername, p.GithubUsername, p.TelegramUsername, p.MastodonUsername, p.DiscordLink
		site.ContentRevision++
		site.PublishPending = true
		if err = s.write(next); err != nil {
			return Site{}, err
		}
		return cloneSite(*site), nil
	}
	return Site{}, errors.New("站点不存在")
}
