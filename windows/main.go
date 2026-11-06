package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"windows/internal/attachments"
	"windows/internal/audiostream"
	"windows/internal/backup"
	"windows/internal/feeds"
	"windows/internal/follow"
	"windows/internal/ipfs"
	"windows/internal/migration"
	"windows/internal/offline"
	"windows/internal/pinnable"
	"windows/internal/preview"
	"windows/internal/publisher"
	"windows/internal/services"
	"windows/internal/sitecleanup"
	"windows/internal/sitegen"
	"windows/internal/siteimage"
	"windows/internal/store"
	"windows/internal/templateinstall"
	"windows/internal/updates"
)

// Library exposes the local authoring functions to the application window.
type Library struct {
	once             sync.Once
	store            *store.Store
	err              error
	publicMu         sync.Mutex
	feedMu           sync.Mutex
	node             *ipfs.Client
	audioMu          sync.Mutex
	audio            *audiostream.Server
	audioTemp        string
	audioSnapshot    string
	audioFile        string
	updateMu         sync.Mutex
	checkedUpdate    *updates.Release
	updateIdentity   string
	downloadedUpdate string
	updateGeneration uint64
	updateCancel     context.CancelFunc
	updateClosing    bool
	updateWorkers    sync.WaitGroup
	telegramAPI      services.TelegramAPI
}

func (l *Library) open() (*store.Store, error) {
	l.once.Do(func() {
		var path string
		path, l.err = mygo.App.Path(mygo.PathUserData)
		if l.err == nil {
			l.store, l.err = store.Open(path)
			if l.err == nil {
				attachments.CleanupDraftFiles(path, l.store)
				attachments.CleanupArticleFiles(path, l.store)
			}
		}
	})
	return l.store, l.err
}

func (l *Library) Sites() ([]store.Site, error) {
	s, err := l.open()
	if err != nil {
		return nil, err
	}
	return s.ListSites()
}

// IPFSStatus observes the existing daemon without waiting for an active operation.
func (l *Library) IPFSStatus() (ipfs.KuboStatus, error) {
	if !l.publicMu.TryLock() {
		return ipfs.KuboStatus{}, errors.New("节点正忙，请稍后刷新")
	}
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return ipfs.KuboStatus{}, err
	}
	disabled := s.IPFSDisabled()
	if l.node == nil {
		return ipfs.KuboStatus{Disabled: disabled}, nil
	}
	status, err := l.node.Status()
	status.Disabled = disabled
	return status, err
}

// SetIPFSEnabled persists the user's node preference and controls the owned daemon.
func (l *Library) SetIPFSEnabled(enabled bool) error {
	if !l.publicMu.TryLock() {
		return errors.New("节点正忙，请稍后重试")
	}
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return err
	}
	wasDisabled := s.IPFSDisabled()
	if err = s.SetIPFSDisabled(!enabled); err != nil {
		return err
	}
	if !enabled {
		if l.node != nil {
			return l.node.Close()
		}
		return nil
	}
	if _, _, err = l.ensureNode(true); err != nil {
		if wasDisabled {
			if restoreErr := s.SetIPFSDisabled(true); restoreErr != nil {
				return fmt.Errorf("启动节点失败：%v；恢复设置失败：%w", err, restoreErr)
			}
		}
		return err
	}
	return nil
}

func (l *Library) IPFSRepoSize() (int64, error) {
	if !l.publicMu.TryLock() {
		return 0, errors.New("节点正忙，请稍后重试")
	}
	defer l.publicMu.Unlock()
	if l.node == nil {
		return 0, errors.New("IPFS 节点未运行")
	}
	return l.node.RepoSize()
}

func (l *Library) IPFSGarbageCollect() error {
	if !l.publicMu.TryLock() {
		return errors.New("节点正忙，请稍后重试")
	}
	defer l.publicMu.Unlock()
	if l.node == nil {
		return errors.New("IPFS 节点未运行")
	}
	return l.node.GarbageCollect()
}
func (l *Library) RemovedSites() ([]store.Site, error) {
	s, err := l.open()
	if err != nil {
		return nil, err
	}
	return s.RemovedSites()
}
func (l *Library) RemoveSite(siteID string) error {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return err
	}
	return s.RemoveSite(siteID)
}
func (l *Library) RestoreSite(siteID string) (store.Site, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return store.Site{}, err
	}
	return s.RestoreSite(siteID)
}

// PermanentlyDeleteSite starts or resumes deletion of a previously removed site.
func (l *Library) PermanentlyDeleteSite(siteID string) error {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	return l.permanentlyDeleteSiteLocked(siteID)
}

func (l *Library) permanentlyDeleteSiteLocked(siteID string) error {
	if !sitecleanup.ValidSiteID(siteID) {
		return errors.New("站点标识无效")
	}
	s, err := l.open()
	if err != nil {
		return err
	}
	sites, err := s.RemovedSites()
	if err != nil {
		return err
	}
	var selected *store.Site
	for i := range sites {
		if sites[i].ID == siteID {
			selected = &sites[i]
			break
		}
	}
	if selected == nil {
		return errors.New("请先移除站点")
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	// Check any existing key before committing irreversible deletion intent.
	hasRepo, err := sitecleanup.KuboRepositoryPresent(root)
	if err != nil {
		return err
	}
	if hasRepo {
		if l.node == nil {
			resources, e := mygo.App.Path(mygo.PathResources)
			if e != nil {
				return e
			}
			executable := filepath.Join(resources, "ipfs.exe")
			if _, e = os.Stat(executable); e != nil {
				return errors.New("缺少 Kubo 程序，无法核对密钥")
			}
			l.node = ipfs.New(executable, filepath.Join(root, "ipfs"))
		}
		if err = l.node.ValidateKeyAPI(); err != nil {
			return err
		}
		if !l.node.Alive() {
			if err = sitecleanup.CheckOfflineKuboConfig(root); err != nil {
				return err
			}
			if err = l.node.Start(true); err != nil {
				return err
			}
			defer func() {
				if e := l.node.Close(); e != nil {
					log.Printf("close temporary Kubo: %v", e)
				}
			}()
		}
		if err = l.node.ValidateKeyAPI(); err != nil {
			return err
		}
		key, err := l.node.LookupKey(siteID)
		if err != nil {
			return err
		}
		if key != "" {
			if selected.IPNS == "" {
				return errors.New("站点密钥身份未知，无法安全删除")
			}
			match, err := l.node.KeyMatches(siteID, selected.IPNS)
			if err != nil {
				return err
			}
			if !match {
				return errors.New("站点密钥与原 IPNS 不匹配，无法安全删除")
			}
		}
	}
	if _, err = s.BeginSiteDeletion(siteID); err != nil {
		return err
	}
	// After the intent is stored, each step is idempotent and a failure remains
	// visible as pending instead of presenting an unsafe restore operation.
	for _, kind := range []string{"attachments", "draft-attachments", "imports"} {
		if err = sitecleanup.RemoveOwnedDirectory(root, kind, siteID); err != nil {
			return fmt.Errorf("永久删除已开始，清理本机文件失败，可稍后重试：%w", err)
		}
	}
	if err = pinnable.Save(root, siteID, ""); err != nil {
		return fmt.Errorf("永久删除已开始，清理固定服务配置失败，可稍后重试：%w", err)
	}
	for _, kind := range []string{"filebase", "plausible", "ssh", "cloudflare"} {
		if err = services.SaveConfig(root, siteID, kind, services.Config{}); err != nil {
			return fmt.Errorf("永久删除已开始，清理服务配置失败，可稍后重试：%w", err)
		}
	}
	if err = services.DeleteTelegramConfig(root, siteID); err != nil {
		return fmt.Errorf("永久删除已开始，清理 Telegram 配置失败，可稍后重试：%w", err)
	}
	if hasRepo {
		if err = l.node.RemoveKeyIfMatches(siteID, selected.IPNS); err != nil {
			return fmt.Errorf("永久删除已开始，清理站点密钥失败，可稍后重试：%w", err)
		}
	}
	if err = s.FinalizeSiteDeletion(siteID); err != nil {
		return fmt.Errorf("永久删除已开始，保存清理状态失败，可稍后重试：%w", err)
	}
	return nil
}
func (l *Library) CreateSite(name, about string) (store.Site, error) {
	s, err := l.open()
	if err != nil {
		return store.Site{}, err
	}
	return s.CreateSite(name, about)
}
func (l *Library) UpdateSite(siteID, name, about string) (store.Site, error) {
	s, err := l.open()
	if err != nil {
		return store.Site{}, err
	}
	return s.UpdateSite(siteID, name, about)
}

func (l *Library) UpdateSiteSettings(siteID, name, about string, doNotIndex bool) (store.Site, error) {
	s, err := l.open()
	if err != nil {
		return store.Site{}, err
	}
	return s.UpdateSiteSettings(siteID, name, about, doNotIndex)
}

func (l *Library) UpdateSiteDetails(siteID, name, about string, doNotIndex bool, slug, author string) (store.Site, error) {
	s, err := l.open()
	if err != nil {
		return store.Site{}, err
	}
	return s.UpdateSiteDetails(siteID, name, about, doNotIndex, slug, author)
}

func (l *Library) UpdateSiteProfile(siteID, profileJSON string) (store.Site, error) {
	var profile store.SiteProfile
	if len(profileJSON) > 16<<10 || json.Unmarshal([]byte(profileJSON), &profile) != nil {
		return store.Site{}, errors.New("站点公开资料无效")
	}
	s, err := l.open()
	if err != nil {
		return store.Site{}, err
	}
	return s.UpdateSiteProfile(siteID, profile)
}

func (l *Library) UpdatePodcastSettings(siteID, language string, explicit bool, categoriesJSON string) (store.Site, error) {
	var categories []string
	if len(categoriesJSON) > 4096 || json.Unmarshal([]byte(categoriesJSON), &categories) != nil {
		return store.Site{}, errors.New("播客分类无效")
	}
	s, err := l.open()
	if err != nil {
		return store.Site{}, err
	}
	return s.UpdatePodcastSettings(siteID, language, explicit, categories)
}

func (l *Library) SelectPodcastCover(ctx context.Context, siteID string) (store.Site, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: mygo.CallerWindow(ctx), Title: "选择播客封面（PNG、JPEG 或 GIF）"})
	if err != nil || len(paths) == 0 {
		return store.Site{}, err
	}
	cover, err := siteimage.ReadAvatar(paths[0])
	if err != nil {
		return store.Site{}, err
	}
	s, err := l.open()
	if err != nil {
		return store.Site{}, err
	}
	return s.SetPodcastCover(siteID, cover)
}

