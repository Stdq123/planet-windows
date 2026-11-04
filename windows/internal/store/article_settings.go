package store

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// UnmarshalJSON preserves the original default for pre-settings libraries,
// while keeping an explicitly saved zero navigation weight.
func (a *Article) UnmarshalJSON(data []byte) error {
	type plainArticle Article
	var value plainArticle
	value.NavigationWeight = 1
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*a = Article(value)
	return nil
}

var articleSlugPattern = regexp.MustCompile(`^[a-z0-9-]{1,120}$`)

// ValidArticleSlug excludes path separators, Windows device names and generated directories.
func ValidArticleSlug(slug string) bool {
	if !articleSlugPattern.MatchString(slug) {
		return false
	}
	upper := strings.ToUpper(slug)
	if upper == "ASSETS" || upper == "TAGS" || upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL" {
		return false
	}
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) && upper[3] >= '1' && upper[3] <= '9' {
		return false
	}
	return true
}

func ValidArticleExternalLink(link string) bool {
	if link == "" {
		return true
	}
	// Verified original templates interpolate links directly into both HTML
	// attributes and JavaScript strings. Require those characters to be encoded.
	if len(link) > 4096 || strings.ContainsAny(link, "\r\n\t \"'<>\\\u2028\u2029") {
		return false
	}
	u, err := url.Parse(link)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil
}

// UpdateArticleSettings changes a local article and queues a fresh publication.
func (s *Store) UpdateArticleSettings(siteID, articleID, slug string, articleType int, externalLink string, inNavigation bool, weight int) (Article, error) {
	slug = strings.TrimSpace(slug)
	externalLink = strings.TrimSpace(externalLink)
	if slug != "" && !ValidArticleSlug(slug) {
		return Article{}, errors.New("自定义路径只能包含小写字母、数字或连字符，且不能使用保留名称")
	}
	if articleType != 0 && articleType != 1 {
		return Article{}, errors.New("文章类型无效")
	}
	if !ValidArticleExternalLink(externalLink) {
		return Article{}, errors.New("外部链接必须是有效的 HTTP 或 HTTPS 地址")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var site *Site
	for i := range s.data.Sites {
		if s.data.Sites[i].ID == siteID && !s.data.Sites[i].Removed && !s.data.Sites[i].DeletionPending {
			site = &s.data.Sites[i]
			break
		}
	}
	if site == nil {
		return Article{}, errors.New("站点不存在")
	}
	if site.Imported {
		return Article{}, errors.New("导入站点暂不支持修改文章设置")
	}
	if site.ContentRevision == ^uint64(0) {
		return Article{}, errors.New("站点修订版本已达到上限")
	}
	for _, candidate := range s.data.Articles {
		if candidate.SiteID != siteID || slug == "" {
			continue
		}
		if strings.EqualFold(slug, candidate.ID) || (candidate.ID != articleID && strings.EqualFold(slug, candidate.Slug)) {
			return Article{}, errors.New("自定义路径与其他文章冲突")
		}
	}
	next := s.data
	next.Articles = append([]Article{}, s.data.Articles...)
	for i, article := range next.Articles {
		if article.SiteID != siteID || article.ID != articleID {
			continue
		}
		if article.Slug == slug && article.ArticleType == articleType && article.ExternalLink == externalLink && article.IsIncludedInNavigation == inNavigation && article.NavigationWeight == weight {
			return cloneArticle(article), nil
		}
		article.Slug, article.ArticleType, article.ExternalLink = slug, articleType, externalLink
		article.IsIncludedInNavigation, article.NavigationWeight, article.Modified = inNavigation, weight, time.Now().UTC()
		next.Articles[i] = article
		markContentChange(&next, siteID, true)
		if err := s.write(next); err != nil {
			return Article{}, err
		}
		return cloneArticle(article), nil
	}
	return Article{}, errors.New("文章不存在")
}
