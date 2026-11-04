package sitegen

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"windows/internal/backup"
	"windows/internal/preview"
	"windows/internal/services"
	"windows/internal/store"
	"windows/internal/templates"
)

// Original templates apply a JavaScript escape filter inside HTML attributes.
// Correct only the known social hrefs after rendering; the values are validated.
func renderSiteTemplate(dir, name string, ctx map[string]any, site store.Site) (string, error) {
	page, err := templates.Render(dir, name, ctx)
	if err != nil {
		return "", err
	}
	escapeJS := func(value string) string {
		var b strings.Builder
		for _, r := range value {
			if r < 32 || strings.ContainsRune(`\\'\"<>&=-;`, r) || r == '\u2028' || r == '\u2029' {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	for _, entry := range []struct{ prefix, value string }{{"https://twitter.com/", site.TwitterUsername}, {"https://github.com/", site.GithubUsername}, {"https://t.me/", site.TelegramUsername}, {"", site.DiscordLink}} {
		if entry.value == "" {
			continue
		}
		old := `href="` + entry.prefix + escapeJS(entry.value) + `"`
		fresh := `href="` + html.EscapeString(entry.prefix+entry.value) + `"`
		page = strings.ReplaceAll(page, old, fresh)
	}
	var missing strings.Builder
	for _, link := range socialLinks(site) {
		href := html.EscapeString(link.Href)
		if !strings.Contains(page, `href="`+href+`"`) {
			missing.WriteString(`<a href="` + href + `" target="_blank" rel="noopener noreferrer">` + html.EscapeString(link.Title) + `</a> `)
		}
	}
	if missing.Len() > 0 {
		if pos := strings.LastIndex(strings.ToLower(page), "</body>"); pos >= 0 {
			extra := `<nav class="planet-social-extra" aria-label="Social links">` + missing.String() + `</nav>`
			page = page[:pos] + extra + page[pos:]
		}
	}
	return page, nil
}

// RenderWithTemplate implements the index, article and archive subset of Planet templates.
func RenderWithTemplate(dir string, site store.Site, articles []store.Article, templateDir string) error {
	return RenderWithTemplateSettings(dir, site, articles, templateDir, site.TemplateSettings)
}

func RenderWithTemplateSettings(dir string, site store.Site, articles []store.Article, templateDir string, settings map[string]string) error {
	return renderWithTemplateSettings(dir, site, articles, templateDir, settings, "")
}

// RenderWithTemplateSettingsForOwnership supplies a private comparison marker;
// its output is evidence only and must never be published.
func RenderWithTemplateSettingsForOwnership(dir string, site store.Site, articles []store.Article, templateDir string, settings map[string]string, marker string) error {
	return renderWithTemplateSettings(dir, site, articles, templateDir, settings, marker)
}

func renderWithTemplateSettings(dir string, site store.Site, articles []store.Article, templateDir string, settings map[string]string, buildTimestamp string) error {
	site = store.ApplyPlausibleOverride(site)
	var err error
	if !site.Imported {
		site, err = normalizedSiteProfile(site)
		if err != nil {
			return err
		}
	}
	site, err = prepareTags(site, articles)
	if err != nil {
		return err
	}
	var metadata struct {
		Name       string `json:"name"`
		Ideal      int    `json:"idealItemsPerPage"`
		Pagination bool   `json:"generateIndexPagination"`
		Archive    bool   `json:"generateArchive"`
		Tags       bool   `json:"generateTagPages"`
	}
	b, err := os.ReadFile(filepath.Join(templateDir, "template.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &metadata); err != nil || metadata.Name == "" {
		return errors.New("模板信息无效")
	}
	seenTags := map[string]string{}
	for _, tags := range append([]map[string]string{site.Tags}, articleTagMaps(articles)...) {
		for key, label := range tags {
			if !backup.ValidTag(key, label) {
				return errors.New("标签名称或路径无效")
			}
			folded := backup.TagFoldKey(key)
			if old, ok := seenTags[folded]; ok && old != key {
				return errors.New("标签路径存在大小写冲突")
			}
			seenTags[folded] = key
		}
	}
	userSettings, err := settingsContext(metadata.Name, settings)
	if err != nil {
		return err
	}
	if err := Render(dir, site, articles); err != nil {
		return err
	}
	if metadata.Name == "Sepia" {
		b, err := json.MarshalIndent(userSettings, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "templateSettings.json"), append(b, '\n'), 0600); err != nil {
			return err
		}
		userSettings["highlightColorFilter"] = "url(#planet-highlight-filter)"
		userSettings["foregroundSecondaryColorFilter"] = "url(#planet-secondary-filter)"
	}
	assets := filepath.Join(templateDir, "assets")
	if info, err := os.Stat(assets); err == nil && info.IsDir() {
		if err := os.Mkdir(filepath.Join(dir, "assets"), 0700); err != nil {
			return err
		}
		if err := backup.CopyTree(assets, filepath.Join(dir, "assets")); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	cssHash := ""
	if css, err := os.ReadFile(filepath.Join(dir, "assets", "style.css")); err == nil {
		hash := sha256.Sum256(css)
		cssHash = hex.EncodeToString(hash[:])
	}
	planetTags := escapedTags(site.Tags)
	if metadata.Name == "Platinum" {
		// This pinned template explicitly escapes tag labels itself.
		planetTags = site.Tags
	}
	planet := map[string]any{"id": site.ID, "name": site.Name, "about": site.About, "slug": site.Slug, "authorName": site.AuthorName, "domain": site.Domain, "twitterUsername": site.TwitterUsername, "githubUsername": site.GithubUsername, "telegramUsername": site.TelegramUsername, "mastodonUsername": site.MastodonUsername, "discordLink": site.DiscordLink, "podcastLanguage": site.PodcastLanguage, "podcastExplicit": site.PodcastExplicit, "podcastCategories": podcastCategoryMap(site.PodcastCategories), "ipns": site.IPNS, "created": site.Created, "tags": planetTags, "plausibleEnabled": site.PlausibleEnabled, "plausibleDomain": site.PlausibleDomain, "plausibleAPIServer": site.PlausibleAPIServer}
	if site.PlausibleScriptID != "" {
		planet["plausibleEnabled"] = false
	}
	items := append([]store.Article{}, articles...)
	sort.Slice(items, func(i, j int) bool { return items[i].Created.After(items[j].Created) })
	rows := make([]map[string]any, 0, len(items))
	byID := map[string]map[string]any{}
	archive := map[string][]map[string]any{}
	sections := []string{}
	tagArticles := map[string][]map[string]any{}
	for _, a := range items {
		html, err := preview.Markdown(a.Content)
		if err != nil {
			return err
		}
		row := map[string]any{"id": a.ID, "link": articleLink(a), "title": a.Title, "content": a.Content, "contentRendered": html, "created": a.Created, "createdTimestamp": a.Created.Unix(), "modified": a.Modified, "attachments": a.Attachments, "articleReference": a.ArticleReference, "externalLink": a.ExternalLink, "articleType": a.ArticleType, "slug": a.Slug, "hasVideo": a.VideoFilename != "", "hasAudio": a.AudioFilename != "", "videoFilename": url.PathEscape(a.VideoFilename), "audioFilename": url.PathEscape(a.AudioFilename), "audioDuration": a.AudioDuration, "audioByteLength": a.AudioByteLength, "heroImage": nil, "tags": escapedTags(a.Tags)}
		if metadata.Name == "Platinum" || metadata.Name == "Sepia" {
			// These pinned templates escape article tag labels explicitly.
			row["tags"] = a.Tags
		}
		if metadata.Name == "Sepia" {
			// Sepia inserts names into a JSON array embedded in an HTML attribute.
			names := make([]string, 0, len(a.Attachments))
			for _, name := range a.Attachments {
				encoded, _ := json.Marshal(url.PathEscape(name))
				names = append(names, htmlEscapeJSON(encoded))
			}
			row["attachments"] = names
		}
		byID[a.ID] = row
		if a.ArticleType == 1 {
			continue
		}
		rows = append(rows, row)
		for key := range a.Tags {
			tagArticles[key] = append(tagArticles[key], row)
		}
		month := a.Created.In(time.Local).Format("January 2006")
		if _, ok := archive[month]; !ok {
			sections = append(sections, month)
		}
		archive[month] = append(archive[month], row)
	}
	aboutHTML, err := preview.Markdown(site.About)
	if err != nil {
		return err
	}
	avatar, avatarErr := os.Lstat(filepath.Join(dir, "avatar.png"))
	hasAvatar := avatarErr == nil && avatar.Mode().IsRegular()
	ogImage := ""
	if site.Domain != "" {
		ogImage = store.CanonicalSiteBase(site.Domain, site.IPNS) + "avatar.png"
	} else if site.IPNS != "" {
		ogImage = "https://" + site.IPNS + ".eth.sucks/avatar.png"
	}
	navigation := []map[string]any{}
	for _, n := range navigationItems(items, "") {
		title := n.Title
		if metadata.Name == "Grid" || metadata.Name == "8-bit" {
			title = html.EscapeString(title)
		}
		navigation = append(navigation, map[string]any{"id": n.ID, "title": title, "slug": n.Slug, "externalLink": n.ExternalLink, "weight": n.Weight})
	}
	_, podcastErr := os.Stat(filepath.Join(dir, "podcast.xml"))
	context := map[string]any{"planet": planet, "articles": rows, "assets_prefix": "./", "page_title": site.Name, "page_description": site.About, "page_description_html": aboutHTML, "article_summary": "", "current_item_type": "index", "site_navigation": navigation, "template_settings": map[string]any{}, "user_settings": userSettings, "og_image_url": ogImage, "style_css_sha256": cssHash, "custom_code_head": "", "custom_code_body_start": "", "custom_code_body_end": "", "total_pages": 1, "current_page": 1, "has_avatar": hasAvatar, "has_podcast": podcastErr == nil}
	if buildTimestamp != "" {
		context["build_timestamp"] = buildTimestamp
	}
	templatesDir := filepath.Join(templateDir, "templates")
	perPage := len(rows)
	if metadata.Pagination && metadata.Ideal > 0 {
		perPage = metadata.Ideal
	}
	if perPage == 0 {
		perPage = 1
	}
	totalPages := (len(rows) + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}
	for page := 1; page <= totalPages; page++ {
		ctx := map[string]any{}
		for k, v := range context {
			ctx[k] = v
		}
		start := (page - 1) * perPage
		end := start + perPage
		if end > len(rows) {
			end = len(rows)
		}
		ctx["articles"] = rows[start:end]
		ctx["current_page"] = page
		ctx["total_pages"] = totalPages
		if page > 1 {
			ctx["previous_page"] = fmt.Sprintf("page%d.html", page-1)
		}
		if page < totalPages {
			ctx["next_page"] = fmt.Sprintf("page%d.html", page+1)
		}
		index, err := renderSiteTemplate(templatesDir, "index.html", ctx, site)
		if err != nil {
			return fmt.Errorf("模板首页渲染失败：%w", err)
		}
		if metadata.Name == "Sepia" {
			index = sepiaIconFilters(index, userSettings)
		}
		index, err = withPlausibleScript(index, site)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(dir, fmt.Sprintf("page%d.html", page)), []byte(index), 0600); err != nil {
			return err
		}
		if page == 1 {
			if err = os.WriteFile(filepath.Join(dir, "index.html"), []byte(index), 0600); err != nil {
				return err
			}
		}
	}
	for _, a := range items {
		ctx := map[string]any{}
		for k, v := range context {
			ctx[k] = v
		}
		ctx["assets_prefix"] = "../"
		ctx["page_title"] = a.Title
		ctx["current_item_type"] = "article"
		if metadata.Name == "Sepia" {
			ctx["current_item_type"] = "blog"
		}
		ctx["article"] = byID[a.ID]
		ctx["article_id"] = a.ID
		ctx["article_type"] = a.ArticleType
		ctx["content_html"] = byID[a.ID]["contentRendered"]
		ctx["social_image_url"] = ogImage
		if site.IPNS != "" || site.Domain != "" {
			for _, name := range a.Attachments {
				switch strings.ToLower(filepath.Ext(name)) {
				case ".avif", ".jpeg", ".jpg", ".png", ".webp", ".gif", ".tiff", ".heic":
					base := "https://" + site.IPNS + ".eth.sucks/"
					if site.Domain != "" {
						base = store.CanonicalSiteBase(site.Domain, site.IPNS)
					}
					ctx["social_image_url"] = base + url.PathEscape(a.ID) + "/" + url.PathEscape(name)
				}
				if ctx["social_image_url"] != ogImage {
					break
				}
			}
		}
		page, err := renderSiteTemplate(templatesDir, "blog.html", ctx, site)
		if err != nil {
			return fmt.Errorf("模板文章渲染失败：%w", err)
		}
		if a.ExternalLink != "" && metadata.Name != "Plain" && metadata.Name != "Platinum" {
			target, marshalErr := json.Marshal(a.ExternalLink)
			if marshalErr != nil {
				return marshalErr
			}
			redirect := `<script>if(!/Planet\/\S*/.test(navigator.userAgent)){window.location.href=` + string(target) + `;}</script>`
			if pos := strings.Index(strings.ToLower(page), "</head>"); pos >= 0 {
				page = page[:pos] + redirect + page[pos:]
			} else {
				page = redirect + page
			}
		}
		if metadata.Name == "Sepia" {
			page = sepiaIconFilters(page, userSettings)
		}
		page, err = withPlausibleScript(page, site)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(dir, a.ID, "index.html"), []byte(page), 0600); err != nil {
			return err
		}
	}
	if metadata.Tags {
		for key, tagged := range tagArticles {
			ctx := map[string]any{}
			for k, v := range context {
				ctx[k] = v
			}
			ctx["articles"], ctx["tag_key"], ctx["tag_value"] = tagged, key, planetTags[key]
			ctx["current_item_type"], ctx["page_title"] = "tags", site.Name+" - "+site.Tags[key]
			page, err := renderSiteTemplate(templatesDir, "index.html", ctx, site)
			if err != nil {
				return fmt.Errorf("模板标签页面渲染失败：%w", err)
			}
			if metadata.Name == "Sepia" {
				page = sepiaIconFilters(page, userSettings)
			}
			page, err = withPlausibleScript(page, site)
			if err != nil {
				return err
			}
			if err = os.WriteFile(filepath.Join(dir, key+".html"), []byte(page), 0600); err != nil {
				return err
			}
		}
		if info, statErr := os.Lstat(filepath.Join(templatesDir, "tags.html")); statErr == nil {
			if !info.Mode().IsRegular() {
				return errors.New("模板标签索引无效")
			}
			ctx := map[string]any{}
			for k, v := range context {
				ctx[k] = v
			}
			ctx["tags"], ctx["tag_articles"], ctx["current_item_type"] = planetTags, tagArticles, "tags"
			page, err := renderSiteTemplate(templatesDir, "tags.html", ctx, site)
			if err != nil {
				return fmt.Errorf("模板标签索引渲染失败：%w", err)
			}
			if metadata.Name == "Sepia" {
				page = sepiaIconFilters(page, userSettings)
			}
			page, err = withPlausibleScript(page, site)
			if err != nil {
				return err
			}
			if err = os.WriteFile(filepath.Join(dir, "tags.html"), []byte(page), 0600); err != nil {
				return err
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
	}
	if metadata.Archive {
		ctx := map[string]any{}
		for k, v := range context {
			ctx[k] = v
		}
		ctx["page_title"] = site.Name + " - Archive"
		ctx["current_item_type"] = "archive"
		ctx["archive"] = archive
		ctx["archive_sections"] = sections
		page, err := renderSiteTemplate(templatesDir, "archive.html", ctx, site)
		if err != nil {
			return fmt.Errorf("模板归档渲染失败：%w", err)
		}
		if metadata.Name == "Sepia" {
			page = sepiaIconFilters(page, userSettings)
		}
		page, err = withPlausibleScript(page, site)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(dir, "archive.html"), []byte(page), 0600); err != nil {
			return err
		}
	}
	return nil
}

func escapedTags(tags map[string]string) map[string]string {
	result := make(map[string]string, len(tags))
	for key, value := range tags {
		result[key] = html.EscapeString(value)
	}
	return result
}

func articleTagMaps(articles []store.Article) []map[string]string {
	result := make([]map[string]string, 0, len(articles))
	for _, article := range articles {
		result = append(result, article.Tags)
	}
	return result
}

func withPlausibleScript(page string, site store.Site) (string, error) {
	if !site.PlausibleEnabled || site.PlausibleScriptID == "" {
		return page, nil
	}
	if !store.ValidPlausibleScriptID(site.PlausibleScriptID) || !strings.Contains(page, "</head>") {
		return "", errors.New("无法安全插入 Plausible 站点专属脚本")
	}
	origin, err := services.PlausibleServerOrigin(site.PlausibleAPIServer)
	if err != nil {
		return "", err
	}
	script := `<script async src="` + origin + `/js/` + site.PlausibleScriptID + `.js"></script><script>window.plausible=window.plausible||function(){(plausible.q=plausible.q||[]).push(arguments)},plausible.init=plausible.init||function(i){plausible.o=i||{}};plausible.init()</script>`
	return strings.Replace(page, "</head>", script+"</head>", 1), nil
}

// SVG flood filters tint icon alpha masks exactly to the chosen color.
// Both colors have already passed the strict six-digit hex validator.
func sepiaIconFilters(page string, settings map[string]any) string {
	defs := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" aria-hidden="true" style="position:absolute;width:0;height:0;overflow:hidden"><defs><filter id="planet-highlight-filter" color-interpolation-filters="sRGB"><feFlood flood-color="%s" result="color"/><feComposite in="color" in2="SourceGraphic" operator="in"/></filter><filter id="planet-secondary-filter" color-interpolation-filters="sRGB"><feFlood flood-color="%s" result="color"/><feComposite in="color" in2="SourceGraphic" operator="in"/></filter></defs></svg>`, settings["highlightColor"], settings["foregroundSecondaryColor"])
	if strings.Contains(page, "</body>") {
		return strings.Replace(page, "</body>", defs+"</body>", 1)
	}
	return defs + page
}

func htmlEscapeJSON(encoded []byte) string {
	if len(encoded) < 2 {
		return ""
	}
	return html.EscapeString(string(encoded[1 : len(encoded)-1]))
}
