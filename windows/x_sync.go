package main

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"windows/internal/services"
	"windows/internal/store"
)

// XSyncInfo deliberately contains neither credentials nor API response bodies.
type XSyncInfo struct {
	Configured bool            `json:"configured"`
	AutoSync   bool            `json:"autoSync"`
	Posts      []XSyncPostInfo `json:"posts"`
}

type XSyncPostInfo struct {
	ArticleID string `json:"articleId"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	URL       string `json:"url"`
	Error     string `json:"error"`
	CanSync   bool   `json:"canSync"`
}

func (l *Library) xSyncSite(siteID string) (store.Site, []store.Article, string, error) {
	s, err := l.open()
	if err != nil {
		return store.Site{}, nil, "", err
	}
	site, articles, _, err := s.PublicationSnapshot(siteID)
	if err != nil {
		return store.Site{}, nil, "", err
	}
	if site.Imported {
		return store.Site{}, nil, "", errors.New("导入站点暂不支持 X 同步")
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	return site, articles, root, err
}

func xPublishedCurrent(site store.Site) bool {
	return site.LastPublishedCID != "" && site.LastPublished != nil && !site.PublishPending && site.PublishedRevision == site.ContentRevision
}

func xPostInfo(site store.Site, article store.Article, record services.XPostRecord, configured bool) XSyncPostInfo {
	return XSyncPostInfo{ArticleID: article.ID, Title: article.Title, Status: record.Status, URL: record.URL, Error: record.Error,
		CanSync: configured && article.ArticleType == 0 && xPublishedCurrent(site) && record.Status != "sent"}
}

func (l *Library) XSyncInfo(siteID string) (XSyncInfo, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	site, articles, root, err := l.xSyncSite(siteID)
	if err != nil {
		return XSyncInfo{}, err
	}
	cfg, err := services.LoadConfig(root, siteID, "x")
	if err != nil {
		return XSyncInfo{}, err
	}
	info := XSyncInfo{Configured: cfg.XCredentials != nil, AutoSync: cfg.AutoSync, Posts: []XSyncPostInfo{}}
	for _, article := range articles {
		if article.ArticleType != 0 {
			continue
		}
		record, e := services.LoadXPost(root, siteID, article.ID)
		if e != nil {
			return XSyncInfo{}, e
		}
		info.Posts = append(info.Posts, xPostInfo(site, article, record, info.Configured))
	}
	return info, nil
}

// ConfigureXSync only stores local credentials. It never sends a post.
func (l *Library) ConfigureXSync(siteID, apiKey, apiSecret, accessToken, accessTokenSecret string, autoSync bool) error {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	_, _, root, err := l.xSyncSite(siteID)
	if err != nil {
		return err
	}
	cfg, err := services.LoadConfig(root, siteID, "x")
	if err != nil {
		return err
	}
	keys := services.XCredentials{APIKey: strings.TrimSpace(apiKey), APISecret: strings.TrimSpace(apiSecret), AccessToken: strings.TrimSpace(accessToken), AccessTokenSecret: strings.TrimSpace(accessTokenSecret)}
	if keys == (services.XCredentials{}) {
		if cfg.XCredentials == nil {
			return errors.New("请填写 X 的四项凭据")
		}
		keys = *cfg.XCredentials
	} else if !services.ValidXCredentials(keys) {
		return errors.New("请完整填写 X 的四项凭据")
	}
	if cfg.XCredentials == nil || autoSync && !cfg.AutoSync || cfg.XEnabledAt.IsZero() {
		cfg.XEnabledAt = time.Now().UTC()
	}
	cfg.Name, cfg.Token, cfg.XCredentials, cfg.AutoSync = "X", keys.AccessToken, &keys, autoSync
	return services.SaveConfig(root, siteID, "x", cfg)
}

// DisableXSync retains delivery records so reconnecting never resends old posts.
func (l *Library) DisableXSync(siteID string) error {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	_, _, root, err := l.xSyncSite(siteID)
	if err != nil {
		return err
	}
	return services.SaveConfig(root, siteID, "x", services.Config{})
}

func xArticleURL(site store.Site, article store.Article) (string, error) {
	base := store.CanonicalSiteBase(site.Domain, site.IPNS)
	if !strings.HasPrefix(base, "https://") {
		return "", errors.New("文章尚无公开链接，请先发布站点")
	}
	path := article.ID
	if article.Slug != "" {
		path = article.Slug
	}
	return base + url.PathEscape(path) + "/", nil
}

func (l *Library) SyncXArticle(siteID, articleID string, retry bool) (XSyncPostInfo, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	site, articles, root, err := l.xSyncSite(siteID)
	if err != nil {
		return XSyncPostInfo{}, err
	}
	cfg, err := services.LoadConfig(root, siteID, "x")
	if err != nil {
		return XSyncPostInfo{}, err
	}
	if cfg.XCredentials == nil {
		return XSyncPostInfo{}, errors.New("请先配置 X 同步")
	}
	for _, article := range articles {
		if article.ID != articleID {
			continue
		}
		if article.ArticleType != 0 {
			return XSyncPostInfo{}, errors.New("页面不参与 X 文章同步")
		}
		record, err := services.LoadXPost(root, siteID, articleID)
		if err != nil {
			return XSyncPostInfo{}, err
		}
		if record.Status == "sent" {
			return xPostInfo(site, article, record, true), nil
		}
		if !xPublishedCurrent(site) {
			return XSyncPostInfo{}, errors.New("请先发布站点的当前保存版本，再同步到 X")
		}
		record, err = l.sendXArticle(root, site, article, cfg, record, retry)
		return xPostInfo(site, article, record, true), err
	}
	return XSyncPostInfo{}, errors.New("文章不存在")
}

// sendXArticle runs under publicMu for both automatic and manual callers.
func (l *Library) sendXArticle(root string, site store.Site, article store.Article, cfg services.Config, record services.XPostRecord, retry bool) (services.XPostRecord, error) {
	if record.Status == "sent" {
		return record, nil
	}
	if record.Status != "" && !retry {
		return record, errors.New("上次 X 请求尚未成功，请检查 X 后手动重试")
	}
	link, err := xArticleURL(site, article)
	if err != nil {
		return record, err
	}
	text, err := services.XPostText(article.Title, link)
	if err != nil {
		return record, err
	}
	record = services.XPostRecord{Status: "pending"}
	if err = services.SaveXPost(root, site.ID, article.ID, record); err != nil {
		return record, errors.New("无法保存 X 请求状态，未发送")
	}
	poster := l.xPoster
	if poster == nil {
		poster = services.NewClient()
	}
	id, err := poster.CreateXPost(*cfg.XCredentials, text)
	if err != nil {
		if services.XPostRejected(err) {
			record.Status = "failed"
		}
		record.Error = services.XPostMessage(err)
		if saveErr := services.SaveXPost(root, site.ID, article.ID, record); saveErr != nil {
			return record, errors.New("X 请求未确认且本机状态保存失败，请先到 X 检查")
		}
		return record, errors.New(record.Error)
	}
	record = services.XPostRecord{Status: "sent", PostID: id, URL: "https://x.com/i/web/status/" + id}
	if err = services.SaveXPost(root, site.ID, article.ID, record); err != nil {
		return services.XPostRecord{Status: "pending"}, errors.New("X 已接收帖子，但保存本机记录失败，请先到 X 检查，勿直接重试")
	}
	return record, nil
}

// syncPublishedX receives precisely the successful publication snapshot. X is
// independent of IPNS success and never participates in the IPNS retry loop.
func (l *Library) syncPublishedX(root string, site store.Site, articles []store.Article) string {
	if site.Imported {
		return ""
	}
	cfg, err := services.LoadConfig(root, site.ID, "x")
	if err != nil {
		return "无法读取 X 同步配置：" + err.Error()
	}
	if cfg.XCredentials == nil || !cfg.AutoSync {
		return ""
	}
	for _, article := range articles {
		if article.ArticleType != 0 || article.Created.Before(cfg.XEnabledAt) {
			continue
		}
		record, err := services.LoadXPost(root, site.ID, article.ID)
		if err != nil {
			return "无法读取 X 同步记录：" + err.Error()
		}
		if record.Status != "" {
			continue
		}
		if _, err = l.sendXArticle(root, site, article, cfg, record, false); err != nil {
			// Stop this batch on the first failure; later publications can send
			// remaining new articles, but never retry this uncertain request.
			return fmt.Sprintf("文章“%s”未同步到 X：%s", article.Title, err)
		}
	}
	return ""
}
