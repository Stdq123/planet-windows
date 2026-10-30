// Package store persists the local sites and articles created in Planet Windows.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"windows/internal/feeds"
)

type Site struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	About             string     `json:"about"`
	Created           time.Time  `json:"created"`
	IPNS              string     `json:"ipns,omitempty"`
	LastPublishedCID  string     `json:"lastPublishedCID,omitempty"`
	LastPublished     *time.Time `json:"lastPublished,omitempty"`
	TemplateID        string     `json:"templateId,omitempty"`
	TemplateName      string     `json:"templateName,omitempty"`
	Imported          bool       `json:"imported,omitempty"`
	ContentRevision   uint64     `json:"contentRevision,omitempty"`
	PublishedRevision uint64     `json:"publishedRevision,omitempty"`
	PublishPending    bool       `json:"publishPending,omitempty"`
}

type Article struct {
	ID               string    `json:"id"`
	SiteID           string    `json:"siteId"`
	Title            string    `json:"title"`
	Content          string    `json:"content"`
	Created          time.Time `json:"created"`
	Modified         time.Time `json:"modified"`
	Attachments      []string  `json:"attachments,omitempty"`
	ArticleNumber    *int      `json:"articleNumber,omitempty"`
	ArticleReference string    `json:"articleReference,omitempty"`
}

type data struct {
	Version        int                        `json:"version"`
	Sites          []Site                     `json:"sites"`
	Articles       []Article                  `json:"articles"`
	Subscriptions  []Subscription             `json:"subscriptions,omitempty"`
	BackupMetadata map[string]json.RawMessage `json:"backupMetadata,omitempty"`
}

type Subscription struct {
	URL  string     `json:"url"`
	Feed feeds.Feed `json:"feed"`
}

type Store struct {
	mu   sync.Mutex
	path string
	data data
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "library.json"), data: data{Version: 1, Sites: []Site{}, Articles: []Article{}}}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("read library: %w", err)
	}
	if s.data.Version != 1 {
		return nil, fmt.Errorf("unsupported library version: %d", s.data.Version)
	}
	return s, nil
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b)
	return strings.ToUpper(s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]), nil
}

// write commits the supplied snapshot before it becomes visible in memory.
func (s *Store) write(next data) error {
	b, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".library-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.data = next
	return nil
}

func (s *Store) ListSites() ([]Site, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Site{}, s.data.Sites...), nil
}

// PublicationSnapshot reads a consistent site, article list and backup metadata.
func (s *Store) PublicationSnapshot(siteID string) (Site, []Article, json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var site Site
	found := false
	for _, item := range s.data.Sites {
		if item.ID == siteID {
			site = item
			found = true
			break
		}
	}
	if !found {
		return Site{}, nil, nil, errors.New("站点不存在")
	}
	articles := []Article{}
	for _, item := range s.data.Articles {
		if item.SiteID == siteID {
			item.Attachments = append([]string(nil), item.Attachments...)
			articles = append(articles, item)
		}
	}
	return site, articles, append(json.RawMessage(nil), s.data.BackupMetadata[siteID]...), nil
}

// markContentChange is called under the store lock on a detached next snapshot.
func markContentChange(next *data, siteID string, publish bool) {
	next.Sites = append([]Site{}, next.Sites...)
	for i := range next.Sites {
		if next.Sites[i].ID == siteID {
			next.Sites[i].ContentRevision++
			if publish {
				next.Sites[i].PublishPending = true
			}
			return
		}
	}
}

func (s *Store) Import(site Site, articles []Article, raw json.RawMessage) error {
	if site.ID == "" || site.IPNS == "" || !json.Valid(raw) {
		return errors.New("备份元数据无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hasSite(site.ID) {
		return errors.New("站点已经存在")
	}
	for _, article := range articles {
		if article.SiteID != site.ID || article.ID == "" {
			return errors.New("备份文章无效")
		}
	}
	next := s.data
	next.Sites = append(append([]Site{}, s.data.Sites...), site)
	next.Articles = append(append([]Article{}, s.data.Articles...), articles...)
	next.BackupMetadata = make(map[string]json.RawMessage, len(s.data.BackupMetadata)+1)
	for id, value := range s.data.BackupMetadata {
		next.BackupMetadata[id] = value
	}
	next.BackupMetadata[site.ID] = append(json.RawMessage(nil), raw...)
	return s.write(next)
}

func (s *Store) BackupMetadata(siteID string) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasSite(siteID) {
		return nil, errors.New("站点不存在")
	}
	return append(json.RawMessage(nil), s.data.BackupMetadata[siteID]...), nil
}