func (l *Library) RemovePodcastCover(siteID string) (store.Site, error) {
	s, err := l.open()
	if err != nil {
		return store.Site{}, err
	}
	return s.SetPodcastCover(siteID, "")
}

// SelectSiteAvatar uses the native file picker and saves a normalized PNG.
func (l *Library) SelectSiteAvatar(ctx context.Context, siteID string) (store.Site, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: mygo.CallerWindow(ctx), Title: "选择站点头像（PNG、JPEG 或 GIF）"})
	if err != nil || len(paths) == 0 {
		return store.Site{}, err
	}
	avatar, err := siteimage.ReadAvatar(paths[0])
	if err != nil {
		return store.Site{}, err
	}
	s, err := l.open()
	if err != nil {
		return store.Site{}, err
	}
	return s.SetSiteAvatar(siteID, avatar)
}

func (l *Library) RemoveSiteAvatar(siteID string) (store.Site, error) {
	s, err := l.open()
	if err != nil {
		return store.Site{}, err
	}
	return s.SetSiteAvatar(siteID, "")
}
func (l *Library) Articles(siteID string) ([]store.Article, error) {
	s, err := l.open()
	if err != nil {
		return nil, err
	}
	return s.ListArticles(siteID)
}
func (l *Library) Drafts(siteID string) ([]store.Draft, error) {
	s, err := l.open()
	if err != nil {
		return nil, err
	}
	return s.ListDrafts(siteID)
}
func (l *Library) SaveDraft(siteID, draftID, articleID, title, content string) (store.Draft, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return store.Draft{}, err
	}
	return s.SaveDraft(siteID, draftID, articleID, title, content)
}
func (l *Library) SaveDraftWithTags(siteID, draftID, articleID, title, content, tagsJSON string) (store.Draft, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return store.Draft{}, err
	}
	return s.SaveDraftWithTags(siteID, draftID, articleID, title, content, tagsJSON)
}
func (l *Library) DeleteDraft(siteID, draftID string) error {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	if err := s.DeleteDraft(siteID, draftID); err != nil {
		return err
	}
	attachments.CleanupDraftFiles(root, s)
	return nil
}
func (l *Library) PublishDraft(siteID, draftID, title, content string) (store.Article, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return store.Article{}, err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return store.Article{}, err
	}
	return attachments.PublishDraft(root, s, siteID, draftID, title, content)
}
func (l *Library) PublishDraftWithTags(siteID, draftID, title, content, tagsJSON string) (store.Article, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return store.Article{}, err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return store.Article{}, err
	}
	return attachments.PublishDraftWithTags(root, s, siteID, draftID, title, content, tagsJSON)
}

func (l *Library) AddDraftAttachment(ctx context.Context, siteID, draftID string) (store.Draft, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: mygo.CallerWindow(ctx), Title: "选择草稿附件"})
	if err != nil || len(paths) == 0 {
		return store.Draft{}, err
	}
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return store.Draft{}, err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return store.Draft{}, err
	}
	return attachments.AddDraft(root, s, siteID, draftID, paths[0])
}

func (l *Library) RemoveDraftAttachment(siteID, draftID, name string) (store.Draft, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return store.Draft{}, err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return store.Draft{}, err
	}
	return attachments.RemoveDraft(root, s, siteID, draftID, name)
}

func (l *Library) SetDraftExistingAttachmentRemoved(siteID, draftID, name string, removed bool) (store.Draft, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return store.Draft{}, err
	}
	return s.SetDraftExistingAttachmentRemoved(siteID, draftID, name, removed)
}

// PreviewDraft supports both private new files and the original article's files.
func (l *Library) PreviewDraft(siteID, draftID, content string) (string, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return "", err
	}
	site, articles, _, err := s.PublicationSnapshot(siteID)
	if err != nil {
		return "", err
	}
	drafts, err := s.ListDrafts(siteID)
	if err != nil {
		return "", err
	}
	var draft *store.Draft
	for i := range drafts {
		if drafts[i].ID == draftID {
			draft = &drafts[i]
			break
		}
	}
	if draft == nil {
		return "", errors.New("草稿不存在")
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return preview.MarkdownWithImages(content, func(name string) ([]byte, string, error) {
		for _, item := range draft.Attachments {
			if item == name {
				return attachments.ReadDraftPreviewImage(root, *draft, name)
			}
		}
		for _, article := range articles {
			if article.ID == draft.ArticleID {
				for _, removed := range draft.RemovedAttachments {
					if removed == name {
						return nil, "", errors.New("草稿中已移除该附件")
					}
				}
				return attachments.ReadPreviewImage(root, site, article, name)
			}
		}
		return nil, "", errors.New("草稿附件不存在")
	})
}
func (l *Library) SaveArticle(siteID, articleID, title, content string) (store.Article, error) {
	s, err := l.open()
	if err != nil {
		return store.Article{}, err
	}
	return s.SaveArticle(siteID, articleID, title, content)
}

