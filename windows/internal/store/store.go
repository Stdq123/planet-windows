// Package store persists the local sites and articles created in Planet Windows.
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"windows/internal/feeds"
	"windows/internal/services"
)

type PlausibleTracking struct {
	Enabled  bool   `json:"enabled"`
	Domain   string `json:"domain,omitempty"`
	ScriptID string `json:"scriptId,omitempty"`
	Server   string `json:"server,omitempty"`
}
type Site struct {
	ID                string             `json:"id"`
	Name              string             `json:"name"`
	About             string             `json:"about"`
	Created           time.Time          `json:"created"`
	IPNS              string             `json:"ipns,omitempty"`
	LastPublishedCID  string             `json:"lastPublishedCID,omitempty"`
	LastPublished     *time.Time         `json:"lastPublished,omitempty"`
	TemplateID        string             `json:"templateId,omitempty"`
	TemplateName      string             `json:"templateName,omitempty"`
	TemplateSettings  map[string]string  `json:"templateSettings,omitempty"`
	NextArticleNumber int                `json:"nextArticleNumber,omitempty"`
	Imported          bool               `json:"imported,omitempty"`
	TagsUnsupported   bool               `json:"tagsUnsupported,omitempty"`
	Removed           bool               `json:"removed,omitempty"`
	DeletionPending   bool               `json:"deletionPending,omitempty"`
	ContentRevision   uint64             `json:"contentRevision,omitempty"`
	PublishedRevision uint64             `json:"publishedRevision,omitempty"`
	PublishPending    bool               `json:"publishPending,omitempty"`
	PlausibleOverride *PlausibleTracking `json:"plausibleOverride,omitempty"`
	// Public analytics values are derived from verified original metadata at
	// render time; the private API key is never stored in this structure.
	PlausibleEnabled   bool              `json:"-"`
	PlausibleDomain    string            `json:"-"`
	PlausibleAPIServer string            `json:"-"`
	PlausibleScriptID  string            `json:"-"`
	Tags               map[string]string `json:"-"`
}

func cloneSite(site Site) Site {
	if site.PlausibleOverride != nil {
		copy := *site.PlausibleOverride
		site.PlausibleOverride = &copy
	}
	if site.TemplateSettings != nil {
		cloned := make(map[string]string, len(site.TemplateSettings))
		for k, v := range site.TemplateSettings {
			cloned[k] = v
		}
		site.TemplateSettings = cloned
	}
	return site
}

var plausibleDomainPattern = regexp.MustCompile(`(?i)^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*$`)
var plausibleScriptPattern = regexp.MustCompile(`^pa-[A-Za-z0-9_-]{1,128}$`)

func ValidPlausibleScriptID(id string) bool {
	return id == "" || plausibleScriptPattern.MatchString(id)
}

func ValidPlausibleDomain(domain string) bool {
	return len(domain) <= 253 && plausibleDomainPattern.MatchString(domain)
}
func ApplyPlausibleOverride(site Site) Site {
	if site.PlausibleOverride != nil {
		site.PlausibleEnabled = site.PlausibleOverride.Enabled
		site.PlausibleDomain = site.PlausibleOverride.Domain
		site.PlausibleScriptID = site.PlausibleOverride.ScriptID
		if site.PlausibleEnabled {
			origin, err := services.PlausibleServerOrigin(site.PlausibleOverride.Server)
			if err == nil {
				site.PlausibleAPIServer = strings.TrimPrefix(origin, "https://")
			} else {
				site.PlausibleAPIServer = site.PlausibleOverride.Server
			}
		} else {
			site.PlausibleDomain = ""
			site.PlausibleAPIServer = ""
			site.PlausibleScriptID = ""
		}
	}
	return site
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
	// Slug is populated only for validated imported-site rendering.
	Slug            string            `json:"-"`
	Tags            map[string]string `json:"tags"`
	AudioFilename   string            `json:"audioFilename,omitempty"`
	VideoFilename   string            `json:"videoFilename,omitempty"`
	AudioDuration   *int              `json:"audioDuration,omitempty"`
	AudioByteLength *int64            `json:"audioByteLength,omitempty"`
}

// Draft is private local work and is never included in publication snapshots.
type Draft struct {
	ID                 string            `json:"id"`
	SiteID             string            `json:"siteId"`
	ArticleID          string            `json:"articleId,omitempty"`
	Title              string            `json:"title"`
	Content            string            `json:"content"`
	Tags               map[string]string `json:"tags"`
	Created            time.Time         `json:"created"`
	Modified           time.Time         `json:"modified"`
	BaseModified       *time.Time        `json:"baseModified,omitempty"`
	Attachments        []string          `json:"attachments,omitempty"`
	RemovedAttachments []string          `json:"removedAttachments,omitempty"`
}

type data struct {
	Version        int                        `json:"version"`
	Sites          []Site                     `json:"sites"`
	Articles       []Article                  `json:"articles"`
	Drafts         []Draft                    `json:"drafts,omitempty"`
	DraftCleanup   []DraftCleanup             `json:"draftCleanup,omitempty"`
	DraftStaging   []DraftCleanup             `json:"draftStaging,omitempty"`
	ArticleCleanup []ArticleCleanup           `json:"articleCleanup,omitempty"`
	Subscriptions  []Subscription             `json:"subscriptions,omitempty"`
	BackupMetadata map[string]json.RawMessage `json:"backupMetadata,omitempty"`
}

type DraftCleanup struct {
	SiteID  string `json:"siteId"`
	DraftID string `json:"draftId"`
	Name    string `json:"name"`
}

type ArticleCleanup struct {
	SiteID    string `json:"siteId"`
	ArticleID string `json:"articleId"`
	Name      string `json:"name"`
}

func (s *Store) PendingArticleCleanup() []ArticleCleanup {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ArticleCleanup(nil), s.data.ArticleCleanup...)
}

func (s *Store) ArticleFileReferences() []ArticleCleanup {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ArticleCleanup
	for _, article := range s.data.Articles {
		for _, name := range article.Attachments {
			out = append(out, ArticleCleanup{SiteID: article.SiteID, ArticleID: article.ID, Name: name})
		}
	}
	return out
}

func (s *Store) CompleteArticleCleanup(entry ArticleCleanup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.ArticleCleanup = make([]ArticleCleanup, 0, len(s.data.ArticleCleanup))
	found := false
	for _, current := range s.data.ArticleCleanup {
		if current == entry {
			found = true
		} else {
			next.ArticleCleanup = append(next.ArticleCleanup, current)
		}
	}
	if !found {
		return nil
	}
	return s.write(next)
}

