package store

import (
	"errors"
	"strings"

	"windows/internal/feeds"
)

type SearchResult struct {
	Title           string `json:"title"`
	SourceName      string `json:"sourceName"`
	Snippet         string `json:"snippet"`
	SiteID          string `json:"siteId,omitempty"`
	ArticleID       string `json:"articleId,omitempty"`
	SubscriptionURL string `json:"subscriptionUrl,omitempty"`
	ItemKey         string `json:"itemKey,omitempty"`
}

// SearchArticles searches cached, visible articles without fetching remote content.
func (s *Store) SearchArticles(query string) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) > 200 {
		return nil, errors.New("搜索内容不能超过 200 个字符")
	}
	results := []SearchResult{}
	if query == "" {
		return results, nil
	}
	query = strings.ToLower(query)
	s.mu.Lock()
	articles := append([]Article(nil), s.data.Articles...)
	sites := append([]Site(nil), s.data.Sites...)
	subs := append([]Subscription(nil), s.data.Subscriptions...)
	for i := range subs {
		subs[i].Feed.Items = append(subs[i].Feed.Items[:0:0], subs[i].Feed.Items...)
	}
	s.mu.Unlock()
	contains := func(text string) bool { return strings.Contains(strings.ToLower(text), query) }
	snippet := func(text string) string {
		chars := []rune(strings.Join(strings.Fields(text), " "))
		if len(chars) > 240 {
			return string(chars[:240]) + "…"
		}
		return string(chars)
	}
	names := map[string]string{}
	for _, site := range sites {
		if !site.Removed && !site.DeletionPending {
			names[site.ID] = site.Name
		}
	}
	for _, article := range articles {
		name, ok := names[article.SiteID]
		if ok && (contains(article.Title) || contains(article.Content)) {
			results = append(results, SearchResult{Title: article.Title, SourceName: name, Snippet: snippet(article.Content), SiteID: article.SiteID, ArticleID: article.ID})
		}
		if len(results) >= 200 {
			return results, nil
		}
	}
	for _, sub := range subs {
		for _, item := range sub.Feed.Items {
			if contains(item.Title) || contains(item.Description) || contains(item.Content) {
				body := item.Description
				if body == "" {
					body = item.Content
				}
				results = append(results, SearchResult{Title: item.Title, SourceName: sub.Feed.Title, Snippet: snippet(body), SubscriptionURL: sub.URL, ItemKey: feeds.ItemKey(item)})
			}
			if len(results) >= 200 {
				return results, nil
			}
		}
	}
	return results, nil
}
