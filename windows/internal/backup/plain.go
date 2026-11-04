package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"windows/internal/services"
	"windows/internal/store"
)

var unsupportedMarkup = regexp.MustCompile(`(?i)<[a-z!/][^>]*>|https?://(?:www\.)?(?:youtube\.com|youtu\.be)/`)
var safeArticleSlug = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func ValidArticleSlug(slug string) bool {
	return safeArticleSlug.MatchString(slug) && SafeName(slug) && !strings.EqualFold(slug, "assets") && !strings.EqualFold(slug, "tags")
}

var plausibleHost = regexp.MustCompile(`(?i)^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*$`)

// PlausibleStoredSettings also preserves a disabled original configuration so
// the UI never offers the official service in place of a self-hosted one.
func PlausibleStoredSettings(original json.RawMessage) (bool, string, string, error) {
	var root map[string]json.RawMessage
	if json.Unmarshal(original, &root) != nil || root == nil {
		return false, "", "", errors.New("备份元数据无效")
	}
	var enabled bool
	var domain, server string
	for key, target := range map[string]any{"plausibleEnabled": &enabled, "plausibleDomain": &domain, "plausibleAPIServer": &server} {
		if raw := root[key]; len(raw) > 0 && string(raw) != "null" {
			if json.Unmarshal(raw, target) != nil {
				return false, "", "", errors.New("原版 Plausible 配置无效")
			}
		}
	}
	domain = strings.TrimSpace(domain)
	if domain != "" && !store.ValidPlausibleDomain(domain) {
		return false, "", "", errors.New("原版 Plausible 域名无效")
	}
	origin, err := services.PlausibleServerOrigin(server)
	if err != nil {
		return false, "", "", err
	}
	return enabled, domain, origin, nil
}

// PlausiblePublicSettings admits only validated public fields, never the API key.
func PlausiblePublicSettings(original json.RawMessage) (bool, string, string, error) {
	var root map[string]json.RawMessage
	if json.Unmarshal(original, &root) != nil || root == nil {
		return false, "", "", errors.New("备份元数据无效")
	}
	var enabled bool
	if raw, ok := root["plausibleEnabled"]; ok && string(raw) != "null" {
		if json.Unmarshal(raw, &enabled) != nil {
			return false, "", "", errors.New("Plausible 启用状态无效")
		}
	}
	values := map[string]string{}
	for _, key := range []string{"plausibleDomain", "plausibleAPIServer", "plausibleAPIKey"} {
		if raw, ok := root[key]; ok && string(raw) != "null" {
			var value string
			if json.Unmarshal(raw, &value) != nil {
				return false, "", "", errors.New("Plausible 配置无效")
			}
			values[key] = value
		}
	}
	if !enabled {
		return false, "", "", nil
	}
	domain, server := strings.TrimSpace(values["plausibleDomain"]), strings.TrimSpace(values["plausibleAPIServer"])
	origin, err := services.PlausibleServerOrigin(server)
	if err != nil || len(domain) > 253 || !plausibleHost.MatchString(domain) {
		return false, "", "", errors.New("Plausible 域名或服务地址不受支持")
	}
	return true, domain, strings.TrimPrefix(origin, "https://"), nil
}

func populated(raw json.RawMessage) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return true
	}
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	default:
		return true
	}
}

