package sitegen

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"windows/internal/backup"
	"windows/internal/store"
)

func relativeArticleLink(a store.Article) string { return strings.TrimPrefix(articleLink(a), "/") }

type navigationItem struct {
	ID, Title, Slug, ExternalLink, Href string
	Weight                              int
}

func normalizedSiteProfile(site store.Site) (store.Site, error) {
	p, err := store.NormalizeSiteProfile(store.SiteProfile{Domain: site.Domain, TwitterUsername: site.TwitterUsername, GithubUsername: site.GithubUsername, TelegramUsername: site.TelegramUsername, MastodonUsername: site.MastodonUsername, DiscordLink: site.DiscordLink})
	if err != nil {
		return site, err
	}
	site.Domain, site.TwitterUsername, site.GithubUsername, site.TelegramUsername, site.MastodonUsername, site.DiscordLink = p.Domain, p.TwitterUsername, p.GithubUsername, p.TelegramUsername, p.MastodonUsername, p.DiscordLink
	return site, nil
}

func socialLinks(site store.Site) []navigationItem {
	links := []navigationItem{}
	add := func(title, href string) {
		if href != "" {
			links = append(links, navigationItem{Title: title, Href: href})
		}
	}
	if site.TwitterUsername != "" {
		add("Twitter", "https://twitter.com/"+site.TwitterUsername)
	}
	if site.GithubUsername != "" {
		add("GitHub", "https://github.com/"+site.GithubUsername)
	}
	if site.TelegramUsername != "" {
		add("Telegram", "https://t.me/"+site.TelegramUsername)
	}
	if site.MastodonUsername != "" {
		parts := strings.Split(strings.TrimPrefix(site.MastodonUsername, "@"), "@")
		if len(parts) == 2 {
			add("Mastodon", "https://"+parts[1]+"/@"+parts[0])
		}
	}
	add("Discord", site.DiscordLink)
	return links
}

func navigationItems(articles []store.Article, prefix string) []navigationItem {
	out := []navigationItem{}
	for _, a := range articles {
		if !a.IsIncludedInNavigation {
			continue
		}
		slug := a.Slug
		if slug == "" {
			slug = a.ID
		}
		href := prefix + slug + "/"
		if a.ExternalLink != "" {
			href = a.ExternalLink
		}
		out = append(out, navigationItem{ID: a.ID, Title: a.Title, Slug: slug, ExternalLink: a.ExternalLink, Weight: a.NavigationWeight, Href: href})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Weight != out[j].Weight {
			return out[i].Weight < out[j].Weight
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func validateArticleSettings(articles []store.Article) error {
	paths := map[string]bool{"assets": true, "tags": true}
	for _, a := range articles {
		key := strings.ToLower(a.ID)
		if !safeID.MatchString(a.ID) || paths[key] {
			return fmt.Errorf("文章路径无效或冲突：%q", a.ID)
		}
		paths[key] = true
	}
	for _, a := range articles {
		if a.ArticleType != 0 && a.ArticleType != 1 {
			return fmt.Errorf("文章类型无效")
		}
		if !store.ValidArticleExternalLink(a.ExternalLink) {
			return fmt.Errorf("文章外部链接无效")
		}
		if a.Slug == "" {
			continue
		}
		key := strings.ToLower(a.Slug)
		if !store.ValidArticleSlug(a.Slug) || paths[key] {
			return fmt.Errorf("文章自定义路径无效或冲突：%q", a.Slug)
		}
		paths[key] = true
	}
	return nil
}

// WriteAliases is called after all article attachments and thumbnails are staged.
func WriteAliases(dir string, articles []store.Article) error {
	if err := validateArticleSettings(articles); err != nil {
		return err
	}
	for _, a := range articles {
		if a.Slug == "" {
			continue
		}
		dst := filepath.Join(dir, a.Slug)
		if err := os.Mkdir(dst, 0700); err != nil {
			return err
		}
		// This source is the already-generated public article directory, not a
		// private backup root. A user-declared attachment may be named planet.key.
		if err := backup.CopyWithKey(filepath.Join(dir, a.ID), dst); err != nil {
			return err
		}
	}
	return nil
}