func (l *Library) UpdateArticleSettings(siteID, articleID, slug string, articleType int, externalLink string, inNavigation bool, weight int) (store.Article, error) {
	s, err := l.open()
	if err != nil {
		return store.Article{}, err
	}
	return s.UpdateArticleSettings(siteID, articleID, slug, articleType, externalLink, inNavigation, weight)
}
func (l *Library) SaveArticleWithTags(siteID, articleID, title, content, tagsJSON string) (store.Article, error) {
	s, err := l.open()
	if err != nil {
		return store.Article{}, err
	}
	return s.SaveArticleWithTags(siteID, articleID, title, content, tagsJSON)
}

// AddAttachment copies a selected local file into a saved Windows article.
func (l *Library) AddAttachment(ctx context.Context, siteID, articleID string) (store.Article, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: mygo.CallerWindow(ctx), Title: "选择文章附件"})
	if err != nil || len(paths) == 0 {
		return store.Article{}, err
	}
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return store.Article{}, err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return store.Article{}, err
	}
	return attachments.Add(root, s, siteID, articleID, paths[0])
}

// RemoveAttachment removes a file previously added to a Windows article.
func (l *Library) RemoveAttachment(siteID, articleID, name string) (store.Article, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return store.Article{}, err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return store.Article{}, err
	}
	return attachments.Remove(root, s, siteID, articleID, name)
}

