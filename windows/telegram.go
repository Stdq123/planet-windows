package main

import (
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"

	"github.com/egoist/mygo"
	xhtml "golang.org/x/net/html"
	"windows/internal/backup"
	"windows/internal/preview"
	"windows/internal/services"
	"windows/internal/store"
)

type TelegramInfo struct {
	Configured  bool   `json:"configured"`
	ChatID      string `json:"chatId"`
	ChatTitle   string `json:"chatTitle"`
	SyncedCount int    `json:"syncedCount"`
	Pending     bool   `json:"pending"`
}

func (l *Library) telegramClient() services.TelegramAPI {
	if l.telegramAPI != nil {
		return l.telegramAPI
	}
	return services.NewTelegramClient()
}

func (l *Library) telegramSite(siteID string) (store.Site, []store.Article, json.RawMessage, string, error) {
	s, err := l.open()
	if err != nil {
		return store.Site{}, nil, nil, "", err
	}
	site, articles, raw, err := s.PublicationSnapshot(siteID)
	if err != nil {
		return store.Site{}, nil, nil, "", err
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	return site, articles, raw, root, err
}

func (l *Library) TelegramInfo(siteID string) (TelegramInfo, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	_, _, _, root, err := l.telegramSite(siteID)
	if err != nil {
		return TelegramInfo{}, err
	}
	cfg, err := services.LoadTelegramConfig(root, siteID)
	if err != nil {
		return TelegramInfo{}, err
	}
	info := TelegramInfo{Configured: cfg.Token != "", ChatID: cfg.ChatID, ChatTitle: cfg.ChatTitle}
	for _, message := range cfg.Messages {
		if message.MessageID > 0 {
			info.SyncedCount++
		}
		info.Pending = info.Pending || message.Pending
	}
	return info, nil
}

// ConfigureTelegram only resolves the target. Publication is a separate action.
func (l *Library) ConfigureTelegram(siteID, chatID, token string) error {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	_, _, _, root, err := l.telegramSite(siteID)
	if err != nil {
		return err
	}
	chatID, token = strings.TrimSpace(chatID), strings.TrimSpace(token)
	if !services.ValidTelegramTarget(chatID) {
		return errors.New("请输入 Telegram 群组或频道的负数 ID 或 @username")
	}
	old, err := services.LoadTelegramConfig(root, siteID)
	if err != nil {
		return err
	}
	if token == "" {
		if old.Token == "" || old.ChatID != chatID {
			return errors.New("首次配置或更换目标时请填写 Bot Token")
		}
		token = old.Token
	}
	if !services.ValidTelegramToken(token) {
		return errors.New("Telegram Bot Token 格式无效")
	}
	chat, err := l.telegramClient().ResolveChat(token, chatID)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(chat.ID, "-") || !services.ValidTelegramTarget(chat.ID) || (chat.Type != "group" && chat.Type != "supergroup" && chat.Type != "channel") {
		return errors.New("Telegram 目标必须是群组或频道")
	}
	cfg := services.TelegramConfig{Token: token, ChatID: chat.ID, ChatTitle: chat.Title}
	// Token rotation for the same bot keeps the bot's existing messages.
	if old.ChatID == chat.ID && strings.SplitN(old.Token, ":", 2)[0] == strings.SplitN(token, ":", 2)[0] {
		cfg.Messages = old.Messages
	}
	return services.SaveTelegramConfig(root, siteID, cfg)
}

func (l *Library) DisableTelegram(siteID string) error {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	_, _, _, root, err := l.telegramSite(siteID)
	if err != nil {
		return err
	}
	return services.DeleteTelegramConfig(root, siteID)
}

func (l *Library) RetryTelegram(siteID string) (int, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	site, articles, raw, root, err := l.telegramSite(siteID)
	if err != nil {
		return 0, err
	}
	if site.LastPublishedCID == "" || site.IPNS == "" || site.PublishPending || site.ContentRevision != site.PublishedRevision {
		return 0, errors.New("请先成功发布当前站点版本，再同步到 Telegram")
	}
	cfg, err := services.LoadTelegramConfig(root, siteID)
	if err != nil {
		return 0, err
	}
	if cfg.Token == "" {
		return 0, errors.New("请先配置 Telegram 同步")
	}
	posts, err := telegramSnapshotPosts(site, articles, raw)
	if err != nil {
		return 0, err
	}
	return services.SyncTelegram(root, siteID, posts, true, l.telegramClient())
}

// Called only with the captured snapshot after successful IPNS publication.
func (l *Library) syncPublishedTelegram(root string, site store.Site, articles []store.Article, raw json.RawMessage) string {
	cfg, err := services.LoadTelegramConfig(root, site.ID)
	if err != nil {
		return "站点已发布，但无法读取 Telegram 配置：" + err.Error()
	}
	if cfg.Token == "" {
		return ""
	}
	posts, err := telegramSnapshotPosts(site, articles, raw)
	if err == nil {
		_, err = services.SyncTelegram(root, site.ID, posts, false, l.telegramClient())
	}
	if err != nil {
		return "站点已发布，但 Telegram 同步未完成：" + err.Error()
	}
	return ""
}

func telegramSnapshotPosts(site store.Site, articles []store.Article, raw json.RawMessage) ([]services.TelegramPost, error) {
	if len(raw) > 0 {
		var original struct {
			ID       string `json:"id"`
			Articles []struct {
				ID   string `json:"id"`
				Slug string `json:"slug"`
			} `json:"articles"`
		}
		if json.Unmarshal(raw, &original) != nil || !strings.EqualFold(original.ID, site.ID) {
			return nil, errors.New("无法读取导入站点的文章链接")
		}
		slugs := make(map[string]string)
		for _, entry := range original.Articles {
			if entry.Slug != "" && !backup.ValidArticleSlug(entry.Slug) {
				return nil, errors.New("导入文章的自定义路径无效")
			}
			slugs[strings.ToUpper(entry.ID)] = entry.Slug
		}
		articles = append([]store.Article(nil), articles...)
		for i := range articles {
			if slug, ok := slugs[strings.ToUpper(articles[i].ID)]; ok {
				articles[i].Slug = slug
			}
		}
	}
	return telegramPosts(site, articles)
}

func telegramPosts(site store.Site, articles []store.Article) ([]services.TelegramPost, error) {
	base := store.CanonicalSiteBase(site.Domain, site.IPNS)
	if base == "/" {
		return nil, errors.New("Telegram 文章链接需要已发布的站点地址")
	}
	ordered := append([]store.Article(nil), articles...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Created.Equal(ordered[j].Created) {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].Created.Before(ordered[j].Created)
	})
	posts := make([]services.TelegramPost, 0, len(ordered))
	for _, article := range ordered {
		if article.ArticleType == 1 {
			continue
		}
		if article.SiteID != site.ID {
			return nil, errors.New("Telegram 文章与站点不匹配")
		}
		path := article.Slug
		if path == "" {
			path = article.ID
		}
		summary, err := telegramSummary(article.Content)
		if err != nil {
			return nil, err
		}
		title := telegramExcerpt(strings.Join(strings.Fields(article.Title), " "), 200)
		if title == "" {
			title = "无标题文章"
		}
		text := title + "\n\n"
		if summary != "" {
			text += summary + "\n\n"
		}
		text += base + url.PathEscape(path) + "/"
		posts = append(posts, services.TelegramPost{ID: article.ID, Text: text})
	}
	return posts, nil
}

func telegramExcerpt(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit-1]) + "…"
}

func telegramSummary(markdown string) (string, error) {
	rendered, err := preview.Markdown(markdown)
	if err != nil {
		return "", errors.New("无法生成 Telegram 文章摘要")
	}
	node, err := xhtml.Parse(strings.NewReader(rendered))
	if err != nil {
		return "", errors.New("无法生成 Telegram 文章摘要")
	}
	var out strings.Builder
	var visit func(*xhtml.Node)
	visit = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && (n.Data == "script" || n.Data == "style") {
			return
		}
		if n.Type == xhtml.TextNode {
			out.WriteString(n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
		if n.Type == xhtml.ElementNode {
			switch n.Data {
			case "p", "div", "br", "li", "pre", "blockquote", "h1", "h2", "h3", "h4", "h5", "h6", "tr", "td", "th":
				out.WriteByte(' ')
			}
		}
	}
	visit(node)
	return telegramExcerpt(strings.Join(strings.Fields(out.String()), " "), 600), nil
}
