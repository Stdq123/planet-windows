package store

import (
	"sort"
	"time"
)

type DashboardEntry struct {
	ID       string    `json:"id"`
	SiteID   string    `json:"siteId"`
	SiteName string    `json:"siteName"`
	Title    string    `json:"title"`
	Modified time.Time `json:"modified"`
}

type DashboardSite struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	ArticleCount   int        `json:"articleCount"`
	DraftCount     int        `json:"draftCount"`
	LastPublished  *time.Time `json:"lastPublished,omitempty"`
	HasPublished   bool       `json:"hasPublished"`
	PublishPending bool       `json:"publishPending"`
}

type DashboardSnapshot struct {
	SiteCount      int              `json:"siteCount"`
	ArticleCount   int              `json:"articleCount"`
	DraftCount     int              `json:"draftCount"`
	UnreadCount    int              `json:"unreadCount"`
	Sites          []DashboardSite  `json:"sites"`
	RecentArticles []DashboardEntry `json:"recentArticles"`
	RecentDrafts   []DashboardEntry `json:"recentDrafts"`
}

// Dashboard returns one consistent, detached metadata snapshot. It performs no
// network or filesystem operations and never copies article or draft bodies.
func (s *Store) Dashboard() DashboardSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := DashboardSnapshot{Sites: []DashboardSite{}, RecentArticles: []DashboardEntry{}, RecentDrafts: []DashboardEntry{}}
	indices := make(map[string]int)
	for _, site := range s.data.Sites {
		if site.Removed || site.DeletionPending {
			continue
		}
		entry := DashboardSite{ID: site.ID, Name: site.Name, HasPublished: site.LastPublishedCID != "", PublishPending: site.PublishPending || site.ContentRevision > site.PublishedRevision}
		if site.LastPublished != nil {
			published := *site.LastPublished
			entry.LastPublished = &published
		}
		indices[site.ID] = len(result.Sites)
		result.Sites = append(result.Sites, entry)
	}
	result.SiteCount = len(result.Sites)
	for _, article := range s.data.Articles {
		if index, ok := indices[article.SiteID]; ok {
			site := &result.Sites[index]
			site.ArticleCount++
			result.ArticleCount++
			result.RecentArticles = recentDashboardEntries(result.RecentArticles, DashboardEntry{article.ID, site.ID, site.Name, article.Title, article.Modified})
		}
	}
	for _, draft := range s.data.Drafts {
		if index, ok := indices[draft.SiteID]; ok {
			site := &result.Sites[index]
			site.DraftCount++
			result.DraftCount++
			result.RecentDrafts = recentDashboardEntries(result.RecentDrafts, DashboardEntry{draft.ID, site.ID, site.Name, draft.Title, draft.Modified})
		}
	}
	for _, subscription := range s.data.Subscriptions {
		for _, item := range subscription.Feed.Items {
			if !item.Read {
				result.UnreadCount++
			}
		}
	}
	return result
}

func recentDashboardEntries(entries []DashboardEntry, entry DashboardEntry) []DashboardEntry {
	entries = append(entries, entry)
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].Modified.Equal(entries[j].Modified) {
			return entries[i].Modified.After(entries[j].Modified)
		}
		if entries[i].SiteID != entries[j].SiteID {
			return entries[i].SiteID < entries[j].SiteID
		}
		return entries[i].ID < entries[j].ID
	})
	if len(entries) > 6 {
		entries = entries[:6]
	}
	return entries
}