// AudioURL provides a revocable loopback stream for the selected article audio.
func (l *Library) AudioURL(siteID, articleID string) (string, error) {
	s, err := l.open()
	if err != nil {
		return "", err
	}
	_, articles, _, err := s.PublicationSnapshot(siteID)
	if err != nil {
		return "", err
	}
	name := ""
	for _, a := range articles {
		if a.ID == articleID {
			name = a.AudioFilename
			break
		}
	}
	if name == "" {
		return "", errors.New("文章没有音频附件")
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	open := func(siteID, articleID, name string) (*os.File, error) {
		site, articles, _, err := s.PublicationSnapshot(siteID)
		if err != nil {
			return nil, err
		}
		for _, a := range articles {
			if a.ID == articleID && a.AudioFilename == name {
				return attachments.OpenRegistered(root, site, a, name)
			}
		}
		return nil, os.ErrNotExist
	}
	return l.startAudio(open, siteID, articleID, name, "")
}

// SubscriptionAudioURL streams a registered Planet audio file from a private temporary copy.
func (l *Library) SubscriptionAudioURL(address, key string) (string, error) {
	s, err := l.open()
	if err != nil {
		return "", err
	}
	subs, err := s.ListSubscriptions()
	if err != nil {
		return "", err
	}
	var item *feeds.Item
	for _, sub := range subs {
		if sub.URL == address {
			for i := range sub.Feed.Items {
				a := &sub.Feed.Items[i]
				if feeds.ItemKey(*a) == key {
					item = a
					break
				}
			}
			break
		}
	}
	if item == nil {
		return "", errors.New("订阅文章不存在")
	}
	if item.ContentCID == "" && item.AudioURL != "" {
		return l.SubscriptionMediaURL(address, key, item.AudioURL)
	}
	if item.ContentCID == "" || item.ArticleID == "" || item.AudioFilename == "" {
		return "", errors.New("订阅文章没有音频附件")
	}
	found := false
	for _, name := range item.Attachments {
		if name == item.AudioFilename {
			found = true
			break
		}
	}
	if !found {
		return "", errors.New("音频附件未登记")
	}
	temp, err := os.CreateTemp("", "planet-audio-*")
	if err != nil {
		return "", err
	}
	path := temp.Name()
	l.publicMu.Lock()
	_, _, err = l.ensureNode(true)
	if err == nil {
		path := item.ArticleID + "/" + item.AudioFilename
		if item.ContentPath != "" {
			path = item.ContentPath + "/" + path
		}
		err = l.node.CopyContent(item.ContentCID, path, temp)
	}
	l.publicMu.Unlock()
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	open := func(siteID, articleID, name string) (*os.File, error) {
		if siteID != address || articleID != key || name != item.AudioFilename {
			return nil, os.ErrNotExist
		}
		return os.Open(path)
	}
	return l.startAudio(open, address, key, item.AudioFilename, path)
}

// SubscriptionMediaURL serves an already-fixed feed media file without network access.
func (l *Library) SubscriptionMediaURL(address, key, mediaURL string) (string, error) {
	l.feedMu.Lock()
	defer l.feedMu.Unlock()
	s, err := l.open()
	if err != nil {
		return "", err
	}
	subs, err := s.ListSubscriptions()
	if err != nil {
		return "", err
	}
	var asset feeds.OfflineAsset
	found := false
	for _, sub := range subs {
		if sub.URL == address {
			for _, item := range sub.Feed.Items {
				if feeds.ItemKey(item) == key {
					asset, found = item.OfflineMedia[mediaURL]
					break
				}
			}
			break
		}
	}
	if !found || (!strings.HasPrefix(asset.ContentType, "audio/") && !strings.HasPrefix(asset.ContentType, "video/")) {
		return "", errors.New("媒体尚未固定到本机")
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	open := func(site, article, name string) (*os.File, error) {
		if site != address || article != key || name != mediaURL {
			return nil, os.ErrNotExist
		}
		return offline.Open(root, asset)
	}
	return l.startMedia(open, address, key, mediaURL, "", asset.ContentType, asset.Snapshot, asset.File)
}

func (l *Library) startAudio(open func(string, string, string) (*os.File, error), site, article, name, temp string) (string, error) {
	return l.startMedia(open, site, article, name, temp, "", "", "")
}
func (l *Library) startMedia(open func(string, string, string) (*os.File, error), site, article, name, temp, mediaType, snapshot, file string) (string, error) {
	l.audioMu.Lock()
	defer l.audioMu.Unlock()
	l.closeAudioLocked()
	server, err := audiostream.New(open)
	if err != nil {
		if temp != "" {
			_ = os.Remove(temp)
		}
		return "", err
	}
	address, err := server.URLWithType(site, article, name, mediaType)
	if err != nil {
		_ = server.Close()
		if temp != "" {
			_ = os.Remove(temp)
		}
		return "", err
	}
	l.audio = server
	l.audioTemp = temp
	l.audioSnapshot, l.audioFile = snapshot, file
	return address, nil
}

// StopAudio revokes the requested stream without stopping newer playback.
func (l *Library) StopAudio(address string) {
	l.audioMu.Lock()
	defer l.audioMu.Unlock()
	if l.audio != nil && l.audio.Revoke(address) {
		l.closeAudioLocked()
	}
}
func (l *Library) closeAudioLocked() {
	l.audioSnapshot, l.audioFile = "", ""
	if l.audio != nil {
		_ = l.audio.Close()
		l.audio = nil
	}
	if l.audioTemp != "" {
		_ = os.Remove(l.audioTemp)
		l.audioTemp = ""
	}
}
func (l *Library) closeAudio() { l.audioMu.Lock(); defer l.audioMu.Unlock(); l.closeAudioLocked() }

func (l *Library) Preview(content string) (string, error) {
	return preview.MarkdownWithImages(content, func(string) ([]byte, string, error) { return nil, "", os.ErrNotExist })
}

// PreviewArticle renders the draft with images registered to its saved article.
func (l *Library) PreviewArticle(siteID, articleID, content string) (string, error) {
	s, err := l.open()
	if err != nil {
		return "", err
	}
	site, articles, _, err := s.PublicationSnapshot(siteID)
	if err != nil {
		return "", err
	}
	var article *store.Article
	for i := range articles {
		if articles[i].ID == articleID {
			article = &articles[i]
			break
		}
	}
	if article == nil {
		return "", errors.New("文章不存在")
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return preview.MarkdownWithImages(content, func(name string) ([]byte, string, error) {
		return attachments.ReadPreviewImage(root, site, *article, name)
	})
}

func (l *Library) DeleteArticle(siteID, articleID string) error {
	s, err := l.open()
	if err != nil {
		return err
	}
	return s.DeleteArticle(siteID, articleID)
}

func (l *Library) Subscriptions() ([]store.Subscription, error) {
	s, err := l.open()
	if err != nil {
		return nil, err
	}
	return s.ListSubscriptions()
}

func (l *Library) SearchArticles(query string) ([]store.SearchResult, error) {
	s, err := l.open()
	if err != nil {
		return nil, err
	}
	return s.SearchArticles(query)
}

// OfflineMediaUsage reports managed feed-cache usage without modifying files.
func (l *Library) OfflineMediaUsage() (offline.Usage, error) { return l.collectOfflineMedia(false) }

// CleanupOfflineMedia removes only files no saved article or active player references.
func (l *Library) CleanupOfflineMedia() (offline.Usage, error) { return l.collectOfflineMedia(true) }
func (l *Library) collectOfflineMedia(clean bool) (offline.Usage, error) {
	l.feedMu.Lock()
	defer l.feedMu.Unlock()
	s, err := l.open()
	if err != nil {
		return offline.Usage{}, err
	}
	subs, err := s.ListSubscriptions()
	if err != nil {
		return offline.Usage{}, err
	}
	refs := map[string]map[string]bool{}
	add := func(snapshot, file string) {
		if snapshot == "" || file == "" {
			return
		}
		if refs[snapshot] == nil {
			refs[snapshot] = map[string]bool{}
		}
		refs[snapshot][file] = true
	}
	for _, sub := range subs {
		for _, item := range sub.Feed.Items {
			for _, asset := range item.OfflineMedia {
				add(asset.Snapshot, asset.File)
			}
		}
	}
	l.audioMu.Lock()
	defer l.audioMu.Unlock()
	add(l.audioSnapshot, l.audioFile)
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return offline.Usage{}, err
	}
	return offline.Collect(root, refs, clean)
}

// Follow adds or refreshes either a web feed or a Planet/IPNS/ENS site.
func (l *Library) Follow(address string) (store.Subscription, error) {
	return l.follow(address, false)
}

// RefreshSubscription only refreshes an existing subscription. A refresh
// queued before removal must not recreate the removed subscription.
func (l *Library) RefreshSubscription(address string) (store.Subscription, error) {
	return l.follow(address, true)
}

func (l *Library) follow(address string, requireExisting bool) (store.Subscription, error) {
	address = strings.TrimSpace(address)
	s, err := l.open()
	if err != nil {
		return store.Subscription{}, err
	}
	if strings.HasPrefix(strings.ToLower(address), "http://") || strings.HasPrefix(strings.ToLower(address), "https://") {
		l.feedMu.Lock()
		defer l.feedMu.Unlock()
		if requireExisting {
			if err := requireSubscription(s, address); err != nil {
				return store.Subscription{}, err
			}
		}
		feed, err := feeds.Fetch(address)
		if err != nil {
			_ = s.SetSubscriptionError(address, err.Error())
			return store.Subscription{}, err
		}
		subs, e := s.ListSubscriptions()
		if e != nil {
			return store.Subscription{}, e
		}
		pinned := false
		var previous feeds.Feed
		for _, sub := range subs {
			if sub.URL == address && sub.OfflinePinned {
				pinned = true
				previous = sub.Feed
				break
			}
		}
		if pinned {
			root, e := mygo.App.Path(mygo.PathUserData)
			if e != nil {
				return store.Subscription{}, e
			}
			saved, snapshot, e := offline.PinWithPrevious(root, address, feed, previous)
			if e != nil {
				_ = s.SetSubscriptionError(address, e.Error())
				return store.Subscription{}, e
			}
			result, e := s.SaveOfflineSubscription(address, saved, snapshot)
			if e != nil {
				_ = offline.Discard(root, snapshot)
			}
			return result, e
		}
		return s.SaveSubscription(address, feed)
	}
	name, err := follow.Normalize(address)
	if err != nil {
		return store.Subscription{}, err
	}
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	if requireExisting {
		if err := requireSubscription(s, name); err != nil {
			return store.Subscription{}, err
		}
	}
	if _, _, err = l.ensureNode(true); err != nil {
		_ = s.SetSubscriptionError(name, err.Error())
		return store.Subscription{}, err
	}
	pinned := false
	subs, err := s.ListSubscriptions()
	if err != nil {
		return store.Subscription{}, err
	}
	for _, sub := range subs {
		if sub.URL == name && sub.PinnedCID != "" {
			pinned = true
			break
		}
	}
	result, err := follow.Fetch(l.node, name, pinned)
	if err != nil {
		_ = s.SetSubscriptionError(name, err.Error())
		return store.Subscription{}, err
	}
	pinnedCID := ""
	if pinned {
		pinnedCID = result.CID
	}
	return s.SavePlanetSubscription(name, result.Feed, result.CID, pinnedCID)
}

func requireSubscription(s *store.Store, address string) error {
	subs, err := s.ListSubscriptions()
	if err != nil {
		return err
	}
	for _, sub := range subs {
		if sub.URL == address {
			return nil
		}
	}
	return errors.New("订阅不存在")
}

// Unfollow removes the subscription and its saved item state. Shared IPFS pins
// and cached files are not deleted by this operation.
func (l *Library) Unfollow(address string) error {
	address = strings.TrimSpace(address)
	s, err := l.open()
	if err != nil {
		return err
	}
	if strings.HasPrefix(strings.ToLower(address), "http://") || strings.HasPrefix(strings.ToLower(address), "https://") {
		l.feedMu.Lock()
		defer l.feedMu.Unlock()
		return s.RemoveSubscription(address)
	}
	address, err = follow.Normalize(address)
	if err != nil {
		return err
	}
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	return s.RemoveSubscription(address)
}

// PinSubscription stores the current Planet revision in Kubo for offline use.
func (l *Library) PinSubscription(address string) error {
	s, err := l.open()
	if err != nil {
		return err
	}
	if strings.HasPrefix(strings.ToLower(address), "http://") || strings.HasPrefix(strings.ToLower(address), "https://") {
		l.feedMu.Lock()
		defer l.feedMu.Unlock()
		subs, e := s.ListSubscriptions()
		if e != nil {
			return e
		}
		var selected *store.Subscription
		for i := range subs {
			if subs[i].URL == address {
				selected = &subs[i]
				break
			}
		}
		if selected == nil {
			return errors.New("订阅不存在")
		}
		root, e := mygo.App.Path(mygo.PathUserData)
		if e != nil {
			return e
		}
		saved, snapshot, e := offline.PinWithPrevious(root, address, selected.Feed, selected.Feed)
		if e != nil {
			return e
		}
		_, e = s.SaveOfflineSubscription(address, saved, snapshot)
		if e != nil {
			_ = offline.Discard(root, snapshot)
		}
		return e
	}
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	subs, err := s.ListSubscriptions()
	if err != nil {
		return err
	}
	cid := ""
	for _, sub := range subs {
		if sub.URL == address {
			cid = sub.CID
			break
		}
	}
	if cid == "" {
		return errors.New("此订阅没有可固定的 IPFS 内容")
	}
	if _, _, err = l.ensureNode(true); err != nil {
		return err
	}
	if err = l.node.Pin(cid); err != nil {
		return err
	}
	return s.SetSubscriptionPinned(address, cid)
}
func (l *Library) StarSubscription(address, key string, starred bool) error {
	s, err := l.open()
	if err != nil {
		return err
	}
	return s.SetSubscriptionStarred(address, key, starred)
}

func (l *Library) SetSubscriptionRead(address, key string, read bool) error {
	s, err := l.open()
	if err != nil {
		return err
	}
	return s.SetSubscriptionRead(address, key, read)
}

func (l *Library) MarkSubscriptionRead(address string) error {
	s, err := l.open()
	if err != nil {
		return err
	}
	return s.MarkSubscriptionRead(address)
}

// SubscriptionPreview renders registered Planet images from the local IPFS repository.
func (l *Library) SubscriptionPreview(address, key string) (string, error) {
	l.feedMu.Lock()
	defer l.feedMu.Unlock()
	s, err := l.open()
	if err != nil {
		return "", err
	}
	subs, err := s.ListSubscriptions()
	if err != nil {
		return "", err
	}
	var selected *feeds.Item
	for _, sub := range subs {
		if sub.URL == address {
			for i := range sub.Feed.Items {
				item := &sub.Feed.Items[i]
				if feeds.ItemKey(*item) == key {
					selected = item
					break
				}
			}
			break
		}
	}
	if selected == nil {
		return "", errors.New("订阅文章不存在")
	}
	if selected.ContentCID == "" && selected.OfflineBase != "" {
		root, e := mygo.App.Path(mygo.PathUserData)
		if e != nil {
			return "", e
		}
		return offline.Render(root, *selected)
	}
	if selected.ContentCID == "" || selected.ArticleID == "" {
		return "", errors.New("订阅文章不存在")
	}
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	if _, _, err = l.ensureNode(true); err != nil {
		return "", err
	}
	allowed := map[string]bool{}
	for _, name := range selected.Attachments {
		allowed[name] = true
	}
	return preview.HTMLWithImages(selected.Content, func(name string) ([]byte, string, error) {
		if !allowed[name] {
			return nil, "", errors.New("附件未登记")
		}
		path := selected.ArticleID + "/" + name
		if selected.ContentPath != "" {
			path = selected.ContentPath + "/" + path
		}
		data, err := l.node.Read(selected.ContentCID, path)
		if err != nil {
			return nil, "", err
		}
		if len(data) > 8<<20 {
			return nil, "", errors.New("图片过大")
		}
		mime := http.DetectContentType(data)
		return data, mime, nil
	})
}

// PublishSite publishes the saved snapshot; Kubo is started only on request.
func (l *Library) PublishSite(siteID string) (publisher.Result, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return publisher.Result{}, err
	}
	snapshot, articles, raw, err := s.PublicationSnapshot(siteID)
	if err != nil {
		return publisher.Result{}, err
	}
	site := &snapshot
	var importedPublic string
	if len(raw) > 0 {
		root, e := mygo.App.Path(mygo.PathUserData)
		if e != nil {
			return publisher.Result{}, e
		}
		importedPublic, e = os.MkdirTemp("", "planet-import-publish-")
		if e != nil {
			return publisher.Result{}, e
		}
		defer os.RemoveAll(importedPublic)
		templateDir, e := l.templatePath(*site)
		if e != nil {
			return publisher.Result{}, e
		}
		if templateDir == "" {
			return publisher.Result{}, errors.New("发布导入站点前，请先选择已验证的模板")
		}
		if e = migration.StageImportedPublic(filepath.Join(root, "imports", siteID), importedPublic, raw, *site, articles, templateDir); e != nil {
			return publisher.Result{}, fmt.Errorf("无法发布原版备份：%w", e)
		}
	}
	if _, _, err := l.ensureNode(true); err != nil {
		return publisher.Result{}, err
	}
	if site.IPNS == "" {
		key, e := l.node.EnsureKey(site.ID)
		if e != nil {
			return publisher.Result{}, e
		}
		if e = s.SetIdentity(site.ID, key); e != nil {
			return publisher.Result{}, e
		}
		site.IPNS = key
	}
	var result publisher.Result
	if importedPublic != "" {
		result, err = publisher.PublishDirectory(l.node, importedPublic, *site)
	} else {
		templateDir, e := l.templatePath(*site)
		if e != nil {
			return publisher.Result{}, e
		}
		root, e := mygo.App.Path(mygo.PathUserData)
		if e != nil {
			return publisher.Result{}, e
		}
		result, err = publisher.PublishWithAssets(l.node, os.TempDir(), *site, articles, templateDir, root)
	}
	if err != nil {
		return publisher.Result{}, err
	}
	result.Revision = site.ContentRevision
	result.Pending, err = s.SetPublishedRevision(siteID, result.IPNS, result.CID, result.Revision)
	if err != nil {
		return result, fmt.Errorf("IPNS 已发布到 %s，但保存本地状态失败：%w", result.IPNS, err)
	}
	// This integration is opt-in. IPNS publication has already succeeded, so a
	// separate warning must not turn it into a failed publication.
	root, e := mygo.App.Path(mygo.PathUserData)
	if e == nil {
		endpoint, e := pinnable.Load(root, siteID)
		if e != nil {
			result.PinnableWarning = "站点已发布，但无法读取 Pinnable 配置。"
		} else if endpoint != "" {
			if e = pinnable.NewClient().PinForSite(endpoint, result.IPNS, true); e != nil {
				result.PinnableWarning = "站点已发布，但 Pinnable 固定请求未完成：" + e.Error() + "。请在固定服务中检查。"
			}
		}
		if e = services.PinPublished(root, siteID, result.CID, services.NewClient()); e != nil {
			result.PinnableWarning += " 站点已发布，但 Filebase 固定请求未完成：" + e.Error()
		}
		if warning := l.autoDeploy(root, *site, articles); warning != "" {
			result.PinnableWarning += " IPNS 已发布，但外部发布未完成：" + warning
		}
		result.TelegramWarning = l.syncPublishedTelegram(root, *site, articles, raw)
	} else {
		result.TelegramWarning = "站点已发布，但无法读取 Telegram 本机配置。"
	}
	return result, nil
}

// ConfigureService stores credentials encrypted for the current Windows user.
func (l *Library) ConfigureService(siteID, kind, name, token string) error {
	return l.configureService(siteID, kind, name, token, "", "")
}

// ConfigurePlausible sets the domain, optional site-specific script ID and key.
func (l *Library) ConfigurePlausible(siteID, domain, token, scriptID string) error {
	return l.ConfigurePlausibleAt(siteID, domain, token, scriptID, "https://plausible.io")
}

// ConfigurePlausibleAt accepts an HTTPS service selected explicitly by the user.
func (l *Library) ConfigurePlausibleAt(siteID, domain, token, scriptID, server string) error {
	domain, token, scriptID = strings.TrimSpace(domain), strings.TrimSpace(token), strings.TrimSpace(scriptID)
	if strings.TrimSpace(server) == "" {
		return errors.New("请明确填写 Plausible HTTPS 服务地址")
	}
	origin, err := services.PlausibleServerOrigin(server)
	if err != nil {
		return err
	}
	if origin == "https://plausible.io" {
		origin = ""
	}
	if token != "" {
		return l.configureService(siteID, "plausible", domain, token, scriptID, origin)
	}
	if !store.ValidPlausibleDomain(domain) || !store.ValidPlausibleScriptID(scriptID) {
		return errors.New("Plausible 域名或脚本 ID 无效")
	}
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return err
	}
	if _, _, _, err = s.PublicationSnapshot(siteID); err != nil {
		return err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	old, err := services.LoadConfig(root, siteID, "plausible")
	if err != nil {
		return err
	}
	if old.Token != "" && old.Server != origin {
		return errors.New("更换 Plausible 服务器时请重新输入该服务器的 API 令牌，或先停用原配置")
	}
	changed := old.Token != "" && old.Name != domain
	if changed {
		next := old
		next.Name = domain
		if err = services.SaveConfig(root, siteID, "plausible", next); err != nil {
			return err
		}
	}
	if _, err = s.SetPlausibleTrackingServer(siteID, true, domain, scriptID, origin); err != nil {
		if changed {
			if rollback := services.SaveConfig(root, siteID, "plausible", old); rollback != nil {
				return errors.New("站点设置保存失败，服务配置恢复也失败，请重新检查")
			}
		}
		return err
	}
	return nil
}
func (l *Library) configureService(siteID, kind, name, token, scriptID, server string) error {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return err
	}
	site, _, raw, err := s.PublicationSnapshot(siteID)
	if err != nil {
		return err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	name, token = strings.TrimSpace(name), strings.TrimSpace(token)
	if kind == "plausible" && token != "" && !store.ValidPlausibleDomain(name) {
		return errors.New("Plausible 站点域名无效")
	}
	if kind == "plausible" && !store.ValidPlausibleScriptID(scriptID) {
		return errors.New("Plausible 脚本 ID 无效，请输入 pa- 开头的站点专属 ID")
	}
	// Disable tracking even when the old encrypted credential is unreadable.
	if kind == "plausible" && token == "" {
		var savedDomain, savedScript, savedServer string
		if site.PlausibleOverride != nil {
			savedDomain = site.PlausibleOverride.Domain
			savedScript = site.PlausibleOverride.ScriptID
			savedServer = site.PlausibleOverride.Server
			if site.PlausibleOverride.Enabled && savedServer == "" {
				savedServer = "https://plausible.io"
			}
		} else if len(raw) > 0 {
			_, savedDomain, savedServer, _ = backup.PlausibleStoredSettings(raw)
		}
		if _, err = s.SetPlausibleTrackingServer(siteID, false, savedDomain, savedScript, savedServer); err != nil {
			return err
		}
		if err = services.SaveConfig(root, siteID, kind, services.Config{}); err != nil {
			return errors.New("站点跟踪已停用，但本机服务凭据未能清理，请重试")
		}
		return nil
	}
	old, err := services.LoadConfig(root, siteID, kind)
	if err != nil {
		return err
	}
	cfg := services.Config{Name: name, Token: token, Server: server}
	if old.Name == name && old.Token == token && old.Server == server {
		cfg = old
	}
	if err = services.SaveConfig(root, siteID, kind, cfg); err != nil {
		return err
	}
	if kind == "plausible" {
		if _, err = s.SetPlausibleTrackingServer(siteID, token != "", name, scriptID, server); err != nil {
			if rollbackErr := services.SaveConfig(root, siteID, kind, old); rollbackErr != nil {
				return errors.New("站点统计设置保存失败，服务配置恢复也失败，请重新检查")
			}
			return err
		}
	}
	return nil
}

type serviceInfo struct {
	Name            string `json:"name"`
	Configured      bool   `json:"configured"`
	LastCID         string `json:"lastCid,omitempty"`
	RequestID       string `json:"requestId,omitempty"`
	PendingCID      string `json:"pendingCid,omitempty"`
	ScriptID        string `json:"scriptId,omitempty"`
	TrackingEnabled bool   `json:"trackingEnabled,omitempty"`
	TrackingWarning string `json:"trackingWarning,omitempty"`
	Server          string `json:"server,omitempty"`
}

func (l *Library) ServiceInfo(siteID, kind string) (serviceInfo, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return serviceInfo{}, err
	}
	site, _, raw, err := s.PublicationSnapshot(siteID)
	if err != nil {
		return serviceInfo{}, err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return serviceInfo{}, err
	}
	cfg, err := services.LoadConfig(root, siteID, kind)
	info := serviceInfo{Name: cfg.Name, Configured: cfg.Token != "", LastCID: cfg.LastCID, RequestID: cfg.RequestID, PendingCID: cfg.PendingCID}
	if kind == "plausible" && site.PlausibleOverride != nil {
		info.ScriptID = site.PlausibleOverride.ScriptID
		info.TrackingEnabled = site.PlausibleOverride.Enabled
		info.Server = site.PlausibleOverride.Server
		if info.Name == "" {
			info.Name = site.PlausibleOverride.Domain
		}
		if !site.PlausibleOverride.Enabled && info.Server == "" && len(raw) > 0 {
			_, domain, server, e := backup.PlausibleStoredSettings(raw)
			if e != nil {
				info.TrackingWarning = "原版统计配置不受支持；请手动填写服务器。"
			} else {
				info.Server = server
				if info.Name == "" {
					info.Name = domain
				}
			}
		}
	} else if kind == "plausible" && len(raw) > 0 {
		enabled, domain, server, e := backup.PlausibleStoredSettings(raw)
		if e != nil {
			info.TrackingWarning = "原版统计配置不受支持；可停用或重新配置。"
		} else {
			info.TrackingEnabled = enabled
			info.Server = server
			if info.Name == "" {
				info.Name = domain
			}
		}
	}
	if kind == "plausible" && info.Server == "" && info.TrackingWarning == "" && (site.PlausibleOverride != nil || len(raw) == 0) {
		info.Server = "https://plausible.io"
	}
	return info, err
}
func (l *Library) PlausibleStats(siteID string) (services.Stats, error) {
	return l.PlausibleStatsRange(siteID, "7d")
}
func (l *Library) PlausibleStatsRange(siteID, period string) (services.Stats, error) {
	if period != "day" && period != "7d" && period != "30d" {
		return services.Stats{}, errors.New("统计时间范围无效")
	}
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return services.Stats{}, err
	}
	site, _, raw, err := s.PublicationSnapshot(siteID)
	if err != nil {
		return services.Stats{}, err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return services.Stats{}, err
	}
	cfg, err := services.LoadConfig(root, siteID, "plausible")
	if err != nil {
		return services.Stats{}, err
	}
	if cfg.Token == "" {
		return services.Stats{}, errors.New("请先配置 Plausible")
	}
	trackingEnabled := false
	trackingDomain, trackingServer := "", ""
	if site.PlausibleOverride != nil {
		trackingEnabled = site.PlausibleOverride.Enabled
		trackingDomain = site.PlausibleOverride.Domain
		trackingServer = site.PlausibleOverride.Server
	} else if len(raw) > 0 {
		enabled, domain, server, e := backup.PlausiblePublicSettings(raw)
		if e != nil {
			return services.Stats{}, e
		}
		trackingEnabled = enabled
		trackingDomain = domain
		trackingServer = server
	}
	trackingOrigin, e := services.PlausibleServerOrigin(trackingServer)
	if e != nil {
		return services.Stats{}, e
	}
	credentialOrigin, e := services.PlausibleServerOrigin(cfg.Server)
	if e != nil {
		return services.Stats{}, e
	}
	if !trackingEnabled || trackingDomain != cfg.Name || trackingOrigin != credentialOrigin {
		return services.Stats{}, errors.New("统计凭据与站点跟踪设置不一致，请重新保存 Plausible 配置")
	}
	return services.NewClient().PlausibleStatsAtRange(cfg.Server, cfg.Token, cfg.Name, period)
}

