package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"windows/internal/attachments"
	"windows/internal/backup"
	"windows/internal/feeds"
	"windows/internal/ipfs"
	"windows/internal/migration"
	"windows/internal/preview"
	"windows/internal/publisher"
	"windows/internal/sitegen"
	"windows/internal/store"
	"windows/internal/templateinstall"
)

// Library exposes the local authoring functions to the application window.
type Library struct {
	once     sync.Once
	store    *store.Store
	err      error
	publicMu sync.Mutex
	node     *ipfs.Client
}

func (l *Library) open() (*store.Store, error) {
	l.once.Do(func() {
		var path string
		path, l.err = mygo.App.Path(mygo.PathUserData)
		if l.err == nil {
			l.store, l.err = store.Open(path)
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
func (l *Library) CreateSite(name, about string) (store.Site, error) {
	s, err := l.open()
	if err != nil {
		return store.Site{}, err
	}
	return s.CreateSite(name, about)
}
func (l *Library) Articles(siteID string) ([]store.Article, error) {
	s, err := l.open()
	if err != nil {
		return nil, err
	}
	return s.ListArticles(siteID)
}
func (l *Library) SaveArticle(siteID, articleID, title, content string) (store.Article, error) {
	s, err := l.open()
	if err != nil {
		return store.Article{}, err
	}
	return s.SaveArticle(siteID, articleID, title, content)
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
func (l *Library) Preview(content string) (string, error) { return preview.Markdown(content) }

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

func (l *Library) Follow(address string) (store.Subscription, error) {
	feed, err := feeds.Fetch(address)
	if err != nil {
		return store.Subscription{}, err
	}
	s, err := l.open()
	if err != nil {
		return store.Subscription{}, err
	}
	return s.SaveSubscription(address, feed)
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
			return publisher.Result{}, errors.New("发布导入站点前，请先选择已验证的 Plain 模板")
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
	return result, nil
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

func (l *Library) SelectTemplate(ctx context.Context, siteID string) (string, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: mygo.CallerWindow(ctx), Title: "选择 Planet 模板目录", Directory: true})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	s, err := l.open()
	if err != nil {
		return "", err
	}
	raw, err := s.BackupMetadata(siteID)
	if err != nil {
		return "", err
	}
	sites, err := s.ListSites()
	if err != nil {
		return "", err
	}
	var site *store.Site
	for i := range sites {
		if sites[i].ID == siteID {
			site = &sites[i]
			break
		}
	}
	if site == nil {
		return "", errors.New("站点不存在")
	}
	articles, err := s.ListArticles(siteID)
	if err != nil {
		return "", err
	}
	if len(raw) > 0 {
		if err := backup.CheckSimplePlain(raw, *site, articles); err != nil {
			return "", err
		}
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	if _, err := templateinstall.Install(root, paths[0], *site, articles, s); err != nil {
		return "", err
	}
	updated, err := s.ListSites()
	if err != nil {
		return "", err
	}
	for _, entry := range updated {
		if entry.ID == siteID {
			return entry.TemplateName, nil
		}
	}
	return "", errors.New("站点不存在")
}

// ensureNode is called with publicMu held. Import/export only need an offline repository.
func (l *Library) ensureNode(online bool) (string, string, error) {
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

func (l *Library) maintainPublished(ctx context.Context) {
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
		if ctx.Err() != nil {
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
	if len(raw) > 0 {
		return "", errors.New("已导入站点的网页导出暂不可用，避免丢失模板或附件")
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
	if templateDir != "" {
		err = sitegen.RenderWithTemplate(dir, *selected, articles, templateDir)
	} else {
		err = sitegen.Render(dir, *selected, articles)
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	if err == nil {
		err = attachments.CopyToSite(root, dir, articles)
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

func main() {
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
	library := &Library{}
	maintenanceCtx, cancelMaintenance := context.WithCancel(context.Background())
	var maintenance sync.WaitGroup
	mygo.Bind(library)
	defer func() { cancelMaintenance(); maintenance.Wait(); library.closeNode() }()
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