// CheckSimplePlain admits only fields whose rendering has been implemented.
// Unknown populated fields are rejected, even though they remain in the backup.
func CheckSimplePlain(original json.RawMessage, site store.Site, articles []store.Article) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(original, &root); err != nil || root == nil {
		return errors.New("备份元数据无效")
	}
	var p rawPlanet
	if err := json.Unmarshal(original, &p); err != nil {
		return err
	}
	if p.TemplateName != "Plain" && p.TemplateName != "8-bit" && p.TemplateName != "Grid" && p.TemplateName != "Platinum" && p.TemplateName != "Sepia" {
		return errors.New("当前只能重建使用已支持模板且站点设置兼容的备份")
	}
	if site.PlausibleOverride == nil {
		if _, _, _, err := PlausiblePublicSettings(original); err != nil {
			return err
		}
	} else if site.PlausibleOverride.Enabled && (!store.ValidPlausibleDomain(site.PlausibleOverride.Domain) || !store.ValidPlausibleScriptID(site.PlausibleOverride.ScriptID)) {
		return errors.New("Plausible 站点域名无效")
	}
	if site.PlausibleOverride != nil && site.PlausibleOverride.Enabled {
		if _, err := services.PlausibleServerOrigin(site.PlausibleOverride.Server); err != nil {
			return err
		}
	}
	if unsupportedMarkup.MatchString(site.About) {
		return errors.New("站点简介包含暂不支持重建的内容")
	}
	if _, _, err := ImportedTags(original, articles); err != nil {
		return err
	}
	allowedSite := map[string]bool{"id": true, "name": true, "about": true, "slug": true, "tags": true, "ipns": true, "created": true, "updated": true, "templateName": true, "articles": true, "lastPublished": true, "lastPublishedCID": true, "nextArticleNumber": true, "publishAsIPNS": true, "windowsPlausibleScriptID": true}
	knownSite := strings.Fields("domain authorName slug archived archivedAt plausibleEnabled plausibleDomain plausibleAPIKey plausibleAPIServer twitterUsername githubUsername telegramUsername mastodonUsername discordLink dWebServicesEnabled dWebServicesDomain dWebServicesAPIKey pinnableEnabled pinnableAPIEndpoint pinnablePinCID filebaseEnabled filebasePinName filebaseAPIToken filebaseRequestID filebasePinCID customCodeHeadEnabled customCodeHead customCodeBodyStartEnabled customCodeBodyStart customCodeBodyEndEnabled customCodeBodyEnd podcastCategories podcastLanguage podcastExplicit juiceboxEnabled juiceboxProjectID juiceboxProjectIDGoerli acceptsDonation acceptsDonationMessage acceptsDonationETHAddress tags aggregation reuseOriginalID saveRoundAvatar doNotIndex prewarmNewPost sshRsyncEnabled sshRsyncDestination sshRsyncKeyPath sshRsyncDeleteEnabled cloudflarePagesEnabled cloudflarePagesAccountID cloudflarePagesAPIToken cloudflarePagesProjectName cloudflarePagesLastDeployedProjectName cloudflarePagesLastDeployedURL")
	knownSiteSet := map[string]bool{}
	for _, key := range knownSite {
		knownSiteSet[key] = true
	}
	// Disabled Mac custom code is retained in private backup metadata only.
	// The supported renderers do not execute or publish these stored snippets.
	customCodeFields := map[string]bool{}
	for _, name := range []string{"customCodeHead", "customCodeBodyStart", "customCodeBodyEnd"} {
		enabledKey := name + "Enabled"
		customCodeFields[name], customCodeFields[enabledKey] = true, true
		if value := root[enabledKey]; len(value) > 0 && string(value) != "null" {
			var enabled bool
			if json.Unmarshal(value, &enabled) != nil || enabled {
				return fmt.Errorf("站点自定义代码 %s 已启用或无效", name)
			}
		}
		if value := root[name]; len(value) > 0 && string(value) != "null" {
			var code string
			if json.Unmarshal(value, &code) != nil {
				return fmt.Errorf("站点自定义代码 %s 无效", name)
			}
		}
	}
	for key, value := range root {
		if customCodeFields[key] {
			continue
		}
		if key == "prewarmNewPost" {
			// A private Mac publication preference; it does not affect the
			// generated site. Preserve it for the original application's use.
			if string(value) != "null" {
				var enabled bool
				if json.Unmarshal(value, &enabled) != nil {
					return errors.New("原预热设置无效")
				}
			}
			continue
		}
		if key == "windowsPreviousPlausibleConfig" {
			var previous []json.RawMessage
			if json.Unmarshal(value, &previous) != nil {
				return errors.New("备份中的历史 Plausible 配置无效")
			}
			continue
		}
		if key == "windowsPlausibleScriptID" {
			var id string
			if json.Unmarshal(value, &id) != nil || !store.ValidPlausibleScriptID(id) {
				return errors.New("备份中的 Plausible 脚本 ID 无效")
			}
			continue
		}
		if key == "plausibleEnabled" || key == "plausibleDomain" || key == "plausibleAPIServer" || key == "plausibleAPIKey" {
			continue
		}
		// Windows integrations are separately opted in. Original pinning fields
		// remain private backup metadata and are never passed to renderers.
		if key == "pinnableEnabled" || key == "pinnableAPIEndpoint" || key == "pinnablePinCID" || key == "filebaseEnabled" || key == "filebasePinName" || key == "filebaseAPIToken" || key == "filebaseRequestID" || key == "filebasePinCID" {
			if string(value) != "null" {
				if key == "pinnableEnabled" || key == "filebaseEnabled" {
					var enabled bool
					if json.Unmarshal(value, &enabled) != nil {
						return errors.New("原固定服务标志无效")
					}
				} else {
					var text string
					if json.Unmarshal(value, &text) != nil {
						return errors.New("原固定服务配置无效")
					}
				}
			}
			continue
		}
		if !allowedSite[key] && (!knownSiteSet[key] || populated(value)) {
			return fmt.Errorf("站点设置 %s 暂不支持重建", key)
		}
	}
	var old []map[string]json.RawMessage
	if err := json.Unmarshal(root["articles"], &old); err != nil {
		return err
	}
	paths := map[string]bool{"assets": true, "tags": true}
	for _, article := range articles {
		paths[strings.ToLower(article.ID)] = true
	}
	for _, entry := range old {
		var id string
		if json.Unmarshal(entry["id"], &id) != nil || !uuidPattern.MatchString(id) {
			return errors.New("文章标识无效")
		}
		paths[strings.ToLower(id)] = true
	}
	for _, entry := range old {
		var slug string
		if data := entry["slug"]; len(data) > 0 && json.Unmarshal(data, &slug) != nil {
			return errors.New("文章自定义路径无效")
		}
		if slug != "" {
			key := strings.ToLower(slug)
			if !ValidArticleSlug(slug) || paths[key] {
				return errors.New("文章自定义路径无效或冲突")
			}
			paths[key] = true
		}
	}
	byID := map[string]map[string]json.RawMessage{}
	for _, entry := range old {
		var id string
		if json.Unmarshal(entry["id"], &id) != nil || !uuidPattern.MatchString(id) {
			return errors.New("文章标识无效")
		}
		var names []string
		if data, ok := entry["attachments"]; ok && json.Unmarshal(data, &names) != nil {
			return errors.New("原附件列表无效")
		}
		seen := map[string]bool{}
		for _, name := range names {
			key := strings.ToLower(name)
			if !SafeName(name) || seen[key] {
				return errors.New("原附件名称无效或冲突")
			}
			seen[key] = true
		}
		for _, key := range []string{"audioFilename", "videoFilename"} {
			var name string
			if raw, ok := entry[key]; ok && string(raw) != "null" {
				if json.Unmarshal(raw, &name) != nil || (name != "" && (!SafeName(name) || !seen[strings.ToLower(name)])) {
					return fmt.Errorf("原文章媒体字段 %s 无效", key)
				}
			}
		}
		byID[strings.ToUpper(id)] = entry
	}
	allowedArticle := map[string]bool{"id": true, "link": true, "slug": true, "tags": true, "title": true, "content": true, "created": true, "modified": true, "starType": true, "starred": true, "contentRendered": true, "articleNumber": true, "attachments": true, "audioFilename": true, "videoFilename": true}
	knownArticle := strings.Fields("articleType slug heroImage heroImageWidth heroImageHeight externalLink summary videoFilename audioFilename attachments cids tags isIncludedInNavigation navigationWeight originalSiteName originalSiteDomain originalPostID originalPostDate pinned")
	knownArticleSet := map[string]bool{}
	for _, key := range knownArticle {
		knownArticleSet[key] = true
	}
	for _, article := range articles {
		for _, name := range []string{article.AudioFilename, article.VideoFilename} {
			if name == "" {
				continue
			}
			found := false
			for _, attachment := range article.Attachments {
				if name == attachment {
					found = true
					break
				}
			}
			if !found || !SafeName(name) {
				return errors.New("文章媒体文件未登记")
			}
		}
		entry, ok := byID[strings.ToUpper(article.ID)]
		if article.SiteID != site.ID || !uuidPattern.MatchString(article.ID) {
			return errors.New("文章标识无效")
		}
		if !ok {
			if unsupportedMarkup.MatchString(article.Content) {
				return errors.New("文章包含暂不支持重建的 HTML 或视频链接")
			}
			continue
		}
		var link string
		var slug string
		_ = json.Unmarshal(entry["slug"], &slug)
		if json.Unmarshal(entry["link"], &link) != nil || (!strings.EqualFold(link, "/"+article.ID+"/") && (slug == "" || link != "/"+slug+"/")) {
			return errors.New("文章自定义路径暂不支持重建")
		}
		for key, value := range entry {
			if !allowedArticle[key] && (!knownArticleSet[key] || populated(value)) {
				return fmt.Errorf("文章设置 %s 暂不支持重建", key)
			}
		}
		if unsupportedMarkup.MatchString(article.Content) {
			return errors.New("文章包含暂不支持重建的 HTML 或视频链接")
		}
	}
	return nil
}