func (l *Library) RequestFilebasePin(siteID string) error {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return err
	}
	site, _, _, err := s.PublicationSnapshot(siteID)
	if err != nil {
		return err
	}
	if site.LastPublishedCID == "" {
		return errors.New("请先发布站点")
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	cfg, err := services.LoadConfig(root, siteID, "filebase")
	if err != nil {
		return err
	}
	if cfg.Token == "" {
		return errors.New("请先配置 Filebase")
	}
	return services.RetryPublished(root, siteID, site.LastPublishedCID, services.NewClient())
}
func (l *Library) FilebaseStatus(siteID string) (services.PinStatus, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return services.PinStatus{}, err
	}
	if _, _, _, err = s.PublicationSnapshot(siteID); err != nil {
		return services.PinStatus{}, err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return services.PinStatus{}, err
	}
	cfg, err := services.LoadConfig(root, siteID, "filebase")
	if err != nil {
		return services.PinStatus{}, err
	}
	if cfg.Token == "" || cfg.RequestID == "" {
		return services.PinStatus{}, errors.New("尚无已提交的固定请求")
	}
	status, err := services.NewClient().FilebaseStatus(cfg.Token, cfg.RequestID)
	if err != nil {
		return status, err
	}
	if status.Pin.CID != cfg.LastCID {
		return services.PinStatus{}, errors.New("服务报告的 CID 与本机记录不一致")
	}
	return status, nil
}