func (s *Store) ListSubscriptions() ([]Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := append([]Subscription{}, s.data.Subscriptions...)
	for i := range result {
		result[i].Feed.Items = append([]feeds.Item{}, result[i].Feed.Items...)
	}
	return result, nil
}

func (s *Store) SaveSubscription(address string, feed feeds.Feed) (Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	feed.Items = append([]feeds.Item{}, feed.Items...)
	subscription := Subscription{URL: address, Feed: feed}
	next := s.data
	next.Subscriptions = append([]Subscription{}, s.data.Subscriptions...)
	found := false
	for i, item := range next.Subscriptions {
		if item.URL == address {
			seen := make(map[string]bool)
			identity := func(entry feeds.Item) string {
				if entry.ID != "" {
					return "id:" + entry.ID
				}
				if entry.Link != "" {
					return "link:" + entry.Link
				}
				return ""
			}
			for _, entry := range subscription.Feed.Items {
				if key := identity(entry); key != "" {
					seen[key] = true
				}
			}
			for _, entry := range item.Feed.Items {
				key := identity(entry)
				if key == "" || !seen[key] {
					subscription.Feed.Items = append(subscription.Feed.Items, entry)
					if key != "" {
						seen[key] = true
					}
				}
			}
			next.Subscriptions[i] = subscription
			found = true
			break
		}
	}
	if !found {
		next.Subscriptions = append(next.Subscriptions, subscription)
	}
	if err := s.write(next); err != nil {
		return Subscription{}, err
	}
	return subscription, nil
}

func (s *Store) CreateSite(name, about string) (Site, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Site{}, errors.New("请输入站点名称")
	}
	id, err := newID()
	if err != nil {
		return Site{}, err
	}
	site := Site{ID: id, Name: name, About: about, Created: time.Now().UTC()}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Sites = append(append([]Site{}, s.data.Sites...), site)
	if err := s.write(next); err != nil {
		return Site{}, err
	}
	return site, nil
}