// CheckSimplePublicArticle rejects public metadata that the rebuilt page cannot represent.
func CheckSimplePublicArticle(fields map[string]json.RawMessage) error {
	allowed := map[string]bool{"id": true, "link": true, "slug": true, "tags": true, "title": true, "content": true, "contentRendered": true, "created": true, "modified": true, "articleNumber": true, "articleReference": true, "attachments": true, "hasVideo": true, "videoFilename": true, "hasAudio": true, "audioFilename": true, "audioDuration": true, "audioByteLength": true}
	known := strings.Fields("articleType slug externalLink hasVideo videoFilename hasAudio audioFilename audioDuration audioByteLength attachments heroImage heroImageWidth heroImageHeight heroImageURL heroImageFilename cids tags originalSiteName originalSiteDomain originalPostID originalPostDate pinned")
	knownSet := map[string]bool{}
	for _, key := range known {
		knownSet[key] = true
	}
	var names []string
	if raw, ok := fields["attachments"]; ok && json.Unmarshal(raw, &names) != nil {
		return errors.New("公开附件列表无效")
	}
	for _, kind := range []string{"Audio", "Video"} {
		var name string
		var enabled bool
		if raw, ok := fields[strings.ToLower(kind[:1])+kind[1:]+"Filename"]; ok && string(raw) != "null" && json.Unmarshal(raw, &name) != nil {
			return errors.New("公开媒体文件名无效")
		}
		if raw, ok := fields["has"+kind]; ok && string(raw) != "null" && json.Unmarshal(raw, &enabled) != nil {
			return errors.New("公开媒体标志无效")
		}
		found := false
		for _, n := range names {
			if n == name {
				found = true
				break
			}
		}
		if raw, ok := fields["has"+kind]; ok && string(raw) != "null" && !enabled && name != "" {
			return errors.New("公开媒体标志与文件名不一致")
		}
		if enabled && (name == "" || !found || !SafeName(name)) {
			return errors.New("公开媒体文件未登记")
		}
		if name != "" && (!found || !SafeName(name)) {
			return errors.New("公开媒体文件未登记")
		}
	}
	for _, key := range []string{"audioDuration", "audioByteLength"} {
		if raw, ok := fields[key]; ok && string(raw) != "null" {
			var audioName string
			if json.Unmarshal(fields["audioFilename"], &audioName) != nil || audioName == "" {
				return errors.New("音频元数据缺少音频文件")
			}
			var value int64
			if json.Unmarshal(raw, &value) != nil || value < 0 {
				return errors.New("公开音频元数据无效")
			}
		}
	}
	for key, value := range fields {
		if !allowed[key] && (!knownSet[key] || populated(value)) {
			return fmt.Errorf("公开文章设置 %s 暂不支持重建", key)
		}
	}
	return nil
}