// ConfigurePinnable saves an explicitly supplied endpoint using Windows DPAPI.
// An empty endpoint disables the integration for this site.
func (l *Library) ConfigurePinnable(siteID, endpoint string) error {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return err
	}
	if _, _, _, err = s.PublicationSnapshot(siteID); err != nil {
		return err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	return pinnable.Save(root, siteID, strings.TrimSpace(endpoint))
}

func (l *Library) PinnableConfigured(siteID string) (bool, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return false, err
	}
	if _, _, _, err = s.PublicationSnapshot(siteID); err != nil {
		return false, err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return false, err
	}
	endpoint, err := pinnable.Load(root, siteID)
	return endpoint != "", err
}

func (l *Library) RequestPinnablePin(siteID string) error {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return err
	}
	site, _, _, err := s.PublicationSnapshot(siteID)
	if err != nil {
		return err
	}
	if site.IPNS == "" || site.LastPublishedCID == "" {
		return errors.New("请先发布站点")
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	endpoint, err := pinnable.Load(root, siteID)
	if err != nil {
		return err
	}
	if endpoint == "" {
		return errors.New("请先配置 Pinnable 地址")
	}
	return pinnable.NewClient().PinForSite(endpoint, site.IPNS, false)
}

func (l *Library) PinnableStatus(siteID string) (pinnable.Status, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return pinnable.Status{}, err
	}
	if _, _, _, err = s.PublicationSnapshot(siteID); err != nil {
		return pinnable.Status{}, err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return pinnable.Status{}, err
	}
	endpoint, err := pinnable.Load(root, siteID)
	if err != nil {
		return pinnable.Status{}, err
	}
	if endpoint == "" {
		return pinnable.Status{}, errors.New("请先配置 Pinnable 地址")
	}
	return pinnable.NewClient().Status(endpoint)
}