func queueDraftCleanup(next *data, draft Draft, names []string) {
	if len(names) == 0 {
		return
	}
	next.DraftCleanup = append([]DraftCleanup(nil), next.DraftCleanup...)
	for _, name := range names {
		next.DraftCleanup = append(next.DraftCleanup, DraftCleanup{SiteID: draft.SiteID, DraftID: draft.ID, Name: name})
	}
}

func (s *Store) PendingDraftCleanup() []DraftCleanup {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]DraftCleanup(nil), s.data.DraftCleanup...)
}

func (s *Store) PendingDraftStaging() []DraftCleanup {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]DraftCleanup(nil), s.data.DraftStaging...)
}

func (s *Store) BeginDraftAttachment(siteID, draftID, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasSite(siteID) || name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return errors.New("草稿附件无效")
	}
	for _, entry := range append(append([]DraftCleanup(nil), s.data.DraftCleanup...), s.data.DraftStaging...) {
		if entry.SiteID == siteID && entry.DraftID == draftID && strings.EqualFold(entry.Name, name) {
			return errors.New("草稿附件等待清理，请稍后重试")
		}
	}
	for _, draft := range s.data.Drafts {
		if draft.SiteID != siteID || draft.ID != draftID {
			continue
		}
		for _, existing := range draft.Attachments {
			if strings.EqualFold(existing, name) {
				return errors.New("附件名称已存在")
			}
		}
		for _, article := range s.data.Articles {
			if article.SiteID == siteID && article.ID == draft.ArticleID {
				for _, existing := range article.Attachments {
					if strings.EqualFold(existing, name) {
						return errors.New("文章已有同名附件")
					}
				}
			}
		}
		next := s.data
		next.DraftStaging = append(append([]DraftCleanup(nil), s.data.DraftStaging...), DraftCleanup{SiteID: siteID, DraftID: draftID, Name: name})
		return s.write(next)
	}
	return errors.New("草稿不存在")
}

func (s *Store) CompleteDraftStaging(entry DraftCleanup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.DraftStaging = make([]DraftCleanup, 0, len(s.data.DraftStaging))
	found := false
	for _, current := range s.data.DraftStaging {
		if current == entry {
			found = true
		} else {
			next.DraftStaging = append(next.DraftStaging, current)
		}
	}
	if !found {
		return nil
	}
	return s.write(next)
}

// DraftFileReferences includes removed sites, whose private files must survive.
func (s *Store) DraftFileReferences() []DraftCleanup {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []DraftCleanup
	for _, draft := range s.data.Drafts {
		for _, name := range draft.Attachments {
			out = append(out, DraftCleanup{SiteID: draft.SiteID, DraftID: draft.ID, Name: name})
		}
	}
	return out
}

func (s *Store) CompleteDraftCleanup(entry DraftCleanup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.DraftCleanup = make([]DraftCleanup, 0, len(s.data.DraftCleanup))
	found := false
	for _, current := range s.data.DraftCleanup {
		if current == entry {
			found = true
		} else {
			next.DraftCleanup = append(next.DraftCleanup, current)
		}
	}
	if !found {
		return nil
	}
	return s.write(next)
}

type Subscription struct {
	URL             string     `json:"url"`
	Feed            feeds.Feed `json:"feed"`
	CID             string     `json:"cid,omitempty"`
	PinnedCID       string     `json:"pinnedCid,omitempty"`
	LastUpdated     *time.Time `json:"lastUpdated,omitempty"`
	LastError       string     `json:"lastError,omitempty"`
	OfflinePinned   bool       `json:"offlinePinned,omitempty"`
	OfflineSnapshot string     `json:"offlineSnapshot,omitempty"`
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
	s := &Store{path: filepath.Join(dir, "library.json"), data: data{Version: 4, Sites: []Site{}, Articles: []Article{}}}
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
	migrating := s.data.Version >= 1 && s.data.Version <= 3
	if migrating {
		// Capture the exact pre-migration bytes once. Version 4 deliberately
		// prevents historical Windows binaries from dropping newly added fields.
		digest := sha256.Sum256(b)
		snapshot := filepath.Join(dir, fmt.Sprintf("library-before-v4-%x.json", digest))
		if err := snapshotLibrary(snapshot, b); err != nil {
			return nil, fmt.Errorf("snapshot original library: %w", err)
		}
	}
	if s.data.Version == 1 {
		type oldArticle struct {
			ID            string   `json:"id"`
			Attachments   []string `json:"attachments"`
			ArticleNumber *int     `json:"articleNumber"`
		}
		for siteID, raw := range s.data.BackupMetadata {
			var original struct {
				Articles []oldArticle `json:"articles"`
			}
			if err := json.Unmarshal(raw, &original); err != nil {
				return nil, fmt.Errorf("read imported metadata: %w", err)
			}
			for i := range s.data.Articles {
				for _, old := range original.Articles {
					if s.data.Articles[i].SiteID == siteID && strings.EqualFold(s.data.Articles[i].ID, old.ID) {
						s.data.Articles[i].Attachments = append([]string(nil), old.Attachments...)
						s.data.Articles[i].ArticleNumber = old.ArticleNumber
					}
				}
			}
		}
		s.data.Version = 2
	}
	if s.data.Version == 2 {
		for i := range s.data.Articles {
			a := &s.data.Articles[i]
			var original struct {
				Articles []struct {
					ID    string `json:"id"`
					Audio string `json:"audioFilename"`
					Video string `json:"videoFilename"`
				} `json:"articles"`
			}
			raw, imported := s.data.BackupMetadata[a.SiteID]
			if imported {
				if err := json.Unmarshal(raw, &original); err != nil {
					return nil, fmt.Errorf("read imported metadata: %w", err)
				}
				for _, old := range original.Articles {
					if strings.EqualFold(old.ID, a.ID) {
						for _, name := range a.Attachments {
							if name == old.Audio {
								a.AudioFilename = name
							}
							if name == old.Video {
								a.VideoFilename = name
							}
						}
					}
				}
			} else {
				for _, name := range a.Attachments {
					switch mediaType(name) {
					case "audio":
						a.AudioFilename = name
					case "video":
						a.VideoFilename = name
					}
				}
			}
		}
		s.data.Version = 3
	}
	if s.data.Version == 3 {
		s.data.Version = 4
	}
	if s.data.Version != 4 {
		return nil, fmt.Errorf("unsupported library version: %d", s.data.Version)
	}
	// Older Windows libraries may already contain imported articles without the
	// numbering the Mac app assigns while importing a backup.
	next := s.data
	next.Sites = append([]Site(nil), s.data.Sites...)
	next.Articles = append([]Article(nil), s.data.Articles...)
	next.Drafts = append([]Draft(nil), s.data.Drafts...)
	changed := migrating
	for i := range next.Sites {
		if !next.Sites[i].Imported {
			continue
		}
		updated, err := normalizeImportedNumbers(&next.Sites[i], next.Articles, next.BackupMetadata[next.Sites[i].ID])
		if err != nil {
			return nil, fmt.Errorf("upgrade imported article numbers: %w", err)
		}
		if updated && next.Sites[i].LastPublishedCID != "" {
			if next.Sites[i].ContentRevision == ^uint64(0) {
				return nil, errors.New("站点修订版本已达到上限")
			}
			next.Sites[i].ContentRevision++
		}
		changed = changed || updated
		restored, unsupported, err := restoreImportedTags(next.Sites[i].ID, next.Articles, next.BackupMetadata[next.Sites[i].ID])
		if err != nil {
			return nil, fmt.Errorf("upgrade imported tags: %w", err)
		}
		changed = changed || restored
		if next.Sites[i].TagsUnsupported != unsupported {
			next.Sites[i].TagsUnsupported = unsupported
			changed = true
		}
	}
	for i := range next.Articles {
		if next.Articles[i].Tags == nil {
			unsupported := false
			for _, site := range next.Sites {
				if site.ID == next.Articles[i].SiteID && site.TagsUnsupported {
					unsupported = true
					break
				}
			}
			if unsupported {
				continue
			}
			next.Articles[i].Tags = map[string]string{}
			changed = true
		}
	}
	for i := range next.Drafts {
		if next.Drafts[i].Tags != nil {
			continue
		}
		next.Drafts[i].Tags = map[string]string{}
		for _, article := range next.Articles {
			if article.SiteID == next.Drafts[i].SiteID && article.ID == next.Drafts[i].ArticleID {
				next.Drafts[i].Tags = copyTags(article.Tags)
				break
			}
		}
		changed = true
	}
	if changed {
		if err := s.write(next); err != nil {
			return nil, fmt.Errorf("upgrade imported article numbers: %w", err)
		}
	}
	return s, nil
}