func (s *Store) SetPublished(siteID, ipns, cid string) error {
	if ipns == "" || cid == "" {
		return errors.New("发布结果缺少 IPNS 或 CID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Sites = append([]Site{}, s.data.Sites...)
	for i := range next.Sites {
		if next.Sites[i].ID == siteID {
			if next.Sites[i].IPNS != "" && next.Sites[i].IPNS != ipns {
				return errors.New("站点 IPNS 身份不匹配")
			}
			now := time.Now().UTC()
			next.Sites[i].IPNS, next.Sites[i].LastPublishedCID, next.Sites[i].LastPublished = ipns, cid, &now
			return s.write(next)
		}
	}
	return errors.New("站点不存在")
}

// SetPublishedRevision records the exact snapshot sent to IPNS without losing
// a newer save or content change that happened while publication was running.
func (s *Store) SetPublishedRevision(siteID, ipns, cid string, revision uint64) (bool, error) {
	if ipns == "" || cid == "" {
		return false, errors.New("发布结果缺少 IPNS 或 CID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Sites = append([]Site{}, s.data.Sites...)
	for i := range next.Sites {
		site := &next.Sites[i]
		if site.ID != siteID {
			continue
		}
		if site.IPNS != "" && site.IPNS != ipns {
			return false, errors.New("站点 IPNS 身份不匹配")
		}
		if revision > site.ContentRevision {
			return false, errors.New("发布版本无效")
		}
		now := time.Now().UTC()
		site.IPNS, site.LastPublishedCID, site.LastPublished = ipns, cid, &now
		site.PublishedRevision = revision
		site.PublishPending = site.ContentRevision != revision
		if err := s.write(next); err != nil {
			return false, err
		}
		return site.PublishPending, nil
	}
	return false, errors.New("站点不存在")
}

func (s *Store) SetIdentity(siteID, ipns string) error {
	if ipns == "" {
		return errors.New("IPNS 身份不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Sites = append([]Site{}, s.data.Sites...)
	for i := range next.Sites {
		if next.Sites[i].ID == siteID {
			if next.Sites[i].IPNS != "" && next.Sites[i].IPNS != ipns {
				return errors.New("站点 IPNS 身份不匹配")
			}
			next.Sites[i].IPNS = ipns
			return s.write(next)
		}
	}
	return errors.New("站点不存在")
}

func (s *Store) SetTemplate(siteID, templateID, name string) error {
	if templateID == "" || name == "" {
		return errors.New("模板信息无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Sites = append([]Site{}, s.data.Sites...)
	for i := range next.Sites {
		if next.Sites[i].ID == siteID {
			next.Sites[i].TemplateID = templateID
			next.Sites[i].TemplateName = name
			next.Sites[i].ContentRevision++
			return s.write(next)
		}
	}
	return errors.New("站点不存在")
}

func (s *Store) hasSite(id string) bool {
	for _, site := range s.data.Sites {
		if site.ID == id {
			return true
		}
	}
	return false
}

func (s *Store) ListArticles(siteID string) ([]Article, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasSite(siteID) {
		return nil, errors.New("站点不存在")
	}
	articles := []Article{}
	for _, a := range s.data.Articles {
		if a.SiteID == siteID {
			articles = append(articles, a)
		}
	}
	return articles, nil
}

// AddAttachment records an already-copied attachment for a Windows article.
func (s *Store) AddAttachment(siteID, articleID, name string) (Article, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var selected *Site
	for i := range s.data.Sites {
		if s.data.Sites[i].ID == siteID {
			selected = &s.data.Sites[i]
			break
		}
	}
	if selected == nil {
		return Article{}, errors.New("站点不存在")
	}
	if selected.Imported {
		return Article{}, errors.New("导入的原版站点暂不支持添加附件")
	}
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return Article{}, errors.New("附件名称无效")
	}
	next := s.data
	next.Articles = append([]Article{}, s.data.Articles...)
	for i, article := range next.Articles {
		if article.SiteID != siteID || article.ID != articleID {
			continue
		}
		for _, existing := range article.Attachments {
			if strings.EqualFold(existing, name) {
				return Article{}, errors.New("附件名称已存在")
			}
		}
		article.Attachments = append(append([]string{}, article.Attachments...), name)
		article.Modified = time.Now().UTC()
		next.Articles[i] = article
		markContentChange(&next, siteID, false)
		if err := s.write(next); err != nil {
			return Article{}, err
		}
		return article, nil
	}
	return Article{}, errors.New("文章不存在")
}

// RemoveAttachment removes a saved attachment reference from a Windows article.
func (s *Store) RemoveAttachment(siteID, articleID, name string) (Article, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var selected *Site
	for i := range s.data.Sites {
		if s.data.Sites[i].ID == siteID {
			selected = &s.data.Sites[i]
			break
		}
	}
	if selected == nil {
		return Article{}, errors.New("站点不存在")
	}
	if selected.Imported {
		return Article{}, errors.New("导入的原版站点暂不支持删除附件")
	}
	next := s.data
	next.Articles = append([]Article{}, s.data.Articles...)
	for i, article := range next.Articles {
		if article.SiteID != siteID || article.ID != articleID {
			continue
		}
		found := false
		remaining := make([]string, 0, len(article.Attachments))
		for _, item := range article.Attachments {
			if item == name {
				found = true
			} else {
				remaining = append(remaining, item)
			}
		}
		if !found {
			return Article{}, errors.New("附件不存在")
		}
		article.Attachments = remaining
		article.Modified = time.Now().UTC()
		next.Articles[i] = article
		markContentChange(&next, siteID, false)
		if err := s.write(next); err != nil {
			return Article{}, err
		}
		return article, nil
	}
	return Article{}, errors.New("文章不存在")
}

func (s *Store) SaveArticle(siteID, articleID, title, content string) (Article, error) {
	if strings.TrimSpace(title) == "" {
		return Article{}, errors.New("请输入文章标题")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasSite(siteID) {
		return Article{}, errors.New("站点不存在")
	}
	next := s.data
	next.Articles = append([]Article{}, s.data.Articles...)
	now := time.Now().UTC()
	if articleID != "" {
		for i, article := range next.Articles {
			if article.ID == articleID && article.SiteID == siteID {
				article.Title, article.Content, article.Modified = title, content, now
				next.Articles[i] = article
				markContentChange(&next, siteID, true)
				if err := s.write(next); err != nil {
					return Article{}, err
				}
				return article, nil
			}
		}
		return Article{}, errors.New("文章不存在")
	}
	id, err := newID()
	if err != nil {
		return Article{}, err
	}
	article := Article{ID: id, SiteID: siteID, Title: title, Content: content, Created: now, Modified: now}
	next.Articles = append(next.Articles, article)
	markContentChange(&next, siteID, true)
	if err := s.write(next); err != nil {
		return Article{}, err
	}
	return article, nil
}

func (s *Store) DeleteArticle(siteID, articleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasSite(siteID) {
		return errors.New("站点不存在")
	}
	next := s.data
	next.Articles = make([]Article, 0, len(s.data.Articles))
	found := false
	for _, article := range s.data.Articles {
		if article.SiteID == siteID && article.ID == articleID {
			found = true
			continue
		}
		next.Articles = append(next.Articles, article)
	}
	if !found {
		return errors.New("文章不存在")
	}
	markContentChange(&next, siteID, false)
	return s.write(next)
}