func (l *Library) templatePath(site store.Site) (string, error) {
	if site.TemplateID == "" {
		return "", nil
	}
	if !strings.HasPrefix(site.TemplateID, "template-") || !backup.SafeName(site.TemplateID) {
		return "", errors.New("模板目录无效")
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, "templates", site.TemplateID)
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("模板目录不存在或无效")
	}
	if err := templateinstall.Verify(path); err != nil {
		return "", fmt.Errorf("模板未通过验证：%w", err)
	}
	return path, nil
}

func (l *Library) SelectTemplate(ctx context.Context, siteID string) (store.Site, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: mygo.CallerWindow(ctx), Title: "选择 Planet 模板目录", Directory: true})
	if err != nil || len(paths) == 0 {
		return store.Site{}, err
	}
	return l.installTemplate(siteID, paths[0])
}

func (l *Library) BuiltinTemplates() []templateinstall.Builtin { return templateinstall.Builtins() }

func (l *Library) BuiltinTemplatePreview(id string) (string, error) {
	return templateinstall.BuiltinPreview(id)
}

func (l *Library) ApplyBuiltinTemplate(siteID, id string) (store.Site, error) {
	source, cleanup, err := templateinstall.ExtractBuiltin(id)
	if err != nil {
		return store.Site{}, err
	}
	defer cleanup()
	return l.installTemplate(siteID, source)
}

func (l *Library) installTemplate(siteID, source string) (store.Site, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return store.Site{}, err
	}
	raw, err := s.BackupMetadata(siteID)
	if err != nil {
		return store.Site{}, err
	}
	sites, err := s.ListSites()
	if err != nil {
		return store.Site{}, err
	}
	var site *store.Site
	for i := range sites {
		if sites[i].ID == siteID {
			site = &sites[i]
			break
		}
	}
	if site == nil {
		return store.Site{}, errors.New("站点不存在")
	}
	articles, err := s.ListArticles(siteID)
	if err != nil {
		return store.Site{}, err
	}
	if len(raw) > 0 {
		if err := backup.CheckSimplePlain(raw, *site, articles); err != nil {
			return store.Site{}, err
		}
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return store.Site{}, err
	}
	if _, err := templateinstall.Install(root, source, *site, articles, s); err != nil {
		return store.Site{}, err
	}
	updated, err := s.ListSites()
	if err != nil {
		return store.Site{}, err
	}
	for _, entry := range updated {
		if entry.ID == siteID {
			return entry, nil
		}
	}
	return store.Site{}, errors.New("站点不存在")
}

// TemplateSettings returns supported settings, including imported values.
func (l *Library) TemplateSettings(siteID string) (map[string]string, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return nil, err
	}
	site, _, raw, err := s.PublicationSnapshot(siteID)
	if err != nil {
		return nil, err
	}
	if site.TemplateName != "Sepia" || site.TemplateID == "" {
		return nil, errors.New("当前模板没有可编辑的设置")
	}
	settings := site.TemplateSettings
	if settings == nil && len(raw) > 0 {
		root, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			return nil, err
		}
		settings, err = migration.LoadTemplateSettings(filepath.Join(root, "imports", siteID), raw, site.TemplateName)
		if err != nil {
			return nil, err
		}
	}
	return sitegen.SepiaSettings(settings)
}

// SaveTemplateSettings validates values before persisting them locally.
func (l *Library) SaveTemplateSettings(siteID, values string) (store.Site, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	if len(values) > 1<<20 {
		return store.Site{}, errors.New("模板设置过大")
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal([]byte(values), &raw) != nil || raw == nil {
		return store.Site{}, errors.New("模板设置无效")
	}
	normalized, err := sitegen.NormalizeSettings("Sepia", raw)
	if err != nil {
		return store.Site{}, err
	}
	s, err := l.open()
	if err != nil {
		return store.Site{}, err
	}
	return s.SetTemplateSettings(siteID, normalized)
}

// ensureNode is called with publicMu held. Import/export only need an offline repository.
func (l *Library) ensureNode(online bool) (string, string, error) {
	if online {
		s, err := l.open()
		if err != nil {
			return "", "", err
		}
		if s.IPFSDisabled() {
			return "", "", errors.New("IPFS 节点已关闭，请在节点状态面板中开启")
		}
	}
	resources, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		return "", "", err
	}
	dataDir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", "", err
	}
	executable := filepath.Join(resources, "ipfs.exe")
	if _, err = os.Stat(executable); err != nil {
		return "", "", errors.New("缺少 Kubo 程序，请使用包含 Kubo 的安装包")
	}
	if l.node == nil {
		l.node = ipfs.New(executable, filepath.Join(dataDir, "ipfs"))
	}
	if !l.node.Alive() {
		if err = l.node.Init(); err != nil {
			return "", "", err
		}
		if online {
			if err = l.node.Start(false); err != nil {
				return "", "", err
			}
		}
	}
	return dataDir, executable, nil
}

// ImportPlanet imports a selected original .planet folder.
func (l *Library) ImportPlanet(ctx context.Context) (store.Site, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: mygo.CallerWindow(ctx), Title: "选择 .planet 备份目录", Directory: true})
	if err != nil {
		return store.Site{}, err
	}
	if len(paths) == 0 {
		return store.Site{}, nil
	}
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return store.Site{}, err
	}
	root, exe, err := l.ensureNode(false)
	if err != nil {
		return store.Site{}, err
	}
	return migration.New(root, exe, s, l.node).Import(paths[0])
}