// A fully synced private temporary file is linked into place without replacing
// an existing snapshot. Interrupted writes never occupy the final digest name.
func snapshotLibrary(destination string, b []byte) error {
	if info, e := os.Lstat(destination); e == nil && !info.Mode().IsRegular() {
		return errors.New("original library snapshot is not a regular file")
	} else if e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	if old, err := os.ReadFile(destination); err == nil {
		if string(old) != string(b) {
			return errors.New("original library snapshot conflicts")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(destination), ".library-snapshot-*.tmp")
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
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	if err = os.Link(tmp, destination); errors.Is(err, os.ErrExist) {
		old, e := os.ReadFile(destination)
		if e == nil && string(old) == string(b) {
			return nil
		}
	}
	return err
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
	sites := []Site{}
	for _, site := range s.data.Sites {
		if !site.Removed {
			sites = append(sites, cloneSite(site))
		}
	}
	return sites, nil
}

func (s *Store) RemovedSites() ([]Site, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sites := []Site{}
	for _, site := range s.data.Sites {
		if site.Removed {
			sites = append(sites, cloneSite(site))
		}
	}
	return sites, nil
}

// RemoveSite moves the site to a recoverable local state. It does not remove
// the IPNS key or make previously published content disappear from IPFS.
func (s *Store) RemoveSite(siteID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Sites = append([]Site(nil), s.data.Sites...)
	for i := range next.Sites {
		site := &next.Sites[i]
		if site.ID != siteID || site.Removed {
			continue
		}
		if site.PublishPending && site.ContentRevision <= site.PublishedRevision {
			if site.ContentRevision == ^uint64(0) {
				return errors.New("站点修订版本已达到上限")
			}
			site.ContentRevision++
		}
		site.Removed = true
		site.PublishPending = false
		return s.write(next)
	}
	return errors.New("站点不存在")
}

func (s *Store) RestoreSite(siteID string) (Site, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Sites = append([]Site(nil), s.data.Sites...)
	for i := range next.Sites {
		site := &next.Sites[i]
		if site.ID != siteID || !site.Removed {
			continue
		}
		if site.DeletionPending {
			return Site{}, errors.New("站点已开始永久删除，无法恢复")
		}
		site.Removed = false
		if err := s.write(next); err != nil {
			return Site{}, err
		}
		return cloneSite(*site), nil
	}
	return Site{}, errors.New("已移除的站点不存在")
}

// BeginSiteDeletion commits an irreversible deletion intent before files or
// keys are removed, so interrupted cleanup remains visible and retryable.
func (s *Store) BeginSiteDeletion(siteID string) (Site, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Sites = append([]Site(nil), s.data.Sites...)
	for i := range next.Sites {
		site := &next.Sites[i]
		if site.ID != siteID || !site.Removed {
			continue
		}
		if site.DeletionPending {
			return cloneSite(*site), nil
		}
		site.DeletionPending = true
		if err := s.write(next); err != nil {
			return Site{}, err
		}
		return cloneSite(*site), nil
	}
	return Site{}, errors.New("请先移除站点")
}

// FinalizeSiteDeletion forgets metadata only after all managed data is removed.
func (s *Store) FinalizeSiteDeletion(siteID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	next := s.data
	next.Sites = make([]Site, 0, len(s.data.Sites))
	for _, site := range s.data.Sites {
		if site.ID == siteID {
			if !site.Removed || !site.DeletionPending {
				return errors.New("站点未开始永久删除")
			}
			found = true
			continue
		}
		next.Sites = append(next.Sites, site)
	}
	if !found {
		return errors.New("站点不存在")
	}
	next.Articles = make([]Article, 0, len(s.data.Articles))
	for _, item := range s.data.Articles {
		if item.SiteID != siteID {
			next.Articles = append(next.Articles, item)
		}
	}
	next.Drafts = make([]Draft, 0, len(s.data.Drafts))
	for _, item := range s.data.Drafts {
		if item.SiteID != siteID {
			next.Drafts = append(next.Drafts, item)
		}
	}
	next.DraftCleanup = make([]DraftCleanup, 0, len(s.data.DraftCleanup))
	for _, item := range s.data.DraftCleanup {
		if item.SiteID != siteID {
			next.DraftCleanup = append(next.DraftCleanup, item)
		}
	}
	next.DraftStaging = make([]DraftCleanup, 0, len(s.data.DraftStaging))
	for _, item := range s.data.DraftStaging {
		if item.SiteID != siteID {
			next.DraftStaging = append(next.DraftStaging, item)
		}
	}
	next.ArticleCleanup = make([]ArticleCleanup, 0, len(s.data.ArticleCleanup))
	for _, item := range s.data.ArticleCleanup {
		if item.SiteID != siteID {
			next.ArticleCleanup = append(next.ArticleCleanup, item)
		}
	}
	next.BackupMetadata = make(map[string]json.RawMessage, len(s.data.BackupMetadata))
	for id, raw := range s.data.BackupMetadata {
		if id != siteID {
			next.BackupMetadata[id] = raw
		}
	}
	return s.write(next)
}

// PublicationSnapshot reads a consistent site, article list and backup metadata.
func (s *Store) PublicationSnapshot(siteID string) (Site, []Article, json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var site Site
	found := false
	for _, item := range s.data.Sites {
		if item.ID == siteID && !item.Removed {
			site = cloneSite(item)
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
			articles = append(articles, cloneArticle(item))
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
	for _, existing := range s.data.Sites {
		if existing.ID == site.ID {
			return errors.New("站点已经存在或已移至回收站")
		}
	}
	for _, article := range articles {
		if article.SiteID != site.ID || article.ID == "" {
			return errors.New("备份文章无效")
		}
	}
	articles = append([]Article(nil), articles...)
	_, unsupported, err := restoreImportedTags(site.ID, articles, raw)
	if err != nil {
		return err
	}
	site.TagsUnsupported = unsupported
	for i := range articles {
		articles[i] = cloneArticle(articles[i])
	}
	if _, err := normalizeImportedNumbers(&site, articles, raw); err != nil {
		return err
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

func copyTags(tags map[string]string) map[string]string {
	copy := make(map[string]string, len(tags))
	for k, v := range tags {
		copy[k] = v
	}
	return copy
}

func cloneArticle(article Article) Article {
	if article.Tags != nil {
		article.Tags = copyTags(article.Tags)
	}
	article.Attachments = append([]string(nil), article.Attachments...)
	return article
}

func cloneDraft(draft Draft) Draft {
	draft.Tags = copyTags(draft.Tags)
	draft.Attachments = append([]string(nil), draft.Attachments...)
	draft.RemovedAttachments = append([]string(nil), draft.RemovedAttachments...)
	return draft
}

func restoreImportedTags(siteID string, articles []Article, raw json.RawMessage) (bool, bool, error) {
	var original struct {
		Tags     json.RawMessage `json:"tags"`
		Articles []struct {
			ID   string          `json:"id"`
			Tags json.RawMessage `json:"tags"`
		} `json:"articles"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &original); err != nil {
			return false, false, err
		}
	}
	byID := map[string]map[string]string{}
	invalid := map[string]bool{}
	unsupported := false
	var rootTags map[string]string
	if len(original.Tags) > 0 && string(original.Tags) != "null" && json.Unmarshal(original.Tags, &rootTags) != nil {
		unsupported = true
	}
	for _, a := range original.Articles {
		id := strings.ToUpper(a.ID)
		var tags map[string]string
		if len(a.Tags) > 0 && string(a.Tags) != "null" && json.Unmarshal(a.Tags, &tags) != nil {
			invalid[id] = true
			unsupported = true
			continue
		}
		byID[id] = tags
	}
	changed := false
	for i := range articles {
		a := &articles[i]
		if a.SiteID == siteID && a.Tags == nil {
			if invalid[strings.ToUpper(a.ID)] {
				continue
			}
			a.Tags = copyTags(byID[strings.ToUpper(a.ID)])
			changed = true
		}
	}
	return changed, unsupported, nil
}

// normalizeImportedNumbers follows Planet's chronological allocation rule and
// preserves already assigned numbers across deletion, publication and export.
func normalizeImportedNumbers(site *Site, articles []Article, raw json.RawMessage) (bool, error) {
	var original struct {
		Next     int    `json:"nextArticleNumber"`
		Slug     string `json:"slug"`
		Articles []struct {
			Number *int `json:"articleNumber"`
		} `json:"articles"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &original); err != nil {
			return false, err
		}
	}
	indices := make([]int, 0, len(articles))
	for i := range articles {
		if articles[i].SiteID == site.ID {
			indices = append(indices, i)
		}
	}
	sort.Slice(indices, func(i, j int) bool {
		a, b := articles[indices[i]], articles[indices[j]]
		if !a.Created.Equal(b.Created) {
			return a.Created.Before(b.Created)
		}
		return strings.ToUpper(a.ID) < strings.ToUpper(b.ID)
	})
	used := map[int]bool{}
	number := 1
	if original.Next > number {
		number = original.Next
	}
	if site.NextArticleNumber > number {
		number = site.NextArticleNumber
	}
	maxInt := int(^uint(0) >> 1)
	for _, old := range original.Articles {
		if old.Number != nil && *old.Number >= number {
			if *old.Number == maxInt {
				return false, errors.New("文章编号已达到上限")
			}
			number = *old.Number + 1
		}
	}
	changed := false
	for _, i := range indices {
		a := &articles[i]
		if a.ArticleNumber == nil {
			continue
		}
		value := *a.ArticleNumber
		if value <= 0 || used[value] {
			a.ArticleNumber = nil
			changed = true
			continue
		}
		used[value] = true
		if value >= number {
			if value == maxInt {
				return false, errors.New("文章编号已达到上限")
			}
			number = value + 1
		}
	}
	prefix := strings.ToUpper(strings.TrimSpace(original.Slug))
	if prefix == "" {
		prefix = strings.ToUpper(site.ID)
		if len(prefix) > 8 {
			prefix = prefix[:8]
		}
	}
	for _, i := range indices {
		a := &articles[i]
		if a.ArticleNumber == nil {
			if number == maxInt {
				return false, errors.New("文章编号已达到上限")
			}
			assigned := number
			a.ArticleNumber = &assigned
			number++
			changed = true
		}
		reference := fmt.Sprintf("%s-%d", prefix, *a.ArticleNumber)
		if a.ArticleReference != reference {
			a.ArticleReference = reference
			changed = true
		}
	}
	if site.NextArticleNumber != number {
		site.NextArticleNumber = number
		changed = true
	}
	return changed, nil
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
		for j := range result[i].Feed.Items {
			result[i].Feed.Items[j].Key = feeds.ItemKey(result[i].Feed.Items[j])
			result[i].Feed.Items[j].Attachments = append([]string(nil), result[i].Feed.Items[j].Attachments...)
			result[i].Feed.Items[j].Media = append([]feeds.RemoteMedia(nil), result[i].Feed.Items[j].Media...)
			if assets := result[i].Feed.Items[j].OfflineMedia; assets != nil {
				copyAssets := make(map[string]feeds.OfflineAsset, len(assets))
				for key, value := range assets {
					copyAssets[key] = value
				}
				result[i].Feed.Items[j].OfflineMedia = copyAssets
			}
		}
	}
	return result, nil
}

func (s *Store) RemoveSubscription(address string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Subscriptions = make([]Subscription, 0, len(s.data.Subscriptions))
	found := false
	for _, sub := range s.data.Subscriptions {
		if sub.URL == address {
			found = true
			continue
		}
		next.Subscriptions = append(next.Subscriptions, sub)
	}
	if !found {
		return errors.New("订阅不存在")
	}
	return s.write(next)
}

func (s *Store) SaveSubscription(address string, feed feeds.Feed) (Subscription, error) {
	return s.saveSubscription(address, feed, nil, "", nil)
}
func (s *Store) SavePlanetSubscription(address string, feed feeds.Feed, cid, pinnedCID string) (Subscription, error) {
	if cid == "" {
		return Subscription{}, errors.New("站点 CID 无效")
	}
	return s.saveSubscription(address, feed, &cid, pinnedCID, nil)
}
func (s *Store) SaveOfflineSubscription(address string, feed feeds.Feed, snapshot string) (Subscription, error) {
	if snapshot == "" {
		return Subscription{}, errors.New("离线缓存标识无效")
	}
	return s.saveSubscription(address, feed, nil, "", &snapshot)
}
func (s *Store) saveSubscription(address string, feed feeds.Feed, cid *string, pinnedCID string, offlineSnapshot *string) (Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	feed.Items = append([]feeds.Item{}, feed.Items...)
	for i := range feed.Items {
		feed.Items[i].Key = feeds.ItemKey(feed.Items[i])
		feed.Items[i].Starred = false
	}
	now := time.Now().UTC()
	subscription := Subscription{URL: address, Feed: feed, LastUpdated: &now}
	if offlineSnapshot != nil {
		subscription.OfflinePinned = true
		subscription.OfflineSnapshot = *offlineSnapshot
	}
	if cid != nil {
		subscription.CID = *cid
		subscription.PinnedCID = pinnedCID
	}
	next := s.data
	next.Subscriptions = append([]Subscription{}, s.data.Subscriptions...)
	found := false
	for i, item := range next.Subscriptions {
		if item.URL == address {
			if offlineSnapshot == nil {
				subscription.OfflinePinned = item.OfflinePinned
				subscription.OfflineSnapshot = item.OfflineSnapshot
			}
			if cid == nil {
				subscription.CID = item.CID
				subscription.PinnedCID = item.PinnedCID
			} else if pinnedCID == "" {
				subscription.PinnedCID = item.PinnedCID
			}
			seen := make(map[string]bool)
			identity := feeds.ItemKey
			stars := map[string]bool{}
			for _, old := range item.Feed.Items {
				stars[identity(old)] = old.Starred
			}
			for i := range subscription.Feed.Items {
				subscription.Feed.Items[i].Starred = stars[identity(subscription.Feed.Items[i])]
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

// SetSubscriptionPinned records only a pin for the current immutable revision.
func (s *Store) SetSubscriptionPinned(address, cid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Subscriptions = append([]Subscription{}, s.data.Subscriptions...)
	for i := range next.Subscriptions {
		sub := &next.Subscriptions[i]
		if sub.URL == address {
			if cid == "" || sub.CID != cid {
				return errors.New("站点版本已改变，请重新固定")
			}
			sub.PinnedCID = cid
			return s.write(next)
		}
	}
	return errors.New("订阅不存在")
}
func (s *Store) SetSubscriptionError(address, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Subscriptions = append([]Subscription{}, s.data.Subscriptions...)
	if chars := []rune(message); len(chars) > 1000 {
		message = string(chars[:1000])
	}
	for i := range next.Subscriptions {
		if next.Subscriptions[i].URL == address {
			next.Subscriptions[i].LastError = message
			return s.write(next)
		}
	}
	return errors.New("订阅不存在")
}

// SetSubscriptionStarred persists the state without replacing refreshed content.
func (s *Store) SetSubscriptionStarred(address, key string, starred bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key == "" {
		return errors.New("文章标识无效")
	}
	next := s.data
	next.Subscriptions = append([]Subscription{}, s.data.Subscriptions...)
	for i, sub := range next.Subscriptions {
		if sub.URL == address {
			sub.Feed.Items = append([]feeds.Item{}, sub.Feed.Items...)
			for j := range sub.Feed.Items {
				if feeds.ItemKey(sub.Feed.Items[j]) == key {
					sub.Feed.Items[j].Starred = starred
					sub.Feed.Items[j].Key = key
					next.Subscriptions[i] = sub
					return s.write(next)
				}
			}
			return errors.New("订阅文章不存在")
		}
	}
	return errors.New("订阅不存在")
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

func (s *Store) UpdateSite(siteID, name, about string) (Site, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Site{}, errors.New("请输入站点名称")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Sites = append([]Site(nil), s.data.Sites...)
	for i := range next.Sites {
		site := &next.Sites[i]
		if site.ID != siteID || site.Removed {
			continue
		}
		if site.Name == name && site.About == about {
			return cloneSite(*site), nil
		}
		if site.ContentRevision == ^uint64(0) {
			return Site{}, errors.New("站点修订版本已达到上限")
		}
		site.Name, site.About = name, about
		site.ContentRevision++
		if err := s.write(next); err != nil {
			return Site{}, err
		}
		return cloneSite(*site), nil
	}
	return Site{}, errors.New("站点不存在")
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
		if next.Sites[i].ID == siteID && !next.Sites[i].Removed {
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
		if site.ID != siteID || site.Removed {
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
		if next.Sites[i].ID == siteID && !next.Sites[i].Removed {
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
		if next.Sites[i].ID == siteID && !next.Sites[i].Removed {
			if next.Sites[i].TemplateName != name {
				next.Sites[i].TemplateSettings = nil
			}
			next.Sites[i].TemplateID = templateID
			next.Sites[i].TemplateName = name
			next.Sites[i].ContentRevision++
			return s.write(next)
		}
	}
	return errors.New("站点不存在")
}

func (s *Store) SetPlausibleTracking(siteID string, enabled bool, domain string) (Site, error) {
	return s.SetPlausibleTrackingScript(siteID, enabled, domain, "")
}
func (s *Store) SetPlausibleTrackingScript(siteID string, enabled bool, domain, scriptID string) (Site, error) {
	return s.SetPlausibleTrackingServer(siteID, enabled, domain, scriptID, "")
}
func (s *Store) SetPlausibleTrackingServer(siteID string, enabled bool, domain, scriptID, server string) (Site, error) {
	if (enabled || domain != "") && !ValidPlausibleDomain(domain) {
		return Site{}, errors.New("Plausible 站点域名无效")
	}
	if !ValidPlausibleScriptID(scriptID) {
		return Site{}, errors.New("Plausible 脚本 ID 无效")
	}
	if enabled || server != "" {
		origin, err := services.PlausibleServerOrigin(server)
		if err != nil {
			return Site{}, err
		}
		server = origin
		if enabled && server == "https://plausible.io" {
			server = ""
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Sites = append([]Site(nil), s.data.Sites...)
	for i := range next.Sites {
		site := &next.Sites[i]
		if site.ID != siteID || site.Removed {
			continue
		}
		if site.PlausibleOverride != nil && site.PlausibleOverride.Enabled == enabled && site.PlausibleOverride.Domain == domain && site.PlausibleOverride.ScriptID == scriptID && site.PlausibleOverride.Server == server {
			return cloneSite(*site), nil
		}
		if site.ContentRevision == ^uint64(0) {
			return Site{}, errors.New("站点修订版本已达到上限")
		}
		site.PlausibleOverride = &PlausibleTracking{Enabled: enabled, Domain: domain, ScriptID: scriptID, Server: server}
		site.ContentRevision++
		if err := s.write(next); err != nil {
			return Site{}, err
		}
		return cloneSite(*site), nil
	}
	return Site{}, errors.New("站点不存在")
}

func (s *Store) SetTemplateSettings(siteID string, settings map[string]string) (Site, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Sites = append([]Site(nil), s.data.Sites...)
	for i := range next.Sites {
		if next.Sites[i].ID != siteID || next.Sites[i].Removed {
			continue
		}
		if next.Sites[i].TemplateName != "Sepia" || next.Sites[i].TemplateID == "" {
			return Site{}, errors.New("当前模板不支持此设置")
		}
		if next.Sites[i].ContentRevision == ^uint64(0) {
			return Site{}, errors.New("站点修订版本已达到上限")
		}
		next.Sites[i].TemplateSettings = make(map[string]string, len(settings))
		for k, v := range settings {
			next.Sites[i].TemplateSettings[k] = v
		}
		next.Sites[i].ContentRevision++
		if err := s.write(next); err != nil {
			return Site{}, err
		}
		return cloneSite(next.Sites[i]), nil
	}
	return Site{}, errors.New("站点不存在")
}

func (s *Store) hasSite(id string) bool {
	for _, site := range s.data.Sites {
		if site.ID == id && !site.Removed {
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
			articles = append(articles, cloneArticle(a))
		}
	}
	return articles, nil
}

// AddAttachment records an already-copied attachment for a Windows article.
func (s *Store) AddAttachment(siteID, articleID, name string) (Article, error) {
	return s.AddAttachmentWithMetadata(siteID, articleID, name, nil, nil)
}

func mediaType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp3", ".m4a", ".aac", ".wav", ".ogg", ".opus", ".flac":
		return "audio"
	case ".mp4", ".mov", ".m4v", ".webm", ".avi", ".mpeg", ".mpg":
		return "video"
	default:
		return ""
	}
}

func (s *Store) AddAttachmentWithMetadata(siteID, articleID, name string, duration *int, size *int64) (Article, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var selected *Site
	for i := range s.data.Sites {
		if s.data.Sites[i].ID == siteID && !s.data.Sites[i].Removed {
			selected = &s.data.Sites[i]
			break
		}
	}
	if selected == nil {
		return Article{}, errors.New("站点不存在")
	}
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return Article{}, errors.New("附件名称无效")
	}
	for _, entry := range s.data.ArticleCleanup {
		if entry.SiteID == siteID && entry.ArticleID == articleID && strings.EqualFold(entry.Name, name) {
			return Article{}, errors.New("原附件等待清理，请稍后重试")
		}
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
		switch mediaType(name) {
		case "audio":
			article.AudioFilename, article.AudioDuration, article.AudioByteLength = name, duration, size
		case "video":
			article.VideoFilename = name
		}
		article.Modified = time.Now().UTC()
		next.Articles[i] = article
		markContentChange(&next, siteID, false)
		if err := s.write(next); err != nil {
			return Article{}, err
		}
		return cloneArticle(article), nil
	}
	return Article{}, errors.New("文章不存在")
}

// RemoveAttachment removes a saved attachment reference from a Windows article.
func (s *Store) RemoveAttachment(siteID, articleID, name string) (Article, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var selected *Site
	for i := range s.data.Sites {
		if s.data.Sites[i].ID == siteID && !s.data.Sites[i].Removed {
			selected = &s.data.Sites[i]
			break
		}
	}
	if selected == nil {
		return Article{}, errors.New("站点不存在")
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
		if article.AudioFilename == name {
			article.AudioFilename, article.AudioDuration, article.AudioByteLength = "", nil, nil
			for _, item := range remaining {
				if mediaType(item) == "audio" {
					article.AudioFilename = item
				}
			}
		}
		if article.VideoFilename == name {
			article.VideoFilename = ""
			for _, item := range remaining {
				if mediaType(item) == "video" {
					article.VideoFilename = item
				}
			}
		}
		article.Modified = time.Now().UTC()
		next.Articles[i] = article
		markContentChange(&next, siteID, false)
		if err := s.write(next); err != nil {
			return Article{}, err
		}
		return cloneArticle(article), nil
	}
	return Article{}, errors.New("文章不存在")
}

func (s *Store) SaveArticle(siteID, articleID, title, content string) (Article, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveArticleLocked(siteID, articleID, title, content, "", nil, nil)
}

// DraftPreparation stages files before the atomic store write and returns a
// rollback function to remove any files created if that write fails.
type DraftPreparation func(Article, []string) (Article, func(), error)

func (s *Store) saveArticleLocked(siteID, articleID, title, content, draftID string, prepare DraftPreparation, tags map[string]string) (Article, error) {
	if strings.TrimSpace(title) == "" {
		return Article{}, errors.New("请输入文章标题")
	}
	if !s.hasSite(siteID) {
		return Article{}, errors.New("站点不存在")
	}
	var draftFiles, removedFiles []string
	next := s.data
	next.Articles = append([]Article{}, s.data.Articles...)
	if draftID != "" {
		next.Drafts = make([]Draft, 0, len(s.data.Drafts))
		for _, draft := range s.data.Drafts {
			if draft.ID != draftID || draft.SiteID != siteID {
				next.Drafts = append(next.Drafts, draft)
			} else {
				draftFiles = append([]string(nil), draft.Attachments...)
				removedFiles = append([]string(nil), draft.RemovedAttachments...)
				queueDraftCleanup(&next, draft, draft.Attachments)
			}
		}
	}
	if len(draftFiles) > 0 && prepare == nil {
		return Article{}, errors.New("草稿附件需要先复制到文章")
	}
	stage := func(article Article) (Article, func(), error) {
		if len(draftFiles) == 0 {
			return article, func() {}, nil
		}
		return prepare(article, draftFiles)
	}
	now := time.Now().UTC()
	if articleID != "" {
		for i, article := range next.Articles {
			if article.ID == articleID && article.SiteID == siteID {
				article.Title, article.Content, article.Modified = title, content, now
				if tags != nil {
					article.Tags = copyTags(tags)
				}
				for _, name := range removedFiles {
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
						return Article{}, errors.New("原文章附件已改变，请检查草稿")
					}
					article.Attachments = remaining
					if article.AudioFilename == name {
						article.AudioFilename, article.AudioDuration, article.AudioByteLength = "", nil, nil
						for _, item := range remaining {
							if mediaType(item) == "audio" {
								article.AudioFilename = item
							}
						}
					}
					if article.VideoFilename == name {
						article.VideoFilename = ""
						for _, item := range remaining {
							if mediaType(item) == "video" {
								article.VideoFilename = item
							}
						}
					}
					next.ArticleCleanup = append(append([]ArticleCleanup(nil), next.ArticleCleanup...), ArticleCleanup{SiteID: siteID, ArticleID: article.ID, Name: name})
				}
				article, rollback, err := stage(article)
				if err != nil {
					return Article{}, err
				}
				next.Articles[i] = article
				markContentChange(&next, siteID, true)
				if err := s.write(next); err != nil {
					rollback()
					return Article{}, err
				}
				return cloneArticle(article), nil
			}
		}
		return Article{}, errors.New("文章不存在")
	}
	id, err := newID()
	if err != nil {
		return Article{}, err
	}
	var original struct {
		Next     int    `json:"nextArticleNumber"`
		Slug     string `json:"slug"`
		Articles []struct {
			Number *int `json:"articleNumber"`
		} `json:"articles"`
	}
	if raw := s.data.BackupMetadata[siteID]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &original); err != nil {
			return Article{}, fmt.Errorf("站点备份元数据无效：%w", err)
		}
	}
	number := 1
	for _, site := range s.data.Sites {
		if site.ID == siteID && site.NextArticleNumber > number {
			number = site.NextArticleNumber
		}
	}
	if original.Next > number {
		number = original.Next
	}
	for _, old := range original.Articles {
		if old.Number != nil && *old.Number >= number {
			if *old.Number == int(^uint(0)>>1) {
				return Article{}, errors.New("文章编号已达到上限")
			}
			number = *old.Number + 1
		}
	}
	for _, existing := range s.data.Articles {
		if existing.SiteID == siteID && existing.ArticleNumber != nil && *existing.ArticleNumber >= number {
			if *existing.ArticleNumber == int(^uint(0)>>1) {
				return Article{}, errors.New("文章编号已达到上限")
			}
			number = *existing.ArticleNumber + 1
		}
	}
	if number == int(^uint(0)>>1) {
		return Article{}, errors.New("文章编号已达到上限")
	}
	prefix := strings.ToUpper(strings.TrimSpace(original.Slug))
	if prefix == "" {
		prefix = strings.ToUpper(siteID)
		if len(prefix) > 8 {
			prefix = prefix[:8]
		}
	}
	article := Article{ID: id, SiteID: siteID, Title: title, Content: content, Created: now, Modified: now, Tags: copyTags(tags), ArticleNumber: &number, ArticleReference: fmt.Sprintf("%s-%d", prefix, number)}
	article, rollback, err := stage(article)
	if err != nil {
		return Article{}, err
	}
	next.Articles = append(next.Articles, article)
	markContentChange(&next, siteID, true)
	for i := range next.Sites {
		if next.Sites[i].ID == siteID {
			next.Sites[i].NextArticleNumber = number + 1
			break
		}
	}
	if err := s.write(next); err != nil {
		rollback()
		return Article{}, err
	}
	return cloneArticle(article), nil
}

func (s *Store) ListDrafts(siteID string) ([]Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasSite(siteID) {
		return nil, errors.New("站点不存在")
	}
	out := []Draft{}
	for _, draft := range s.data.Drafts {
		if draft.SiteID == siteID {
			out = append(out, cloneDraft(draft))
		}
	}
	return out, nil
}

func (s *Store) SaveDraft(siteID, draftID, articleID, title, content string) (Draft, error) {
	return s.saveDraft(siteID, draftID, articleID, title, content, nil)
}

func (s *Store) SaveDraftWithTags(siteID, draftID, articleID, title, content, tagsJSON string) (Draft, error) {
	tags, err := parseEditedTags(tagsJSON)
	if err != nil {
		return Draft{}, err
	}
	return s.saveDraft(siteID, draftID, articleID, title, content, tags)
}

func (s *Store) saveDraft(siteID, draftID, articleID, title, content string, tags map[string]string) (Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasSite(siteID) {
		return Draft{}, errors.New("站点不存在")
	}
	if tags != nil {
		if err := s.validateTagsLocked(siteID, articleID, tags); err != nil {
			return Draft{}, err
		}
	}
	next := s.data
	next.Drafts = append([]Draft(nil), s.data.Drafts...)
	now := time.Now().UTC()
	if draftID != "" {
		for i, draft := range next.Drafts {
			if draft.ID != draftID || draft.SiteID != siteID {
				continue
			}
			if draft.ArticleID != articleID {
				return Draft{}, errors.New("草稿关联的文章已改变")
			}
			draft.Title, draft.Content, draft.Modified = title, content, now
			if tags != nil {
				draft.Tags = copyTags(tags)
			}
			next.Drafts[i] = draft
			if err := s.write(next); err != nil {
				return Draft{}, err
			}
			return cloneDraft(draft), nil
		}
		return Draft{}, errors.New("草稿不存在")
	}
	var base *time.Time
	inheritedTags := map[string]string{}
	if articleID != "" {
		for _, article := range s.data.Articles {
			if article.SiteID == siteID && article.ID == articleID {
				value := article.Modified
				base = &value
				inheritedTags = copyTags(article.Tags)
				break
			}
		}
		if base == nil {
			return Draft{}, errors.New("原文章不存在")
		}
	}
	id, err := newID()
	if err != nil {
		return Draft{}, err
	}
	if tags != nil {
		inheritedTags = copyTags(tags)
	}
	draft := Draft{ID: id, SiteID: siteID, ArticleID: articleID, Title: title, Content: content, Tags: inheritedTags, Created: now, Modified: now, BaseModified: base}
	next.Drafts = append(next.Drafts, draft)
	if err := s.write(next); err != nil {
		return Draft{}, err
	}
	return cloneDraft(draft), nil
}

func (s *Store) DeleteDraft(siteID, draftID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasSite(siteID) {
		return errors.New("站点不存在")
	}
	next := s.data
	next.Drafts = make([]Draft, 0, len(s.data.Drafts))
	found := false
	for _, draft := range s.data.Drafts {
		if draft.ID == draftID && draft.SiteID == siteID {
			found = true
			queueDraftCleanup(&next, draft, draft.Attachments)
			continue
		}
		next.Drafts = append(next.Drafts, draft)
	}
	if !found {
		return errors.New("草稿不存在")
	}
	return s.write(next)
}

func (s *Store) PublishDraft(siteID, draftID, title, content string) (Article, error) {
	return s.PublishDraftPrepared(siteID, draftID, title, content, nil)
}

func (s *Store) PublishDraftPrepared(siteID, draftID, title, content string, prepare DraftPreparation) (Article, error) {
	return s.publishDraftPrepared(siteID, draftID, title, content, prepare, nil)
}

func (s *Store) PublishDraftPreparedWithTags(siteID, draftID, title, content, tagsJSON string, prepare DraftPreparation) (Article, error) {
	tags, err := parseEditedTags(tagsJSON)
	if err != nil {
		return Article{}, err
	}
	return s.publishDraftPrepared(siteID, draftID, title, content, prepare, tags)
}

func (s *Store) publishDraftPrepared(siteID, draftID, title, content string, prepare DraftPreparation, tags map[string]string) (Article, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, draft := range s.data.Drafts {
		if draft.ID != draftID || draft.SiteID != siteID {
			continue
		}
		if draft.ArticleID != "" {
			found := false
			for _, article := range s.data.Articles {
				if article.SiteID == siteID && article.ID == draft.ArticleID {
					found = true
					if draft.BaseModified == nil || !article.Modified.Equal(*draft.BaseModified) {
						return Article{}, errors.New("原文章已更改，请先检查草稿以免覆盖新内容")
					}
					break
				}
			}
			if !found {
				return Article{}, errors.New("原文章已删除，无法发布此草稿")
			}
		}
		for _, site := range s.data.Sites {
			if site.ID == siteID && site.TagsUnsupported && tags == nil {
				// Preserve the published article's existing tags; unsupported raw
				// metadata and any private draft tags must not be promoted.
				return s.saveArticleLocked(siteID, draft.ArticleID, title, content, draft.ID, prepare, nil)
			}
		}
		if tags == nil {
			tags = draft.Tags
		}
		if err := s.validateTagsLocked(siteID, draft.ArticleID, tags); err != nil {
			return Article{}, err
		}
		return s.saveArticleLocked(siteID, draft.ArticleID, title, content, draft.ID, prepare, tags)
	}
	return Article{}, errors.New("草稿不存在")
}

// ChangeDraftAttachment changes only the private draft metadata.
func (s *Store) ChangeDraftAttachment(siteID, draftID, name string, add bool) (Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasSite(siteID) {
		return Draft{}, errors.New("站点不存在")
	}
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return Draft{}, errors.New("附件名称无效")
	}
	next := s.data
	next.Drafts = append([]Draft(nil), s.data.Drafts...)
	for i, draft := range next.Drafts {
		if draft.ID != draftID || draft.SiteID != siteID {
			continue
		}
		if add {
			for _, pending := range s.data.DraftCleanup {
				if pending.SiteID == siteID && pending.DraftID == draftID && strings.EqualFold(pending.Name, name) {
					return Draft{}, errors.New("草稿附件等待清理，请稍后重试")
				}
			}
			for _, item := range draft.Attachments {
				if strings.EqualFold(item, name) {
					return Draft{}, errors.New("附件名称已存在")
				}
			}
			for _, article := range s.data.Articles {
				if article.ID == draft.ArticleID && article.SiteID == siteID {
					for _, item := range article.Attachments {
						if strings.EqualFold(item, name) {
							return Draft{}, errors.New("文章已有同名附件")
						}
					}
				}
			}
			draft.Attachments = append(append([]string(nil), draft.Attachments...), name)
			next.DraftStaging = make([]DraftCleanup, 0, len(s.data.DraftStaging))
			for _, entry := range s.data.DraftStaging {
				if entry.SiteID != siteID || entry.DraftID != draftID || entry.Name != name {
					next.DraftStaging = append(next.DraftStaging, entry)
				}
			}
		} else {
			files := make([]string, 0, len(draft.Attachments))
			found := false
			for _, item := range draft.Attachments {
				if item == name {
					found = true
				} else {
					files = append(files, item)
				}
			}
			if !found {
				return Draft{}, errors.New("草稿附件不存在")
			}
			queueDraftCleanup(&next, draft, []string{name})
			draft.Attachments = files
		}
		draft.Modified = time.Now().UTC()
		next.Drafts[i] = draft
		if err := s.write(next); err != nil {
			return Draft{}, err
		}
		return cloneDraft(draft), nil
	}
	return Draft{}, errors.New("草稿不存在")
}

// SetDraftExistingAttachmentRemoved changes only the draft until publication.
func (s *Store) SetDraftExistingAttachmentRemoved(siteID, draftID, name string, removed bool) (Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasSite(siteID) {
		return Draft{}, errors.New("站点不存在")
	}
	next := s.data
	next.Drafts = append([]Draft(nil), s.data.Drafts...)
	for i, draft := range next.Drafts {
		if draft.SiteID != siteID || draft.ID != draftID {
			continue
		}
		valid := false
		for _, article := range s.data.Articles {
			if article.SiteID == siteID && article.ID == draft.ArticleID {
				for _, item := range article.Attachments {
					if item == name {
						valid = true
					}
				}
			}
		}
		if !valid {
			return Draft{}, errors.New("原文章附件不存在")
		}
		found := false
		remaining := make([]string, 0, len(draft.RemovedAttachments))
		for _, item := range draft.RemovedAttachments {
			if item == name {
				found = true
			} else {
				remaining = append(remaining, item)
			}
		}
		if removed && !found {
			remaining = append(remaining, name)
		}
		if !removed && !found {
			return Draft{}, errors.New("该附件未标记为删除")
		}
		draft.RemovedAttachments = remaining
		draft.Modified = time.Now().UTC()
		next.Drafts[i] = draft
		if err := s.write(next); err != nil {
			return Draft{}, err
		}
		return cloneDraft(draft), nil
	}
	return Draft{}, errors.New("草稿不存在")
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