// ExportPlanet writes a .planet backup containing the site's private key.
func (l *Library) ExportPlanet(ctx context.Context, siteID string) (string, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: mygo.CallerWindow(ctx), Title: "选择备份保存位置", Directory: true})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return "", err
	}
	root, exe, err := l.ensureNode(false)
	if err != nil {
		return "", err
	}
	return migration.New(root, exe, s, l.node).Export(siteID, paths[0])
}

func (l *Library) closeNode() {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	if l.node != nil {
		if err := l.node.Close(); err != nil {
			log.Printf("close Kubo: %v", err)
		}
	}
}

func (l *Library) resumeSiteDeletions() {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return
	}
	sites, err := s.RemovedSites()
	if err != nil {
		return
	}
	for _, site := range sites {
		if site.DeletionPending {
			if err := l.permanentlyDeleteSiteLocked(site.ID); err != nil {
				log.Printf("site deletion remains pending for %s: %v", site.ID, err)
			}
		}
	}
}

func (l *Library) maintainPublished(ctx context.Context) {
	l.resumeSiteDeletions()
	s, err := l.open()
	if err != nil {
		log.Printf("read library: %v", err)
		return
	}
	sites, err := s.ListSites()
	if err != nil {
		log.Printf("list sites: %v", err)
		return
	}
	for _, site := range sites {
		if site.LastPublishedCID != "" {
			l.publicMu.Lock()
			_, _, err = l.ensureNode(true)
			l.publicMu.Unlock()
			if err != nil {
				log.Printf("start IPFS node: %v", err)
			}
			break
		}
	}
	refresh := func() {
		l.publicMu.Lock()
		defer l.publicMu.Unlock()
		if ctx.Err() != nil || s.IPFSDisabled() {
			return
		}
		current, err := s.ListSites()
		if err != nil {
			log.Printf("list sites: %v", err)
			return
		}
		needsNode := false
		for _, site := range current {
			if site.LastPublishedCID != "" {
				needsNode = true
				break
			}
		}
		if !needsNode {
			return
		}
		if l.node == nil || !l.node.Alive() {
			if _, _, err = l.ensureNode(true); err != nil {
				log.Printf("start IPFS node: %v", err)
				return
			}
		}
		for _, site := range current {
			if ctx.Err() != nil {
				return
			}
			if site.PublishPending || site.IPNS == "" || site.LastPublishedCID == "" || !publisher.RenewDue(time.Now().UTC(), site.LastPublished) {
				continue
			}
			key, err := l.node.LookupKeyContext(ctx, site.ID)
			if err != nil || key != site.IPNS {
				log.Printf("skip IPNS renewal for %s: key mismatch or unavailable", site.ID)
				continue
			}
			name, err := l.node.PublishOnlineContext(ctx, site.ID, site.LastPublishedCID)
			if err != nil {
				log.Printf("IPNS renewal failed for %s: %v", site.ID, err)
				continue
			}
			if name != site.IPNS {
				log.Printf("IPNS renewal identity mismatch for %s", site.ID)
				continue
			}
			if err = s.SetPublished(site.ID, name, site.LastPublishedCID); err != nil {
				log.Printf("saving renewal for %s: %v", site.ID, err)
			}
		}
	}
	refresh()
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refresh()
		}
	}
}

// ExportWebsite creates a new folder containing a basic static website.
func (l *Library) ExportWebsite(ctx context.Context, siteID string) (string, error) {
	s, err := l.open()
	if err != nil {
		return "", err
	}
	sites, err := s.ListSites()
	if err != nil {
		return "", err
	}
	var selected *store.Site
	for i := range sites {
		if sites[i].ID == siteID {
			selected = &sites[i]
			break
		}
	}
	if selected == nil {
		return "", errors.New("站点不存在")
	}
	articles, err := s.ListArticles(siteID)
	if err != nil {
		return "", err
	}
	raw, err := s.BackupMetadata(siteID)
	if err != nil {
		return "", err
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: mygo.CallerWindow(ctx), Title: "选择网站导出位置", Directory: true})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	dir, err := os.MkdirTemp(paths[0], "Planet-website-")
	if err != nil {
		return "", err
	}
	templateDir, err := l.templatePath(*selected)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	if len(raw) > 0 {
		l.publicMu.Lock()
		defer l.publicMu.Unlock()
		*selected, articles, raw, err = s.PublicationSnapshot(siteID)
		if err == nil {
			templateDir, err = l.templatePath(*selected)
		}
		if err == nil && templateDir == "" {
			err = errors.New("请先选择已验证的模板")
		}
		if err == nil {
			err = migration.StageImportedPublic(filepath.Join(root, "imports", siteID), dir, raw, *selected, articles, templateDir)
		}
	} else {
		articles, err = attachments.EnrichMedia(root, *selected, articles)
		if err == nil && templateDir != "" {
			err = sitegen.RenderWithTemplate(dir, *selected, articles, templateDir)
		} else if err == nil {
			err = sitegen.Render(dir, *selected, articles)
		}
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	if len(raw) == 0 {
		err = attachments.CopyToSite(root, dir, articles)
		if err == nil {
			for _, article := range articles {
				if err = attachments.GenerateVideoThumbnail(root, *selected, article, dir); err != nil {
					break
				}
			}
		}
		if err == nil {
			err = sitegen.WriteAliases(dir, articles)
		}
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

func main() {
	// MyGo invokes the executable only to generate bindings. It must not
	// acquire the application or installer locks held by a running instance.
	if os.Getenv("MYGO_GENERATE") != "" {
		mygo.Bind(&Library{})
		if err := mygo.App.Run(); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--planet-version" {
		fmt.Println(appVersion)
		return
	}
	if executable, err := os.Executable(); err != nil {
		showStartupError(err)
		log.Fatal(err)
	} else if err = checkUpdateReady(executable); err != nil {
		showStartupError(err)
		log.Fatal(err)
	}
	mygo.App.SetName("Planet Windows")
	if mygo.IsDev() {
		path, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			log.Fatal(err)
		}
		mygo.App.SetPath(mygo.PathUserData, filepath.Join(path, "development"))
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	releaseUpdateLock, err := acquireUpdateLock()
	if err != nil {
		showStartupError(err)
		log.Fatal(err)
	}
	defer releaseUpdateLock()
	library := &Library{}
	maintenanceCtx, cancelMaintenance := context.WithCancel(context.Background())
	var maintenance sync.WaitGroup
	mygo.Bind(library)
	defer func() {
		library.closeUpdates()
		cancelMaintenance()
		maintenance.Wait()
		library.closeNode()
		library.closeAudio()
	}()
	mygo.App.WhenReady(func() {
		maintenance.Add(1)
		go func() { defer maintenance.Done(); library.maintainPublished(maintenanceCtx) }()
		win := mygo.NewWindow(mygo.WindowOptions{
			Title: "Planet", Width: 1280, Height: 820, MinWidth: 860, MinHeight: 600,
			BackgroundColor: "#f7f7f5", StateKey: "main", URL: "/",
		})
		var closing atomic.Bool
		win.OnClose(func(event *mygo.CloseEvent) {
			event.PreventDefault()
			if !closing.CompareAndSwap(false, true) {
				return
			}
			go func() {
				defer closing.Store(false)
				result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: win, Type: mygo.MessageQuestion, Title: "关闭 Planet", Message: "确定关闭窗口？", Detail: "尚未保存的文章更改将会丢失。", Buttons: []string{"取消", "关闭"}, DefaultButton: 0, CancelButton: 0})
				if err == nil && result.Button == 1 {
					win.Destroy()
				}
			}()
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
